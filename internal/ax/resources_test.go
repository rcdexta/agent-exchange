package ax

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestWatchdogPausesBeforeStoppingOnlyTheHotHelper(t *testing.T) {
	dir := testDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)
	cpu := make(chan time.Duration)
	pauses := make(chan struct{}, 4)
	stops := make(chan struct{}, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchResourceSamples(ctx, dir, func() { pauses <- struct{}{} }, func() { stops <- struct{}{} }, ticks, func() (time.Duration, error) { return <-cpu, nil })
	}()
	start := time.Unix(1000, 0)
	feed := func(second int, usage time.Duration) {
		ticks <- start.Add(time.Duration(second) * time.Second)
		cpu <- usage
	}
	for second := 0; second <= 10; second++ {
		feed(second, time.Duration(second+1)*600*time.Millisecond)
	}
	select {
	case <-pauses:
	case <-time.After(time.Second):
		t.Fatal("watchdog did not pause messaging")
	}
	select {
	case <-stops:
		t.Fatal("stopped helper before trying a pause")
	default:
	}
	feed(12, 7800*time.Millisecond)
	select {
	case <-stops:
	case <-time.After(time.Second):
		t.Fatal("continuing CPU consumption did not stop hot helper")
	}
	feed(14, 7800*time.Millisecond)
	// Sending another tick waits until the preceding sample has been processed.
	feed(16, 7800*time.Millisecond)
	select {
	case <-stops:
		t.Fatal("idle helper was stopped")
	default:
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchdog ignored cancellation")
	}
}

func TestSharedCPUBudgetAndCooldown(t *testing.T) {
	var s resourceBudget
	start := time.Unix(1000, 0)
	for second := 0; second <= 10; second++ {
		now := start.Add(time.Duration(second) * time.Second)
		// Two helpers are individually below 50%, but exceed it collectively.
		first := s.add(now, 300*time.Millisecond)
		secondReport := s.add(now, 300*time.Millisecond)
		if second < 10 && (first || secondReport) {
			t.Fatal("tripped during startup allowance")
		}
		if second == 10 && !secondReport {
			t.Fatal("aggregate CPU was not limited")
		}
	}
	until := s.PausedUntil
	if !s.add(time.UnixMilli(until-1), 0) {
		t.Fatal("cooldown ended early")
	}
	if s.add(time.UnixMilli(until), 0) {
		t.Fatal("cooldown did not expire")
	}
	if s.PausedUntil != until {
		t.Fatal("reading cooldown extended it")
	}
	for second := 1; second <= 30; second++ {
		if s.add(time.UnixMilli(until).Add(time.Duration(second)*time.Second), 50*time.Millisecond) {
			t.Fatal("low CPU retriggered")
		}
	}
}

func TestResourceCooldownSurvivesProcessRestart(t *testing.T) {
	if dir := os.Getenv("AX_TEST_RESOURCE_CHILD"); dir != "" {
		var paused *resourcePaused
		if !errors.As(resourcePause(dir, time.Now()), &paused) {
			os.Exit(2)
		}
		if err := ensureBroker(dir); !errors.As(err, &paused) {
			os.Exit(3)
		}
		os.Exit(0)
	}
	dir := testDir(t)
	start := time.Now().Add(-11 * time.Second)
	for second := 0; second <= 11; second++ {
		if _, err := reportCPU(dir, start.Add(time.Duration(second)*time.Second), 600*time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestResourceCooldownSurvivesProcessRestart$")
	cmd.Env = append(os.Environ(), "AX_TEST_RESOURCE_CHILD="+dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("restart ignored cooldown: %v %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(dir, "broker.log")); !os.IsNotExist(err) {
		t.Fatal("paused startup touched broker log")
	}
	info, err := os.Stat(filepath.Join(dir, "resource-budget.json"))
	if err != nil || info.Mode().Perm() != 0600 || info.Size() > 4096 {
		t.Fatal("unbounded or public budget state", info, err)
	}
}

func TestBudgetContentionDoesNotBlockMessaging(t *testing.T) {
	dir := testDir(t)
	lock, err := lockFile(filepath.Join(dir, "resource-budget.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	done := make(chan error, 1)
	go func() { _, err := reportCPU(dir, time.Now(), time.Second); done <- err }()
	select {
	case err := <-done:
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("resource monitor waited on a held lock")
	}
}

func TestBrokerOwnerPreventsAnotherSpawn(t *testing.T) {
	dir := testDir(t)
	owner, err := lockFile(filepath.Join(dir, "broker.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err = ensureBroker(dir); err == nil {
		t.Fatal("missing socket treated as healthy")
	}
	if _, err = os.Stat(filepath.Join(dir, "broker.log")); !os.IsNotExist(err) {
		t.Fatal("attempted another broker while owner was alive")
	}
	if other, err := lockFile(filepath.Join(dir, "broker.lock")); err == nil {
		other.Close()
		t.Fatal("replaced live lock inode")
	}
}

func TestLifecycleSkipsOnlyUnchangedWrites(t *testing.T) {
	b, _ := localBroker(t)
	s, _, _ := endpoint(t, b, "web", testMesh)
	args := object{"agent_id": s.ID, "secret": s.Secret, "native_session_id": s.Native, "permission_mode": "read-only", "state": "ready"}
	var before, after int
	b.db.QueryRow("SELECT total_changes()").Scan(&before)
	for i := 0; i < 20; i++ {
		request(t, b, &serverConn{}, "ax.lifecycle", args)
	}
	b.db.QueryRow("SELECT total_changes()").Scan(&after)
	if after != before {
		t.Fatalf("unchanged hooks wrote %d rows", after-before)
	}
	args["state"] = "blocked"
	request(t, b, &serverConn{}, "ax.lifecycle", args)
	args["state"] = "busy"
	request(t, b, &serverConn{}, "ax.lifecycle", args)
	b.db.QueryRow("SELECT total_changes()").Scan(&after)
	if after-before != 2 || b.peers[s.ID].State != "busy" {
		t.Fatal("lost permission blocking or recovery")
	}
	for _, method := range []string{"ax.ping", "ax.heartbeat", "ax.inbox", "ax.status"} {
		if dispatchAfter(method) {
			t.Fatal("read request dispatched", method)
		}
	}
	for _, method := range []string{"ax.send", "ax.receipt", "ax.ready", "ax.lifecycle"} {
		if !dispatchAfter(method) {
			t.Fatal("delivery transition stopped dispatching", method)
		}
	}
}

func TestRetentionIsBatchedAndPreservesPendingMail(t *testing.T) {
	b, _ := localBroker(t)
	from, _, _ := endpoint(t, b, "web", testMesh)
	to, _, _ := endpoint(t, b, "api", testMesh)
	old := time.Now().Add(-8 * 24 * time.Hour).UnixMilli()
	_, err := b.db.Exec(`WITH RECURSIVE n(i) AS (VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i<600)
 INSERT INTO messages SELECT 'old_'||i,?,?, 'key_'||i,'hash',i,
 CASE WHEN i=600 THEN 'delivery_uncertain' ELSE 'acknowledged' END,'{}',?,? FROM n`, from.ID, to.ID, old, old+1)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	b.cleanup(now)
	b.cleanup(now.Add(time.Second))
	var count int
	b.db.QueryRow("SELECT count(*) FROM messages").Scan(&count)
	if count != 344 {
		t.Fatal("unbounded or repeated retention", count)
	}
	b.cleanup(now.Add(time.Minute))
	b.cleanup(now.Add(2 * time.Minute))
	b.db.QueryRow("SELECT count(*) FROM messages").Scan(&count)
	if count != 1 {
		t.Fatal("pending delivery was deleted", count)
	}
}
