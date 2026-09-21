package ax

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestCodexBindsBeforeFirstTurnThroughNativeAPI(t *testing.T) {
	dir := startTestServer(t)
	file := filepath.Join(dir, "session.json")
	s := Session{ID: randomID("agt_"), Secret: randomID("") + randomID(""), Name: "automatic", Host: "codex", Mesh: testMesh}
	if err := saveSession(file, s); err != nil {
		t.Fatal(err)
	}
	broker, err := dial(socketPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer broker.close()
	if err = broker.call("ax.enroll", s, nil); err != nil {
		t.Fatal(err)
	}
	native, child := uuid(), uuid()
	lists := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		for {
			var request packet
			if conn.ReadJSON(&request) != nil {
				return
			}
			var result any
			switch request.Method {
			case "thread/loaded/list":
				lists++
				ids := []string{}
				if lists > 1 {
					ids = []string{child, native}
				}
				result = object{"data": ids}
			case "thread/read":
				var args struct {
					ID    string `json:"threadId"`
					Turns bool   `json:"includeTurns"`
				}
				json.Unmarshal(request.Params, &args)
				if args.Turns {
					t.Error("startup requested transcript history")
				}
				source := any("cli")
				if args.ID == child {
					source = object{"subAgent": object{}}
				}
				result = object{"thread": object{"id": args.ID, "source": source}}
			default:
				t.Errorf("unexpected startup RPC %s", request.Method)
				return
			}
			// Native notifications can arrive between an RPC request and its response.
			conn.WriteJSON(object{"method": "thread/status/changed", "params": object{}})
			conn.WriteJSON(object{"id": request.ID, "result": result})
		}
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = bindCodex(context.Background(), conn, dir, file); err != nil {
		t.Fatal(err)
	}
	saved, err := loadSession(file)
	if err != nil || saved.Native != native || !saved.Started {
		t.Fatalf("startup binding: %+v %v", saved, err)
	}
	var agents []Agent
	if err = broker.call("ax.inspect", object{}, &agents); err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].Online {
		t.Fatalf("native binding bypassed MCP readiness: %+v", agents)
	}
	// The native picker can select a different root for an already-bound name.
	// That must fail without replacing the saved identity or activating delivery.
	saved.Started = false
	if err = saveSession(file, saved); err != nil {
		t.Fatal(err)
	}
	native = uuid()
	err = bindCodex(context.Background(), conn, dir, file)
	if err == nil || !strings.Contains(err.Error(), "ax codex --name automatic") || !strings.Contains(err.Error(), saved.Native) || !strings.Contains(err.Error(), native) {
		t.Fatalf("missing mismatch recovery: %v", err)
	}
	after, err := loadSession(file)
	if err != nil || after.Native != saved.Native || after.Started {
		t.Fatalf("wrong picker selection changed binding: %+v %v", after, err)
	}
}

func TestNamedSessionPathRetainsLegacyIdentity(t *testing.T) {
	dir := testDir(t)
	legacy := filepath.Join(dir, testMesh+"_api.json")
	if err := saveSession(legacy, Session{Name: "api"}); err != nil {
		t.Fatal(err)
	}
	path, err := namedSessionPath(dir, "api")
	if err != nil || path != legacy {
		t.Fatalf("legacy identity: %s %v", path, err)
	}
	// An underscore in a new global name must not look like a legacy prefix.
	if err := saveSession(filepath.Join(dir, "other_web.json"), Session{Name: "other_web"}); err != nil {
		t.Fatal(err)
	}
	path, err = namedSessionPath(dir, "web")
	if err != nil || path != filepath.Join(dir, "web.json") {
		t.Fatalf("global name: %s %v", path, err)
	}
	if err := saveSession(filepath.Join(dir, "api.json"), Session{Name: "api"}); err != nil {
		t.Fatal(err)
	}
	if _, err = namedSessionPath(dir, "api"); err == nil {
		t.Fatal("ambiguous legacy name accepted")
	}
}

func TestCodexNativeOptionBoundary(t *testing.T) {
	if codexHelp([]string{"resume", "session", "--", "--help"}) {
		t.Fatal("literal prompt disabled automatic startup")
	}
	if !codexHelp([]string{"resume", "--help"}) {
		t.Fatal("native help started a backend")
	}
	args := []string{"-c", "model=\"a\"", "resume", "session", "--config=model_reasoning_effort=\"low\"", "--", "-cignored"}
	configs := codexConfigOverrides(args)
	if len(configs) != 2 || configs[0] != "model=\"a\"" || configs[1] != "model_reasoning_effort=\"low\"" {
		t.Fatalf("native configuration changed: %q", configs)
	}
}
