package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

// A plugin answers two one-shot discovery commands besides `serve`. Each
// writes one JSON value to stdout and exits; a failure exits nonzero with the
// reason on stderr.
//
// `describe` reads nothing and writes a Description. It starts no native
// agent.
//
// `models` reads a ModelsRequest from stdin and writes a Catalog: the models
// the native harness offers in the request's directory. With a model, the
// catalog also lists that model's efforts.

// Settings is a complete harness, model, and effort combination. An empty
// Effort leaves the effort to the harness.
type Settings struct {
	Harness string `json:"harness"`
	Model   string `json:"model"`
	Effort  string `json:"effort"`
}

// Check reports a missing harness or model.
func (s Settings) Check() error {
	switch {
	case strings.TrimSpace(s.Harness) == "":
		return errors.New("missing harness")
	case strings.TrimSpace(s.Model) == "":
		return errors.New("missing model")
	}
	return nil
}

// Description is a plugin's answer to `describe`. Installed reports whether
// the native harness's executable is on the backend's PATH; it does not mean
// the harness is signed in.
type Description struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
}

// ModelsRequest asks for the models offered in Dir, an absolute directory,
// and, with Model, that model's efforts.
type ModelsRequest struct {
	Dir   string `json:"dir"`
	Model string `json:"model,omitempty"`
}

// Model is one model a harness offers. ID is the native ID a session passes
// back. Resolved is the model an alias stands for, when the harness reports
// it.
type Model struct {
	ID       string `json:"id"`
	Label    string `json:"label,omitempty"`
	Provider string `json:"provider,omitempty"`
	Resolved string `json:"resolved,omitempty"`
}

// Catalog is a plugin's answer to `models`. Default is the harness's default
// model when it reports one. For a requested Model, Efforts lists its efforts
// in native order, empty when the model has no effort setting, and
// DefaultEffort is the effort the harness uses when none is chosen, when it
// reports one.
type Catalog struct {
	Models        []Model  `json:"models"`
	Default       string   `json:"default,omitempty"`
	Model         string   `json:"model,omitempty"`
	Efforts       []string `json:"efforts,omitempty"`
	DefaultEffort string   `json:"default_effort,omitempty"`
}

// Check validates a catalog received from a plugin for the request.
func (c Catalog) Check(req ModelsRequest) error {
	ids := map[string]bool{}
	for _, m := range c.Models {
		switch {
		case strings.TrimSpace(m.ID) == "":
			return errors.New("a model has no ID")
		case ids[m.ID]:
			return fmt.Errorf("model %q is listed twice", m.ID)
		}
		ids[m.ID] = true
	}
	if c.Default != "" && !ids[c.Default] {
		return fmt.Errorf("default model %q is not listed", c.Default)
	}
	if c.Model != req.Model {
		return fmt.Errorf("the catalog is for model %q, not %q", c.Model, req.Model)
	}
	if req.Model == "" && (len(c.Efforts) > 0 || c.DefaultEffort != "") {
		return errors.New("efforts without a requested model")
	}
	for i, e := range c.Efforts {
		if strings.TrimSpace(e) == "" || slices.Contains(c.Efforts[:i], e) {
			return fmt.Errorf("model %q has an empty or repeated effort", req.Model)
		}
	}
	if c.DefaultEffort != "" && !slices.Contains(c.Efforts, c.DefaultEffort) {
		return fmt.Errorf("default effort %q is not listed", c.DefaultEffort)
	}
	return nil
}

// Harness is an installed plugin and what it said about its native harness.
// Error is why the plugin could not describe it.
type Harness struct {
	ID string `json:"id"`
	Description
	Error string `json:"error,omitempty"`
}

const pluginPrefix = "hypercube-harness-"

// pluginDir is dir, or the running executable's directory when dir is empty.
func pluginDir(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// Plugins returns the IDs of the executable plugins in dir, or beside the
// running executable when dir is empty, in order.
func Plugins(dir string) ([]string, error) {
	dir, err := pluginDir(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		id, ok := strings.CutPrefix(e.Name(), pluginPrefix)
		if !ok || id == "" {
			continue
		}
		// Stat follows the links a development build installs.
		info, err := os.Stat(filepath.Join(dir, e.Name()))
		if err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// Find returns the path of the harness's plugin in dir, or beside the running
// executable when dir is empty. The harness must name an installed plugin.
func Find(dir, name string) (string, error) {
	ids, err := Plugins(dir)
	if err != nil {
		return "", err
	}
	if !slices.Contains(ids, name) {
		return "", fmt.Errorf("harness %q has no plugin", name)
	}
	dir, err = pluginDir(dir)
	return filepath.Join(dir, pluginPrefix+name), err
}

// describeTimeout bounds `describe`, which only looks for an executable.
const describeTimeout = 10 * time.Second

// Describe asks every plugin in dir about its native harness. A plugin that
// fails is listed with its error.
func Describe(ctx context.Context, dir string) ([]Harness, error) {
	ids, err := Plugins(dir)
	if err != nil {
		return nil, err
	}
	list := make([]Harness, len(ids))
	done := make(chan struct{})
	for i, id := range ids {
		go func() {
			defer func() { done <- struct{}{} }()
			list[i].ID = id
			ctx, cancel := context.WithTimeout(ctx, describeTimeout)
			defer cancel()
			if err := run(ctx, dir, id, "describe", nil, &list[i].Description); err != nil {
				list[i].Error = err.Error()
			}
		}()
	}
	for range ids {
		<-done
	}
	return list, nil
}

// Models asks the harness's plugin in dir for its catalog.
func Models(ctx context.Context, dir, harness string, req ModelsRequest) (Catalog, error) {
	var c Catalog
	if !filepath.IsAbs(req.Dir) {
		return c, fmt.Errorf("directory %q is not absolute", req.Dir)
	}
	if err := run(ctx, dir, harness, "models", req, &c); err != nil {
		return c, err
	}
	if err := c.Check(req); err != nil {
		return Catalog{}, fmt.Errorf("harness %s: invalid catalog: %w", harness, err)
	}
	return c, nil
}

// cancelGrace is how long a cancelled plugin has to clean up what it started,
// such as a temporary native session, before its process group is killed.
var cancelGrace = 15 * time.Second

// run runs a one-shot plugin command and decodes its output. Cancelling ctx
// asks the plugin to stop with SIGTERM, then kills its process group, native
// processes included, after cancelGrace.
func run(ctx context.Context, dir, harness, command string, in, out any) error {
	path, err := Find(dir, harness)
	if err != nil {
		return err
	}
	cmd := exec.Command(path, command)
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		cmd.Stdin = bytes.NewReader(data)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Native processes may keep the output open after the plugin exits.
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("harness %s: %w", harness, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = cmd.Process.Signal(syscall.SIGTERM)
		grace := time.NewTimer(cancelGrace)
		select {
		case <-done:
			grace.Stop()
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		case <-grace.C:
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
		return fmt.Errorf("harness %s: %w", harness, ctx.Err())
	}
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if i := strings.LastIndexByte(detail, '\n'); i >= 0 {
			detail = detail[i+1:]
		}
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("harness %s: %s", harness, strings.TrimPrefix(detail, pluginPrefix+harness+": "))
	}
	if err := json.Unmarshal(stdout.Bytes(), out); err != nil {
		return fmt.Errorf("harness %s: malformed %s output: %w", harness, command, err)
	}
	return nil
}
