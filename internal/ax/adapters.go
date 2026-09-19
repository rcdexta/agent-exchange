package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// A harness owns session selection and native wake delivery. The broker and
// messaging tools remain shared, independent of the harness's implementation.
type harnessAdapter struct {
	toolPrefix       string
	nativeID         func(string) bool
	readyOnDiscovery bool
	prepare          func(context.Context, string, string, string, Session, []string, []string, chan<- error) ([]string, []string, func(), error)
}

var harnesses = map[string]harnessAdapter{
	"claude":   {toolPrefix: "mcp__ax__", nativeID: validNative},
	"codex":    {toolPrefix: "ax.", nativeID: validNative},
	"grok":     {toolPrefix: "ax__", nativeID: validNative, readyOnDiscovery: true},
	"opencode": {toolPrefix: "ax_", nativeID: regexp.MustCompile(`^ses_[a-zA-Z0-9]+$`).MatchString, readyOnDiscovery: true},
}

func init() {
	grok, opencode := harnesses["grok"], harnesses["opencode"]
	grok.prepare, opencode.prepare = prepareGrok, prepareOpenCode
	harnesses["grok"], harnesses["opencode"] = grok, opencode
}

func nativeID(host, id string) bool {
	a, ok := harnesses[host]
	return ok && a.nativeID(id)
}

func toolName(host, name string) string { return harnesses[host].toolPrefix + name }

func bindAdapter(dir, file, native, permission, state string) error {
	s, err := loadSession(file)
	if err != nil {
		return err
	}
	if !nativeID(s.Host, native) {
		return errors.New("invalid native session ID")
	}
	if s.Native != "" && s.Native != native {
		return fmt.Errorf("AX name %q belongs to conversation %s; choose a new AX name for %s", s.Name, s.Native, native)
	}
	s.Native, s.Started = native, true
	if err = saveSession(file, s); err != nil {
		return err
	}
	c, err := dial(socketPath(dir))
	if err != nil {
		return err
	}
	defer c.close()
	return c.call("ax.lifecycle", object{"agent_id": s.ID, "secret": s.Secret, "native_session_id": native, "permission_mode": permission, "state": state}, nil)
}

// The adapter socket is private to one launch. A native plugin may wait for a
// wake here, or a Go adapter can supply its own native ingress implementation.
type adapterWake struct {
	ID     string `json:"id"`
	Native string `json:"native"`
	Text   string `json:"text"`
	done   chan error
}

type adapterHost struct {
	mu      sync.Mutex
	pending map[string]*adapterWake
	wakes   chan *adapterWake
	wake    func(context.Context, string, string) error
}

func startAdapterHost(ctx context.Context, dir, file string, failures chan<- error, wake func(context.Context, string, string) error) (string, func(), error) {
	path := filepath.Join(dir, randomID("adapter_")+".sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		return "", nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		l.Close()
		return "", nil, err
	}
	a := &adapterHost{pending: map[string]*adapterWake{}, wakes: make(chan *adapterWake), wake: wake}
	mux := http.NewServeMux()
	mux.HandleFunc("/bind", func(w http.ResponseWriter, r *http.Request) {
		var input struct{ Native, Permission, State string }
		err := json.NewDecoder(io.LimitReader(r.Body, maxFrame)).Decode(&input)
		if err == nil {
			err = bindAdapter(dir, file, input.Native, input.Permission, input.State)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			select {
			case failures <- err:
			default:
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/wake", func(w http.ResponseWriter, r *http.Request) {
		var input adapterWake
		err := json.NewDecoder(io.LimitReader(r.Body, maxFrame)).Decode(&input)
		s, loadErr := loadSession(file)
		if err != nil || loadErr != nil || !s.Started || input.Native != s.Native {
			http.Error(w, "adapter session is not ready", http.StatusConflict)
			return
		}
		if a.wake != nil {
			err = a.wake(r.Context(), input.Native, input.Text)
		} else {
			input.ID, input.done = randomID("wake_"), make(chan error, 1)
			a.mu.Lock()
			a.pending[input.ID] = &input
			a.mu.Unlock()
			defer func() { a.mu.Lock(); delete(a.pending, input.ID); a.mu.Unlock() }()
			select {
			case a.wakes <- &input:
				select {
				case err = <-input.done:
				case <-r.Context().Done():
					err = r.Context().Err()
				}
			case <-r.Context().Done():
				err = r.Context().Err()
			}
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/next", func(w http.ResponseWriter, r *http.Request) {
		select {
		case wake := <-a.wakes:
			json.NewEncoder(w).Encode(wake)
		case <-r.Context().Done():
		}
	})
	mux.HandleFunc("/receipt", func(w http.ResponseWriter, r *http.Request) {
		var input struct{ ID, Error string }
		if json.NewDecoder(io.LimitReader(r.Body, maxFrame)).Decode(&input) != nil {
			http.Error(w, "invalid receipt", 400)
			return
		}
		a.mu.Lock()
		pending := a.pending[input.ID]
		a.mu.Unlock()
		if pending != nil {
			var err error
			if input.Error != "" {
				err = errors.New(input.Error)
			}
			select {
			case pending.done <- err:
			default:
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := &http.Server{Handler: mux, BaseContext: func(net.Listener) context.Context { return ctx }}
	go server.Serve(l)
	return path, func() { server.Close(); os.Remove(path) }, nil
}

func notifyAdapter(ctx context.Context, s Session, text string) error {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", s.AdapterSocket)
	}}
	defer transport.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, "POST", "http://ax/wake", bytes.NewReader(raw(adapterWake{Native: s.Native, Text: text})))
	if err != nil {
		return err
	}
	res, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("native wake was not confirmed: %s", res.Status)
	}
	return nil
}
