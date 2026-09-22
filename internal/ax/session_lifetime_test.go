package ax

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func savedOwner(t *testing.T, dir string, s Session) (*os.File, string) {
	t.Helper()
	if err := privateDir(filepath.Join(dir, "sessions")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sessions", s.Name+".json")
	if err := saveSession(path, s); err != nil {
		t.Fatal(err)
	}
	lock, err := lockFile(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lock.Close() })
	return lock, path
}

func TestSessionExitPreservesDisconnectedOwnerAndResume(t *testing.T) {
	b, dir := localBroker(t)
	s, c, _ := endpoint(t, b, "api", testMesh)
	lock, path := savedOwner(t, dir, s)
	b.disconnect(c)
	b.reapSessions(time.Now().Add(time.Minute))
	if len(b.list()) != 1 {
		t.Fatal("bridge disconnect retired a running harness")
	}
	lock.Close()
	b.reapSessions(time.Now().Add(2 * time.Minute))
	if len(b.list()) != 0 || len(b.peers) != 0 {
		t.Fatal("exited session retained in discovery or active map")
	}
	saved, err := loadSession(path)
	if err != nil || saved.ID != s.ID || saved.Secret != s.Secret || saved.Native != s.Native {
		t.Fatalf("saved conversation changed: %+v %v", saved, err)
	}
	archived, err := b.resolve("api")
	if err != nil || agentSnapshot(archived).State != "exited" {
		t.Fatalf("missing archived identity: %+v %v", archived, err)
	}
	bad := s
	bad.Secret = strings.Repeat("x", 64)
	if _, err := b.request(&serverConn{}, "ax.enroll", raw(bad)); err == nil {
		t.Fatal("archived credential could be replaced")
	}
	if _, err := b.request(&serverConn{}, "ax.connect", raw(object{"version": "1", "agent_id": s.ID, "secret": s.Secret})); err == nil {
		t.Fatal("orphaned bridge resurrected exited session")
	}
	savedOwner(t, dir, s)
	request(t, b, &serverConn{}, "ax.enroll", s)
	if len(b.list()) != 1 || b.peers[s.ID].Native != s.Native || b.peers[s.ID].epoch != archived.epoch {
		t.Fatal("resume lost binding or lease history")
	}
}

func TestSessionExitRestartAndArchiveCapacity(t *testing.T) {
	b, dir := localBroker(t)
	alive, _, _ := endpoint(t, b, "alive", testMesh)
	savedOwner(t, dir, alive)
	ended, _, _ := endpoint(t, b, "ended", testMesh)
	lock, _ := savedOwner(t, dir, ended)
	lock.Close()
	b.db.Close()
	b, err := openBroker(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer b.db.Close()
	if len(b.list()) != 1 || b.list()[0].ID != alive.ID {
		t.Fatalf("restart retained exited peers or lost live owner: %+v", b.list())
	}
	for i := 0; i < 260; i++ {
		s := Session{ID: randomID("agt_"), Secret: strings.Repeat("s", 64), Name: fmt.Sprintf("failed%d", i), Host: "codex", Mesh: testMesh}
		request(t, b, &serverConn{}, "ax.enroll", s)
		b.reapSession(b.peers[s.ID], time.Now().Add(time.Minute))
	}
	if len(b.peers) != 1 {
		t.Fatalf("archived records occupy active slots: %d", len(b.peers))
	}
	b.db.Close()
	b, err = openBroker(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer b.db.Close()
	if len(b.peers) != 1 {
		t.Fatal("archived records reloaded into dispatch map")
	}
}

func TestSessionExitRetainsMailRepliesAndIdempotency(t *testing.T) {
	b, dir := localBroker(t)
	sender, from, _ := endpoint(t, b, "web", testMesh)
	recipient, to, wire := endpoint(t, b, "api", testMesh)
	owner, _ := savedOwner(t, dir, sender)
	args := object{"target": "api", "text": "please review", "client_message_id": "task"}
	task := request(t, b, from, "ax.send", args).(Message)
	b.disconnect(from)
	owner.Close()
	b.reapSession(b.peers[sender.ID], time.Now())
	// Mail already sent must still reach its recipient after the sender exits.
	offered := make(chan packet, 1)
	go func() { p, _ := readFrame(wire); offered <- p }()
	b.dispatch()
	select {
	case p := <-offered:
		if p.Method != "ax.delivery.offer" {
			t.Fatal(p)
		}
	case <-time.After(time.Second):
		t.Fatal("sender exit stranded queued work")
	}
	request(t, b, to, "ax.receipt", object{"message_id": task.ID, "receipt": "channel_written"})
	page, err := b.pending(b.peers[recipient.ID], 0)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].Preview != "please review" {
		t.Fatalf("archived sender lost recovery preview: %+v %v", page, err)
	}
	replyArgs := object{"message_id": task.ID, "text": "review complete", "client_message_id": "review"}
	reply := request(t, b, to, "ax.reply", replyArgs).(Message)
	if reply.Receipt.Recipient.State != "exited" || !strings.Contains(reply.Receipt.Evidence, "next AX launch") {
		t.Fatal(reply.Receipt)
	}
	again := request(t, b, to, "ax.reply", replyArgs).(Message)
	if again.ID != reply.ID {
		t.Fatal("retry created another reply")
	}
	inbox, err := b.inbox("web")
	if err != nil || len(inbox) != 2 || inbox[0].Recipient != "web" {
		t.Fatalf("archived inbox inaccessible: %+v %v", inbox, err)
	}
	if len(b.list()) != 1 {
		t.Fatal("sending mail restored exited sender to discovery")
	}
	savedOwner(t, dir, sender)
	request(t, b, &serverConn{}, "ax.enroll", sender)
	x, y := net.Pipe()
	defer x.Close()
	defer y.Close()
	resumed := &serverConn{Conn: x}
	request(t, b, resumed, "ax.connect", object{"version": "1", "agent_id": sender.ID, "secret": sender.Secret})
	request(t, b, resumed, "ax.presence", object{"native_session_id": sender.Native, "permission_mode": "read-only", "state": "ready"})
	request(t, b, resumed, "ax.ready", object{})
	retry := request(t, b, resumed, "ax.send", args).(Message)
	if retry.ID != task.ID {
		t.Fatal("resume lost idempotency")
	}
	go func() { p, _ := readFrame(y); offered <- p }()
	b.dispatch()
	select {
	case p := <-offered:
		if p.Method != "ax.delivery.offer" || !strings.Contains(string(p.Params), reply.ID) {
			t.Fatal(p)
		}
	case <-time.After(time.Second):
		t.Fatal("reply not offered after resume")
	}
	next := request(t, b, to, "ax.send", object{"target": "web", "text": "follow-up", "client_message_id": "followup"}).(Message)
	if next.Seq != reply.Seq+1 {
		t.Fatal("resume reused a recipient sequence")
	}
}

func TestSessionExitExpiresMailWithoutResurrectingRecipient(t *testing.T) {
	b, dir := localBroker(t)
	sender, from, _ := endpoint(t, b, "web", testMesh)
	recipient, to, _ := endpoint(t, b, "api", testMesh)
	lock, _ := savedOwner(t, dir, recipient)
	b.disconnect(to)
	lock.Close()
	b.reapSession(b.peers[recipient.ID], time.Now())
	args := object{"target": "api", "text": "time sensitive", "client_message_id": "expiring", "ttl_seconds": 1}
	sent := request(t, b, from, "ax.send", args).(Message)
	b.cleanup(time.Now().Add(2 * time.Second))
	msg, err := b.message(sent.ID)
	if err != nil || msg.State != "expired" {
		t.Fatalf("archived mail never expired: %+v %v", msg, err)
	}
	notices, err := b.notices(sender.ID)
	if err != nil || len(notices) != 1 || notices[0].MessageID != sent.ID {
		t.Fatalf("expiration notice lost: %+v %v", notices, err)
	}
	again := request(t, b, from, "ax.send", args).(Message)
	if again.ID != sent.ID || again.State != "expired" || again.Receipt.Recipient.State != "exited" {
		t.Fatal("retry revived expired work")
	}
}

func TestSessionExitWaitsForNativeLockThenReapsCrash(t *testing.T) {
	b, dir := localBroker(t)
	s, _, _ := endpoint(t, b, "native", testMesh)
	lock, path := savedOwner(t, dir, s)
	cmd := exec.Command("/bin/sh", "-c", "read line")
	cmd.ExtraFiles = []*os.File{lock}
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	lock.Close() // Simulate losing the launcher while its native child survives.
	b.reapSession(b.peers[s.ID], time.Now())
	if b.peers[s.ID] == nil {
		t.Fatal("launcher exit retired its surviving native child")
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	b.reapSession(b.peers[s.ID], time.Now())
	if b.peers[s.ID] != nil {
		t.Fatal("native crash retained active registration")
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("crash cleanup deleted saved identity")
	}
}
