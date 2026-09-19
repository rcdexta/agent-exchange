package ax

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/term"
)

const Version = "0.5.4"

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

// AX owns only name. Preserve every other argument, including native subcommands
// and the host's end-of-options separator.
func launchArgs(args []string) (string, []string, error) {
	var name string
	var native []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			native = append(native, args[i:]...)
			break
		}
		if arg == "--name" || arg == "-name" {
			if i+1 == len(args) {
				return "", nil, errors.New("name needs a value")
			}
			i++
			name = args[i]
		} else if strings.HasPrefix(arg, "--name=") || strings.HasPrefix(arg, "-name=") {
			name = strings.SplitN(arg, "=", 2)[1]
		} else {
			native = append(native, arg)
		}
	}
	if !validName.MatchString(name) {
		return "", nil, errors.New("choose an AX name with -name api")
	}
	return name, native, nil
}

func withAXOptions(native, options []string) []string {
	for i, arg := range native {
		if arg == "--" {
			out := append([]string(nil), native[:i]...)
			out = append(out, options...)
			return append(out, native[i:]...)
		}
	}
	return append(append([]string(nil), native...), options...)
}

func Launch(ctx context.Context, dir, host string, args []string) error {
	name, nativeArgs, e := launchArgs(args)
	if e != nil {
		return e
	}
	bypass := false
	if host == "codex" && !codexHelp(nativeArgs) {
		nativeArgs, bypass, e = codexBypassArgs(nativeArgs)
		if e != nil {
			return e
		}
	}
	if host == "grok" {
		for i, arg := range nativeArgs {
			if arg == "--" {
				break
			}
			if arg == "--always-approve" || arg == "--yolo" || arg == "--dangerously-skip-permissions" || arg == "--permission-mode=bypassPermissions" || (arg == "--permission-mode" && i+1 < len(nativeArgs) && nativeArgs[i+1] == "bypassPermissions") {
				bypass = true
			}
		}
	}
	nativeBin, e := exec.LookPath(host)
	if e != nil {
		return fmt.Errorf("install and sign in to %s first", host)
	}
	cwd, e := os.Getwd()
	if e != nil {
		return e
	}
	cwd, e = filepath.EvalSymlinks(cwd)
	if e != nil {
		return e
	}
	mesh, e := meshFor(cwd)
	if e != nil {
		return e
	}
	sessions := filepath.Join(dir, "sessions")
	if e = privateDir(sessions); e != nil {
		return e
	}
	path, e := namedSessionPath(sessions, name)
	if e != nil {
		return e
	}
	lock, e := lockFile(path + ".lock")
	if e != nil {
		return fmt.Errorf("agent %q is already running on this machine", name)
	}
	defer lock.Close()
	s, e := loadSession(path)
	if os.IsNotExist(e) {
		s = Session{ID: randomID("agt_"), Secret: randomID("") + randomID(""), Name: name, Host: host, Mesh: mesh, Workspace: cwd}
	} else if e != nil {
		return e
	}
	if s.Host != host {
		return fmt.Errorf("name %q belongs to %s; choose another name", name, s.Host)
	}
	s.Workspace = cwd
	s.Started = false // This launch must receive its own native SessionStart.
	s.AllowBypass = bypass || os.Getenv("AX_ALLOW_BYPASS") == "1"
	if e = saveSession(path, s); e != nil {
		return e
	}
	if e = ensureBroker(dir); e != nil {
		return e
	}
	c, e := dial(socketPath(dir))
	if e != nil {
		return e
	}
	e = c.call("ax.enroll", s, nil)
	c.close()
	if e != nil {
		return e
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	env := append(os.Environ(), "AX_HOME="+dir, "AX_SESSION_FILE="+path)
	// Native daemons may run hooks without the launching shell environment.
	hookCommand := "AX_HOME=" + shellQuote(dir) + " AX_SESSION_FILE=" + shellQuote(path) + " " + shellQuote(exe) + " hook"
	startupErrors := make(chan error, 1)
	var argv []string
	if host == "claude" {
		config := object{"mcpServers": object{"ax": object{"command": exe, "args": []string{"bridge"}, "alwaysLoad": true, "env": object{"AX_HOME": dir, "AX_SESSION_FILE": path}}}}
		hook := object{"type": "command", "command": hookCommand, "timeout": 3}
		hooks := object{}
		for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PermissionRequest", "PostToolUse", "Stop"} {
			hooks[event] = []object{{"hooks": []object{hook}}}
		}
		settings := object{"hooks": hooks}
		allowed := []string{}
		for _, tool := range toolSpecs {
			allowed = append(allowed, "mcp__ax__"+tool.name)
		}
		argv = []string{"--mcp-config", string(raw(config)), "--settings", string(raw(settings)), "--dangerously-load-development-channels", "server:ax", "--allowedTools", strings.Join(allowed, ","), "--append-system-prompt", instructions}
		if len(nativeArgs) == 0 && s.Native != "" {
			nativeArgs = []string{"--resume", s.Native}
		}
		fmt.Fprintf(os.Stderr, "Agent Exchange · %s\nConfirm Claude's local development Channel prompt when it appears.\n", name)
	} else if host == "codex" {
		b, e := exec.Command(nativeBin, "queue", "--help").Output()
		if e != nil || !strings.Contains(string(b), "--thread") {
			return errors.New("AX needs Codex with native queue support; update Codex first")
		}
		config := []string{"mcp_servers.ax.command=" + strconv.Quote(exe), "mcp_servers.ax.args=[\"bridge\"]", "mcp_servers.ax.env.AX_HOME=" + strconv.Quote(dir), "mcp_servers.ax.env.AX_SESSION_FILE=" + strconv.Quote(path), "mcp_servers.ax.enabled=true"}
		if !codexHelp(nativeArgs) {
			remote, cleanup, err := startCodex(ctx, dir, path, nativeBin, append(codexConfigOverrides(nativeArgs), config...), startupErrors)
			if err != nil {
				return err
			}
			defer cleanup()
			s.CodexRemote = remote
			if err = saveSession(path, s); err != nil {
				return err
			}
			if bypass {
				remote, cleanup, err = codexBypassSocket(ctx, dir, remote)
				if err != nil {
					return err
				}
				defer cleanup()
			}
			argv = append(argv, "--remote", remote)
		}
		for _, v := range config {
			argv = append(argv, "-c", v)
		}
		if len(nativeArgs) == 0 && s.Native != "" {
			nativeArgs = []string{"resume", s.Native}
		}
		fmt.Fprintf(os.Stderr, "Agent Exchange · %s\nCodex connects AX automatically after selecting your session.\n", name)
	} else {
		adapter := harnesses[host]
		if adapter.prepare == nil {
			return fmt.Errorf("unsupported harness %q", host)
		}
		var cleanup func()
		nativeArgs, env, cleanup, e = adapter.prepare(ctx, dir, path, nativeBin, s, nativeArgs, env, startupErrors)
		if e != nil {
			return e
		}
		defer cleanup()
		fmt.Fprintf(os.Stderr, "Agent Exchange · %s · %s\n", name, host)
	}
	// Keep injected Codex configuration in the subcommand's option scope, where
	// it survives resume alongside user-supplied subcommand config overrides.
	argv = withAXOptions(nativeArgs, argv)
	cmd := exec.CommandContext(ctx, nativeBin, argv...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Let the real harness own terminal input and its normal signal handling.
	// AX never emulates keystrokes or reads/mutates its private transcript files.
	ignored := make(chan os.Signal, 2)
	signal.Notify(ignored, os.Interrupt)
	defer signal.Stop(ignored)
	return runHost(cmd, startupErrors)
}

// Startup errors must survive the TUI, rather than disappear behind a session
// that looks usable while its AX endpoint remains permanently "starting".
func runHost(cmd *exec.Cmd, startupErrors <-chan error) error {
	restore := func() {}
	if input, ok := cmd.Stdin.(*os.File); ok {
		fd := int(input.Fd())
		if state, err := term.GetState(fd); err == nil {
			restore = func() {
				term.Restore(fd, state)
				if cmd.Stdout != nil {
					// A killed native TUI cannot disable its terminal reporting modes.
					fmt.Fprint(cmd.Stdout, "\x1b[<u\x1b[=0u\x1b[>4;0m\x1b[?2004l\x1b[?1004l\x1b[?1049l\x1b[0 q\x1b[?25h\x1b[0m\n")
				}
			}
		}
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case err := <-startupErrors:
		cmd.Process.Kill()
		<-done
		restore()
		return err
	}
}
func Hook(dir, file string, in io.Reader) error {
	s, e := loadSession(file)
	if e != nil {
		return e
	}
	var input struct {
		Session    string `json:"session_id"`
		Event      string `json:"hook_event_name"`
		Permission string `json:"permission_mode"`
		Tool       string `json:"tool_name"`
	}
	if e = json.NewDecoder(io.LimitReader(in, maxFrame)).Decode(&input); e != nil {
		return e
	}
	if !validNative(input.Session) {
		return errors.New("invalid native session ID in lifecycle hook")
	}
	if s.Native == "" && input.Event == "SessionStart" {
		s.Native = input.Session
	}
	if input.Session != s.Native {
		if input.Event == "SessionStart" {
			return fmt.Errorf("AX name %q belongs to conversation %s, but %s opened %s. Run ax %s -name %s to resume the saved conversation, or choose a new AX name for this one", s.Name, s.Native, s.Host, input.Session, s.Host, s.Name)
		}
		return nil
	}
	if input.Event == "SessionStart" {
		s.Started = true
		if e = saveSession(file, s); e != nil {
			return e
		}
	}
	state := "busy"
	switch input.Event {
	case "SessionStart", "Stop":
		state = "ready"
	case "PermissionRequest":
		state = "blocked"
	}
	if input.Tool == "AskUserQuestion" {
		state = "blocked"
	}
	c, e := dial(socketPath(dir))
	if e != nil {
		return e
	}
	defer c.close()
	return c.call("ax.lifecycle", object{"agent_id": s.ID, "secret": s.Secret, "native_session_id": input.Session, "permission_mode": input.Permission, "state": state}, nil)
}
func Main(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Println(`Agent Exchange — connect your local coding agents.

  ax claude -name api
  ax codex -name web
  ax grok -name worker
  ax opencode -name editor

AX owns the name option. Other arguments go to the native harness.
  ax claude -name api -r "session-name"
  ax codex -name web resume "session-name"
  ax grok -name worker -r "session-name"
  ax opencode -name editor -s SESSION_ID

Ask either agent to message another by name.

  ax agents                     List local agents
  ax status MESSAGE_ID          Inspect delivery receipts
  ax resolve MESSAGE_ID abandon Release a stuck message without redelivery
  ax policy NAME hold           Pause incoming mail (accept/hold/refuse)
  ax doctor                     Check installed harnesses
  ax version                    Show the AX version`)
		return nil
	}
	if args[0] == "version" {
		fmt.Println(Version)
		return nil
	}
	dir, e := dataDir()
	if e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer cancel()
	switch args[0] {
	case "serve":
		return Serve(ctx, dir)
	case "claude", "codex", "grok", "opencode":
		return Launch(ctx, dir, args[0], args[1:])
	case "bridge":
		return Bridge(ctx, dir, os.Getenv("AX_SESSION_FILE"), os.Stdin, os.Stdout)
	case "hook":
		return Hook(dir, os.Getenv("AX_SESSION_FILE"), os.Stdin)
	case "doctor":
		for _, host := range []string{"claude", "codex", "grok", "opencode"} {
			cmd := exec.Command(host, "--version")
			b, e := cmd.Output()
			if e != nil {
				fmt.Printf("%s: unavailable\n", host)
			} else {
				fmt.Printf("%s: %s", host, b)
			}
		}
		fmt.Println("Claude: interactive development Channel opt-in required at launch.\nDiscovery: named agents connect across repositories on this machine.\nDelivery: queued and acknowledged are reported separately.")
		return ensureBroker(dir)
	case "agents", "status", "policy", "resolve":
		if e = ensureBroker(dir); e != nil {
			return e
		}
		c, e := dial(socketPath(dir))
		if e != nil {
			return e
		}
		defer c.close()
		var result any
		switch args[0] {
		case "agents":
			var agents []Agent
			e = c.call("ax.inspect", object{}, &agents)
			if e != nil {
				return e
			}
			if len(agents) == 0 {
				fmt.Println("No AX agents yet. Launch a named session with ax HARNESS -name NAME.")
			}
			for _, a := range agents {
				state := a.State
				if !a.Online {
					state = "offline"
				}
				fmt.Printf("%-18s %-8s %-10s policy=%s\n", a.Name, a.Host, state, a.Policy)
			}
			return nil
		case "status":
			if len(args) != 2 {
				return errors.New("usage: ax status MESSAGE_ID")
			}
			e = c.call("ax.inspect_message", object{"message_id": args[1]}, &result)
		case "resolve":
			if len(args) != 3 || args[2] != "abandon" {
				return errors.New("usage: ax resolve MESSAGE_ID abandon")
			}
			e = c.call("ax.abandon", object{"message_id": args[1]}, &result)
		case "policy":
			if len(args) != 3 {
				return errors.New("usage: ax policy NAME accept|hold|refuse")
			}
			e = c.call("ax.policy", object{"target": args[1], "policy": args[2]}, &result)
		}
		if e != nil {
			return e
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	return fmt.Errorf("unknown command %q; run ax help", args[0])
}
