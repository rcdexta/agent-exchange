package ax

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gorilla/websocket"
)

func TestCodexBypassKeepsNativeArguments(t *testing.T) {
	flag := "--dangerously-bypass-approvals-and-sandbox"
	for _, tc := range []struct {
		args, want []string
		bypass     bool
	}{
		{[]string{flag, "resume"}, []string{"resume"}, true},
		{[]string{"resume", "id", flag, "prompt"}, []string{"resume", "id", "prompt"}, true},
		{[]string{"--yolo", "resume", "--last"}, []string{"resume", "--last"}, true},
		{[]string{"resume", "--", flag}, []string{"resume", "--", flag}, false},
		{[]string{"-c", flag}, []string{"-c", flag}, false},
		{[]string{"-m", flag}, []string{"-m", flag}, false},
		{[]string{"resume", "id", "-s", "read-only"}, []string{"resume", "id", "-s", "read-only"}, false},
	} {
		got, bypass, err := codexBypassArgs(tc.args)
		if err != nil || bypass != tc.bypass || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q: args=%q bypass=%v err=%v", tc.args, got, bypass, err)
		}
	}
	for _, conflict := range []string{"-anever", "--ask-for-approval=never", "--approve-for-me", "--not-so-yolo", "-sread-only", "--sandbox=read-only"} {
		if _, _, err := codexBypassArgs([]string{flag, "resume", conflict}); err == nil {
			t.Fatalf("silently overrode conflicting permission option %s", conflict)
		}
	}
}

func TestCodexBypassOnlyChangesSessionPermissions(t *testing.T) {
	for _, method := range []string{"thread/start", "thread/resume", "thread/fork", "thread/queue/add", "turn/start", "initialize", "thread/settings/update", ""} {
		data := raw(object{"id": "same-id", "method": method, "extra": object{"keep": true}, "params": object{"threadId": "original", "model": "chosen", "sandbox": "read-only", "permissions": "restricted", "config": object{"future": true}}})
		got, err := codexBypassRequest(data)
		if err != nil {
			t.Fatal(err)
		}
		if method != "thread/start" && method != "thread/resume" && method != "thread/fork" {
			if string(got) != string(data) {
				t.Fatalf("rewrote unrelated frame %s", method)
			}
			continue
		}
		var want object
		json.Unmarshal(data, &want)
		params := want["params"].(map[string]any)
		params["approvalPolicy"], params["sandbox"] = "never", "danger-full-access"
		delete(params, "permissions")
		if string(got) != string(raw(want)) {
			t.Fatalf("lost session fields: %s", got)
		}
	}
	for _, data := range []string{`{`, `{"method":"thread/resume","params":null}`} {
		if _, err := codexBypassRequest([]byte(data)); err == nil {
			t.Fatal("invalid session request accepted")
		}
	}
}

func TestCodexBypassSocketForwardsAndCloses(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ax-permissions-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "backend.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var p packet
		if conn.ReadJSON(&p) == nil {
			conn.WriteJSON(object{"id": p.ID, "result": json.RawMessage(p.Params)})
		}
	})}
	go server.Serve(listener)
	defer server.Close()
	remote, cleanup, err := codexBypassSocket(context.Background(), dir, "unix://"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	conn, err := dialCodex(context.Background(), remote)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var got object
	if err := codexCall(conn, 7, "thread/resume", object{"threadId": "original"}, &got); err != nil || got["approvalPolicy"] != "never" || got["sandbox"] != "danger-full-access" || got["threadId"] != "original" {
		t.Fatalf("native resume did not receive permissions: %+v %v", got, err)
	}
	cleanup()
	if _, err = dialCodex(context.Background(), remote); err == nil {
		t.Fatal("permission adapter remained available after cleanup")
	}
}
