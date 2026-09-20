package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Codex defers SessionStart hooks until a user turn. Run its native backend on a
// private socket so its ordinary TUI can select a thread before AX wakes it.
func startCodex(ctx context.Context, dir, file, bin string, config []string, startupErrors chan<- error) (string, func(), error) {
	ctx, cancel := context.WithCancel(ctx)
	socket := filepath.Join(dir, randomID("codex_")+".sock")
	log, err := openDiagnosticLog(dir, "codex.log")
	if err != nil {
		cancel()
		return "", nil, err
	}
	args := []string{"app-server", "--listen", "unix://" + socket, "-c", "features.remote_control=false"}
	for _, value := range config {
		args = append(args, "-c", value)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	// Shell commands issued by this backend belong to this AX launch too.
	cmd.Env = append(os.Environ(), "AX_HOME="+dir, "AX_SESSION_FILE="+file)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = cmd.Start(); err != nil {
		cancel()
		log.Close()
		return "", nil, err
	}
	done := make(chan error, 1)
	go watchDiagnosticLog(ctx, dir, "codex.log")
	go func() { done <- cmd.Wait(); log.Close() }()
	cleanup := func() { cancel(); <-done; os.Remove(socket) }
	var conn *websocket.Conn
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		conn, err = dialCodex(ctx, "unix://"+socket)
		if err == nil {
			break
		}
		select {
		case err = <-done:
			cancel()
			os.Remove(socket)
			return "", nil, fmt.Errorf("Codex backend exited: %v; inspect %s", err, filepath.Join(dir, "codex.log"))
		case <-ctx.Done():
		case <-time.After(50 * time.Millisecond):
		}
	}
	if conn == nil {
		if err == nil {
			err = ctx.Err()
		}
		cleanup()
		return "", nil, fmt.Errorf("Codex backend startup: %w", err)
	}
	conn.SetReadLimit(4 << 20)
	var result any
	if err = codexCall(conn, 1, "initialize", object{"clientInfo": object{"name": "ax", "version": Version}}, &result); err != nil {
		conn.Close()
		cleanup()
		return "", nil, err
	}
	stop := cleanup
	cleanup = func() { cancel(); conn.Close(); stop() }
	go func() {
		defer conn.Close()
		if err := bindCodex(ctx, conn, dir, file); err != nil && ctx.Err() == nil {
			startupErrors <- fmt.Errorf("Codex connection: %w", err)
		}
	}()
	return "unix://" + socket, cleanup, nil
}

func dialCodex(ctx context.Context, remote string) (*websocket.Conn, error) {
	if !strings.HasPrefix(remote, "unix:///") {
		return nil, errors.New("AX requires a private local Codex socket")
	}
	dialer := websocket.Dialer{HandshakeTimeout: time.Second, NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", strings.TrimPrefix(remote, "unix://"))
	}}
	conn, _, err := dialer.DialContext(ctx, "ws://localhost", nil)
	return conn, err
}

// Address the backend that owns the loaded thread. Queuing through a separate
// embedded backend leaves delivery to Codex's ten-second external queue watcher.
func queueCodex(ctx context.Context, s Session, text string) error {
	conn, err := dialCodex(ctx, s.CodexRemote)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadLimit(4 << 20)
	var result any
	if err = codexCall(conn, 1, "initialize", object{"clientInfo": object{"name": "ax", "version": Version}, "capabilities": object{"experimentalApi": true}}, &result); err != nil {
		return err
	}
	// Only the fixed wake enters user input. An ambiguous response must not retry
	// through the CLI, since the native queue may already have accepted it.
	return codexCall(conn, 2, "thread/queue/add", object{"threadId": s.Native, "input": []object{{"type": "text", "text": text, "text_elements": []any{}}}, "clientUserMessageId": uuid()}, &result)
}

func codexCall(conn *websocket.Conn, id int, method string, params, result any) error {
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteJSON(object{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var reply packet
		if err := conn.ReadJSON(&reply); err != nil {
			return err
		}
		if string(reply.ID) != fmt.Sprint(id) {
			continue
		}
		if reply.Error != nil {
			return reply.Error
		}
		return json.Unmarshal(reply.Result, result)
	}
}

func bindCodex(ctx context.Context, conn *websocket.Conn, dir, file string) error {
	for id := 2; ctx.Err() == nil; id += 2 {
		var loaded struct {
			Data []string `json:"data"`
		}
		if err := codexCall(conn, id, "thread/loaded/list", object{}, &loaded); err != nil {
			return err
		}
		// This backend belongs only to this launcher. Filter subagents if the user
		// supplied an initial prompt that started one before we observed the root.
		for _, native := range loaded.Data {
			var info struct {
				Thread struct {
					ID     string          `json:"id"`
					Source json.RawMessage `json:"source"`
				} `json:"thread"`
			}
			if err := codexCall(conn, id+1, "thread/read", object{"threadId": native, "includeTurns": false}, &info); err != nil {
				return err
			}
			var source string
			if json.Unmarshal(info.Thread.Source, &source) != nil {
				continue
			}
			if !validNative(info.Thread.ID) || info.Thread.ID != native {
				return errors.New("invalid Codex thread identity")
			}
			return Hook(dir, file, bytes.NewReader(raw(object{"session_id": native, "hook_event_name": "SessionStart"})))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return ctx.Err()
}

// Remote TUIs send model/sandbox choices in thread RPCs, but server configuration
// belongs to the backend. Give it the same native config overrides as the TUI.
func codexConfigOverrides(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if (arg == "-c" || arg == "--config") && i+1 < len(args) {
			i++
			out = append(out, args[i])
		} else if strings.HasPrefix(arg, "--config=") {
			out = append(out, strings.TrimPrefix(arg, "--config="))
		} else if strings.HasPrefix(arg, "-c") && len(arg) > 2 {
			out = append(out, strings.TrimPrefix(strings.TrimPrefix(arg, "-c"), "="))
		}
	}
	return out
}

func codexHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "-h" || arg == "--help" || arg == "-V" || arg == "--version" {
			return true
		}
	}
	return false
}
