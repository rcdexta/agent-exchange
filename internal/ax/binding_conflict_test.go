package ax

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResumeBindingConflictIsDurableAndActionable(t *testing.T) {
	for _, host := range []string{"claude", "codex"} {
		t.Run(host, func(t *testing.T) {
			dir := startTestServer(t)
			original, selected := uuid(), uuid()
			s := Session{ID: randomID("agt_"), Secret: randomID("") + randomID(""), Name: "saved", Host: host, Mesh: testMesh, Native: original}
			file := filepath.Join(dir, "session.json")
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
			sender := connectDiscoveryPeer(t, dir, Session{ID: randomID("agt_"), Secret: randomID("") + randomID(""), Name: "sender", Host: "codex", Mesh: testMesh, Native: uuid()})
			var mail Message
			if err = sender.call("ax.send", object{"target": s.Name, "text": "original mail", "client_message_id": "durable"}, &mail); err != nil {
				t.Fatal(err)
			}
			hook := func(native, event string) error {
				return Hook(dir, file, bytes.NewReader(raw(object{"hook_event_name": event, "source": "resume", "session_id": native, "permission_mode": "default"})))
			}
			conflict := hook(selected, "SessionStart")
			if conflict == nil || !strings.Contains(conflict.Error(), original) || !strings.Contains(conflict.Error(), selected) || !strings.Contains(conflict.Error(), "unused AX name") {
				t.Fatalf("missing recovery detail: %v", conflict)
			}
			// Late hooks from either conversation cannot erase this launch's conflict.
			for _, id := range []string{original, selected} {
				if hook(id, "SessionStart") == nil || hook(id, "Stop") == nil {
					t.Fatal("late hook cleared conflict")
				}
			}
			after, err := loadSession(file)
			if err != nil || after.Native != original || after.ID != s.ID || after.Secret != s.Secret || after.Started || after.BindingError != conflict.Error() {
				t.Fatalf("saved state changed incorrectly: %+v %v", after, err)
			}
			if err = c.call("ax.connect", object{"version": "1", "agent_id": s.ID, "secret": s.Secret}, nil); err != nil {
				t.Fatal(err)
			}
			if err = c.call("ax.ready", object{}, nil); err == nil {
				t.Fatal("conflict activated delivery")
			}
			agents := discoveryList(t, c)
			if a := agents[s.ID]; a.State != "binding-conflict" || a.BindingError != conflict.Error() {
				t.Fatalf("hidden conflict: %+v", a)
			}
			var out bytes.Buffer
			b := &bridge{session: s, file: file, dir: dir, c: c, ctx: context.Background(), out: &out, toolsListed: true}
			b.bootstrap()
			if b.active || out.Len() != 0 {
				t.Fatal("conflicted session was bootstrapped")
			}
			if err = b.toolCall(context.Background(), nil, "ax.list", object{}, nil); err == nil || err.Error() != conflict.Error() {
				t.Fatalf("generic tool error: %v", err)
			}
			doctorBroker(context.Background(), dir, &out)
			if !strings.Contains(out.String(), conflict.Error()) {
				t.Fatal("doctor hid recovery instructions")
			}
			var status struct{ Message Message }
			if err = sender.call("ax.status", object{"message_id": mail.ID}, &status); err != nil {
				t.Fatal(err)
			}
			if status.Message.State != "queued" || status.Message.Recipient != s.ID || status.Message.Text != "original mail" {
				t.Fatalf("mail moved or replayed: %+v", status)
			}
			// A deliberate relaunch resets the error while retaining the same identity.
			after.BindingError = ""
			if err = saveSession(file, after); err != nil {
				t.Fatal(err)
			}
			admin, err := dial(socketPath(dir))
			if err != nil {
				t.Fatal(err)
			}
			defer admin.close()
			if err = admin.call("ax.enroll", after, nil); err != nil {
				t.Fatal(err)
			}
			if err = hook(original, "SessionStart"); err != nil {
				t.Fatal(err)
			}
			if err = c.call("ax.ready", object{}, nil); err != nil {
				t.Fatal(err)
			}
			if a := discoveryList(t, c)[s.ID]; a.BindingError != "" || a.Native != original {
				t.Fatalf("recovery changed identity: %+v", a)
			}
		})
	}
}

func TestExplicitResumeConflictDoesNotEnrollOrLaunch(t *testing.T) {
	for _, host := range []string{"claude", "codex"} {
		t.Run(host, func(t *testing.T) {
			dir := testDir(t)
			bin := filepath.Join(dir, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bin, host), []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			sessions := filepath.Join(dir, "sessions")
			if err := os.Mkdir(sessions, 0700); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(sessions, "saved.json")
			s := Session{ID: randomID("agt_"), Secret: randomID("") + randomID(""), Name: "saved", Host: host, Mesh: testMesh, Native: uuid()}
			if err := saveSession(file, s); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(file)
			flag := "-r"
			if host == "codex" {
				flag = "resume"
			}
			err := Launch(context.Background(), dir, host, []string{"-n", "saved", flag, uuid()})
			if err == nil || !strings.Contains(err.Error(), "belongs to conversation") {
				t.Fatalf("preflight: %v", err)
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("preflight changed saved session")
			}
			if _, err = os.Stat(socketPath(dir)); !os.IsNotExist(err) {
				t.Fatal("preflight started broker")
			}
		})
	}
}

func TestExplicitResumeIDPreservesNativeNamesAndPrompts(t *testing.T) {
	id := uuid()
	for _, tc := range []struct {
		host string
		args []string
		want string
	}{
		{"claude", []string{"-r", id}, id}, {"claude", []string{"--resume=" + id}, id}, {"codex", []string{"resume", id}, id},
		{"claude", []string{"-r", "session-name"}, ""}, {"codex", []string{"resume", "--last"}, ""},
		{"claude", []string{"--", "-r", id}, ""}, {"codex", []string{"-c", "resume", id}, ""},
	} {
		if got := explicitResumeID(tc.host, tc.args); got != tc.want {
			t.Fatalf("%v: %s", tc.args, got)
		}
	}
}

func TestProvisionalStartupNeverRebindsPersistedIdentity(t *testing.T) {
	dir := startTestServer(t)
	s := Session{ID: randomID("agt_"), Secret: randomID("") + randomID(""), Name: "fresh", Host: "claude", Mesh: testMesh}
	file := filepath.Join(dir, "session.json")
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
	native := uuid()
	first := object{"hook_event_name": "SessionStart", "source": "startup", "session_id": native, "transcript_path": filepath.Join(dir, "not-created.jsonl"), "permission_mode": "default"}
	if err = Hook(dir, file, bytes.NewReader(raw(first))); err != nil {
		t.Fatal(err)
	}
	first["source"], first["session_id"] = "resume", uuid()
	if err = Hook(dir, file, bytes.NewReader(raw(first))); err == nil {
		t.Fatal("provisional startup silently rebound")
	}
	saved, err := loadSession(file)
	if err != nil || saved.Native != native || saved.BindingError == "" {
		t.Fatalf("lost first binding: %+v %v", saved, err)
	}
}

func TestBrokerKeepsBindingConflictAcrossRestart(t *testing.T) {
	b, dir := localBroker(t)
	s, c, _ := endpoint(t, b, "saved", testMesh)
	message := bindingConflict(s, uuid()).Error()
	request(t, b, &serverConn{}, "ax.lifecycle", object{"agent_id": s.ID, "secret": s.Secret, "native_session_id": s.Native, "binding_error": message, "state": "blocked"})
	request(t, b, &serverConn{}, "ax.lifecycle", object{"agent_id": s.ID, "secret": s.Secret, "native_session_id": s.Native, "state": "ready"})
	if b.peers[s.ID].State != "blocked" {
		t.Fatal("late lifecycle cleared broker guard")
	}
	if _, err := b.request(c, "ax.presence", raw(object{"state": "ready"})); err == nil {
		t.Fatal("MCP presence cleared broker guard")
	}
	b.db.Close()
	reopened, err := openBroker(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.db.Close()
	a := reopened.list()[0]
	if a.State != "binding-conflict" || a.BindingError != message || a.Native != s.Native {
		t.Fatalf("conflict lost on restart: %+v", a)
	}
}
