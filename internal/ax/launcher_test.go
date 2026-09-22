package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLaunchArgsPreserveNativeSyntax(t *testing.T) {
	for _, tc := range []struct{ args, native []string }{
		{[]string{"--name", "api", "--resume", "session name", "--model", "sonnet"}, []string{"--resume", "session name", "--model", "sonnet"}},
		{[]string{"--name=api", "resume", "session name", "-c", "model_reasoning_effort=\"low\"", "my prompt"}, []string{"resume", "session name", "-c", "model_reasoning_effort=\"low\"", "my prompt"}},
		{[]string{"--name=api", "--resume"}, []string{"--resume"}},
		{[]string{"--name", "api", "--", "--name", "literal prompt"}, []string{"--", "--name", "literal prompt"}},
		{[]string{"--name", "api", "--unknown-future-option", "value"}, []string{"--unknown-future-option", "value"}},
		{[]string{"-n", "api", "--model", "sonnet"}, []string{"--model", "sonnet"}},
		{[]string{"-n=api", "resume", "session name"}, []string{"resume", "session name"}},
		{[]string{"-n", "api", "--", "-n", "literal prompt"}, []string{"--", "-n", "literal prompt"}},
		// -name is an ordinary native argument now, not the AX name option.
		{[]string{"-n", "api", "-name", "display"}, []string{"-name", "display"}},
	} {
		name, args, err := launchArgs(tc.args)
		if err != nil || name != "api" || !reflect.DeepEqual(args, tc.native) {
			t.Fatalf("%q: name=%q args=%q error=%v", tc.args, name, args, err)
		}
	}
	for _, args := range [][]string{{"--name"}, {"-n"}, {"--resume", "session"}, {"--name", "bad/name"}, {"-n", "bad/name"}} {
		if _, _, err := launchArgs(args); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
	// The removed spelling gets an error that names its replacement.
	for _, args := range [][]string{{"-name", "api"}, {"-name=api"}, {"-name", "api", "-r", "s"}} {
		_, _, err := launchArgs(args)
		if err == nil || !strings.Contains(err.Error(), "-name is no longer the AX name option") {
			t.Fatalf("%q: %v", args, err)
		}
	}
}

// Claude Code names its own sessions, so the AX name should reach it and show in
// the prompt box, resume picker, and terminal title. A harness release without the
// option must still launch.
func TestLauncherForwardsAXNameToNativeDisplayName(t *testing.T) {
	for _, tc := range []struct {
		help string
		want bool
	}{
		{"Options:\n  -n, --name <name>  Set a display name for this session\n", true},
		{"Options:\n  -r, --resume [id]  Resume a conversation\n", false},
		// A longer option that merely starts with the flag is not the flag.
		{"Options:\n  --namespace <ns>  Unrelated\n", false},
	} {
		dir := startTestServer(t)
		bin := testDir(t)
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		out := filepath.Join(bin, "claude.args")
		script := "#!/bin/sh\nif [ \"$1\" = --help ]; then printf '%s' " + shellQuote(tc.help) + "; exit 0; fi\nprintf '%s\\0' \"$@\" > " + shellQuote(out) + "\n"
		if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		if err := Launch(context.Background(), dir, "claude", []string{"--name", "api"}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		argv := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
		i := slices.Index(argv, "--name")
		got := i >= 0 && i+1 < len(argv) && argv[i+1] == "api"
		if got != tc.want {
			t.Fatalf("help %q: forwarded=%v want=%v argv=%q", tc.help, got, tc.want, argv)
		}
	}
}

// The probe is an optional lookup for a cosmetic option. A harness that stalls
// on --help must not stall the launch, so the probe gives up and omits the name.
func TestLauncherNameProbeCannotStallLaunch(t *testing.T) {
	dir := startTestServer(t)
	bin := testDir(t)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out := filepath.Join(bin, "claude.args")
	script := "#!/bin/sh\nif [ \"$1\" = --help ]; then sleep 600; fi\nprintf '%s\\0' \"$@\" > " + shellQuote(out) + "\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := Launch(context.Background(), dir, "claude", []string{"--name", "api"}); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited > flagProbeTimeout+10*time.Second {
		t.Fatalf("probe stalled the launch for %s", waited)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(strings.Split(string(data), "\x00"), "--name") {
		t.Fatal("forwarded a name the harness never advertised")
	}
}

func startTestServer(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ax-launch-")
	if err != nil {
		t.Fatal(err)
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, dir) }()
	t.Cleanup(func() { cancel(); <-done; os.RemoveAll(dir) })
	for i := 0; i < 100; i++ {
		c, err := dial(socketPath(dir))
		if err == nil {
			c.close()
			return dir
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not start")
	return ""
}

func TestLauncherForwardsPromptAndResume(t *testing.T) {
	dir := startTestServer(t)
	bin := testDir(t)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, host := range []string{"claude", "codex"} {
		out := filepath.Join(bin, host+".args")
		script := "#!/bin/sh\nif [ \"$1\" = queue ]; then printf '%s\\n' --thread; exit 0; fi\nprintf '%s\\0' \"$@\" > " + shellQuote(out) + "\n"
		if err := os.WriteFile(filepath.Join(bin, host), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		native := []string{"--resume", "existing session", "--model", "sonnet", "--", "literal --name prompt"}
		if host == "codex" {
			native = []string{"resume", "existing session", "-c", "model_reasoning_effort=\"low\"", "--help", "--", "literal --name prompt"}
		}
		args := append([]string{"--name", host}, native...)
		if err := Launch(context.Background(), dir, host, args); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		actual := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
		split := len(native) - 2
		if !reflect.DeepEqual(actual[:split], native[:split]) || !reflect.DeepEqual(actual[len(actual)-2:], native[split:]) {
			t.Fatalf("native args changed: %q", actual)
		}
		for i, arg := range actual {
			if strings.Contains(arg, "Call the ax list_agents tool once") {
				t.Fatal("bootstrap replaced native prompt")
			}
			if host == "claude" && arg == "--mcp-config" {
				var config struct {
					Servers map[string]struct {
						AlwaysLoad bool `json:"alwaysLoad"`
					} `json:"mcpServers"`
				}
				if json.Unmarshal([]byte(actual[i+1]), &config) != nil || !config.Servers["ax"].AlwaysLoad {
					t.Fatal("AX tools require a discovery turn")
				}
			}
		}
	}
}

func TestFreshClaudeNameResumesOnlyAfterNativeBinding(t *testing.T) {
	dir, bin := startTestServer(t), testDir(t)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out := filepath.Join(bin, "args")
	script := "#!/bin/sh\nprintf '%s\\0' \"$@\" > " + shellQuote(out) + "\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	launchArgs := func() []string {
		t.Helper()
		if err := Launch(context.Background(), dir, "claude", []string{"--name", "fresh"}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00")
	}
	for i := 0; i < 2; i++ {
		// The fake harness exits before SessionStart, leaving an unbound identity.
		for _, arg := range launchArgs() {
			if arg == "--resume" || arg == "-r" {
				t.Fatalf("launch %d resumed an unbound conversation", i)
			}
		}
	}
	path, err := namedSessionPath(filepath.Join(dir, "sessions"), "fresh")
	if err != nil {
		t.Fatal(err)
	}
	native := uuid()
	if err := Hook(dir, path, bytes.NewReader(raw(object{"session_id": native, "hook_event_name": "SessionStart", "permission_mode": "default"}))); err != nil {
		t.Fatal(err)
	}
	actual := launchArgs()
	if len(actual) < 2 || actual[0] != "--resume" || actual[1] != native {
		t.Fatalf("saved native binding was not resumed: %q", actual)
	}
}

func TestLifecycleBindsExistingSession(t *testing.T) {
	dir := startTestServer(t)
	for _, host := range []string{"claude", "codex", "pi"} {
		s := Session{ID: randomID("agt_"), Secret: randomID("") + randomID(""), Name: host, Host: host, Mesh: testMesh}
		file := filepath.Join(dir, host+".json")
		if err := saveSession(file, s); err != nil {
			t.Fatal(err)
		}
		c, err := dial(socketPath(dir))
		if err != nil {
			t.Fatal(err)
		}
		defer c.close()
		if err = c.call("ax.enroll", s, nil); err != nil {
			t.Fatal(err)
		}
		id := uuid()
		hook := object{"session_id": id, "hook_event_name": "SessionStart", "permission_mode": "default", "session_file": "/original repo/session.jsonl"}
		if err = Hook(dir, file, bytes.NewReader(raw(hook))); err != nil {
			t.Fatal(err)
		}
		saved, err := loadSession(file)
		if err != nil || saved.Native != id || !saved.Started {
			t.Fatalf("binding: %+v %v", saved, err)
		}
		if host == "pi" && saved.NativeFile != hook["session_file"] {
			t.Fatal("Pi lost the session file needed to resume from another repository")
		}
		hook["session_id"] = uuid()
		if err = Hook(dir, file, bytes.NewReader(raw(hook))); err == nil {
			t.Fatal("rebound an existing AX name to another conversation")
		}
	}
}

func TestBootstrapWaitsForDiscoveryAndSessionStart(t *testing.T) {
	var out bytes.Buffer
	dir := testDir(t)
	file := filepath.Join(dir, "session.json")
	s := Session{Host: "claude", Name: "api", Native: uuid()}
	if err := saveSession(file, s); err != nil {
		t.Fatal(err)
	}
	b := &bridge{session: s, file: file, out: &out, ctx: context.Background()}
	b.bootstrap()
	b.toolsListed = true
	b.bootstrap()
	if out.Len() != 0 {
		t.Fatal("wake before native SessionStart")
	}
	s.Started = true
	if err := saveSession(file, s); err != nil {
		t.Fatal(err)
	}
	b.bootstrap()
	if !strings.Contains(out.String(), "mcp__ax__list_agents") {
		t.Fatal("missing scoped setup tool")
	}
	size := out.Len()
	b.bootstrap()
	if out.Len() != size {
		t.Fatal("repeated setup wake")
	}
}

func TestBridgeHonorsLatestStartupBinding(t *testing.T) {
	for _, host := range []string{"claude", "codex"} {
		file := filepath.Join(testDir(t), "session.json")
		initial := Session{Host: host, Name: "api"}
		current := initial
		current.Native = uuid()
		// A stale saved Claude ID does not prove this launch's SessionStart ran.
		current.Started = host == "codex"
		if err := saveSession(file, current); err != nil {
			t.Fatal(err)
		}
		b := &bridge{session: initial, file: file}
		if err := b.bind(context.Background(), nil, raw(object{"threadId": uuid()})); err == nil {
			t.Fatalf("%s accepted an unconfirmed or mismatched conversation", host)
		}
		saved, err := loadSession(file)
		if err != nil || saved.Native != current.Native {
			t.Fatalf("binding overwritten: %+v %v", saved, err)
		}
	}
}

func TestHostSurvivesMessagingFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	in, input, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	defer input.Close()
	logs, err := os.CreateTemp(t.TempDir(), "host-stderr-")
	if err != nil {
		t.Fatal(err)
	}
	defer logs.Close()
	var output bytes.Buffer
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "read line; printf '%s' \"$line\"; exit 7")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, &output, logs
	startupErrors := make(chan error)
	done := make(chan error, 1)
	go func() { done <- runHost(cmd, startupErrors) }()
	select {
	case startupErrors <- errors.New("broker unavailable"):
	case err := <-done:
		t.Fatalf("host exited before failure injection: %v", err)
	case <-ctx.Done():
		t.Fatal("host did not start")
	}
	close(startupErrors)
	if _, err := input.WriteString("still coding\n"); err != nil {
		t.Fatal(err)
	}
	err = <-done
	logData, readErr := os.ReadFile(logs.Name())
	if readErr != nil {
		t.Fatal(readErr)
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 7 || output.String() != "still coding" || !strings.Contains(string(logData), "AX messaging unavailable: broker unavailable") {
		t.Fatalf("messaging failure interrupted host: err=%v output=%q logs=%q", err, output.String(), logData)
	}
}

func TestHostKeepsNativeExitStatus(t *testing.T) {
	err := runHost(exec.Command("/bin/sh", "-c", "exit 7"), nil)
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 7 {
		t.Fatalf("native exit status changed: %v", err)
	}
}
