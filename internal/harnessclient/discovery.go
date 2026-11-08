package harnessclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

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
func Describe(ctx context.Context, dir string) ([]harness.Harness, error) {
	ids, err := Plugins(dir)
	if err != nil {
		return nil, err
	}
	list := make([]harness.Harness, len(ids))
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
func Models(ctx context.Context, dir, harnessID string, req harness.ModelsRequest) (harness.Catalog, error) {
	var c harness.Catalog
	if !filepath.IsAbs(req.Dir) {
		return c, fmt.Errorf("directory %q is not absolute", req.Dir)
	}
	if err := run(ctx, dir, harnessID, "models", req, &c); err != nil {
		return c, err
	}
	if err := c.Check(req); err != nil {
		return harness.Catalog{}, fmt.Errorf("harness %s: invalid catalog: %w", harnessID, err)
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
