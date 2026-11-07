package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sync"

	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

// server owns one native process and its authenticated loopback connection.
type server struct {
	proc     *plugin.Process
	url      string
	password string
	client   *http.Client
}

func launch(ctx context.Context, dir string, env ...string) (*server, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, err
	}
	s := &server{password: hex.EncodeToString(secret[:]), client: &http.Client{}}
	cmd := exec.Command("opencode", "serve", "--stdio", "--port", "0")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Env = append(cmd.Env, "OPENCODE_PASSWORD="+s.password)
	var err error
	s.proc, err = plugin.StartProcess("OpenCode", cmd)
	if err != nil {
		return nil, err
	}
	ready := make(chan error, 1)
	var once sync.Once
	go func() {
		err := s.proc.Read(func(line []byte) {
			once.Do(func() {
				var info struct {
					URL string `json:"url"`
				}
				err := json.Unmarshal(line, &info)
				if err == nil {
					u, parseErr := url.Parse(info.URL)
					if parseErr != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" {
						err = errors.New("OpenCode returned no loopback server URL")
					} else {
						s.url = info.URL
					}
				}
				ready <- err
			})
		})
		once.Do(func() { ready <- err })
	}()
	select {
	case err = <-ready:
	case <-ctx.Done():
		err = ctx.Err()
	}
	if err != nil {
		s.stop()
		return nil, err
	}
	return s, nil
}

func (s *server) stop() {
	s.proc.Stop()
	s.client.CloseIdleConnections()
}

func (s *server) request(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var in io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		in = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.url+path, in)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("opencode", s.password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenCode %s %s: %w", method, path, err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		var detail struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&detail)
		if detail.Message == "" {
			detail.Message = res.Status
		}
		return nil, fmt.Errorf("OpenCode %s %s: %s", method, path, detail.Message)
	}
	return res, nil
}

func (s *server) call(ctx context.Context, method, path string, body, result any) error {
	res, err := s.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if result == nil {
		_, err = io.Copy(io.Discard, res.Body)
		return err
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("OpenCode %s: %w", path, err)
	}
	return json.Unmarshal(envelope.Data, result)
}
