package ax

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPiPreservesArgumentsAndSavedConversation(t *testing.T) {
	dir := testDir(t)
	s := Session{Host: "pi", Native: uuid(), NativeFile: "/original repo/session.jsonl"}
	for _, native := range [][]string{nil, {"-r"}, {"--session", "another.jsonl", "--", "literal @file /command"}, {"--help"}} {
		args, env, cleanup, err := preparePi(context.Background(), dir, "session.json", "pi", s, native, []string{"EXISTING=value"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		cleanup()
		if nativeHelp(native) {
			if !reflect.DeepEqual(args, native) || len(env) != 1 {
				t.Fatal("help changed")
			}
			continue
		}
		var path string
		var remaining []string
		for i := 0; i < len(args); i++ {
			if args[i] == "-e" {
				i++
				path = args[i]
			} else {
				remaining = append(remaining, args[i])
			}
		}
		want := native
		if len(want) == 0 {
			want = []string{"--session", s.NativeFile}
		}
		if !reflect.DeepEqual(remaining, want) {
			t.Fatalf("native syntax changed: %q", args)
		}
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, piExtension) {
			t.Fatalf("extension: %v", err)
		}
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0600 || env[0] != "EXISTING=value" {
			t.Fatal("configuration not private or native environment lost")
		}
	}
}

func TestPiNativeExecPreservesPIDAndNameLock(t *testing.T) {
	if os.Getenv("AX_TEST_PI_EXEC") == "1" {
		lock, err := lockFile(os.Getenv("AX_TEST_LOCK"))
		if err == nil {
			err = execNative(os.Getenv("AX_TEST_NODE"), []string{"-e", `console.log(process.pid); process.stdin.once('data', () => { console.log('native still works'); process.exit(7); });`}, os.Environ(), lock)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the Pi adapter contract tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	lockPath := filepath.Join(testDir(t), "pi.lock")
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPiNativeExecPreservesPIDAndNameLock$")
	cmd.Env = append(os.Environ(), "AX_TEST_PI_EXEC=1", "AX_TEST_NODE="+node, "AX_TEST_LOCK="+lockPath)
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	scan := bufio.NewScanner(out)
	if !scan.Scan() || scan.Text() != strconv.Itoa(cmd.Process.Pid) {
		t.Fatal("AX remains the native harness parent")
	}
	if other, err := lockFile(lockPath); err == nil {
		other.Close()
		t.Fatal("native process lost the AX name lock")
	}
	fmt.Fprintln(in, "continue")
	if !scan.Scan() || scan.Text() != "native still works" {
		t.Fatal("native input unavailable")
	}
	err = cmd.Wait()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 7 {
		t.Fatalf("native exit status lost: %v", err)
	}
	lock, err := lockFile(lockPath)
	if err != nil {
		t.Fatal("name lock survived native process exit")
	}
	lock.Close()
}

func TestPiNotificationIncludesNativeIdentity(t *testing.T) {
	var out bytes.Buffer
	s := Session{Host: "pi", Native: uuid()}
	b := &bridge{out: &out}
	text := "literal /command @file\n$(not a shell command)"
	if err := b.notify(s, text, object{"message_id": "msg_test"}); err != nil {
		t.Fatal(err)
	}
	var p struct {
		Method string
		Params struct {
			Content string
			Native  string `json:"native_session_id"`
		}
	}
	if err := json.Unmarshal(out.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Method != "notifications/ax/message" || p.Params.Native != s.Native || p.Params.Content != text {
		t.Fatal("notification changed peer content or identity")
	}
}

func TestPiExtensionContract(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the Pi adapter contract tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/pi_host.mjs")
	cmd.Env = append(os.Environ(), "AX_TEST_PI_TOOLS="+string(raw(toolList())))
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "Pi extension contract passed") {
		t.Fatalf("%v\n%s", err, output)
	}
}

func TestPiNativePermissionIsScopedToPi(t *testing.T) {
	for _, host := range []string{"pi", "claude", "codex", "grok", "opencode"} {
		p := &peer{Agent: Agent{Host: host, Permission: "native"}}
		if safe(p) != (host == "pi") {
			t.Fatalf("native permission changed policy for %s", host)
		}
	}
}
