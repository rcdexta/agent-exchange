package ax

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestRequestDeadlineIncludesWriterAndResponse(t *testing.T) {
	for _, phase := range []string{"writer", "write", "response"} {
		t.Run(phase, func(t *testing.T) {
			x, y := net.Pipe()
			c := newClient(x)
			defer c.close()
			defer y.Close()
			if phase == "writer" {
				c.writing <- struct{}{}
				defer func() { <-c.writing }()
			}
			if phase == "response" {
				go readFrame(y)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
			defer cancel()
			start := time.Now()
			err := c.callContext(ctx, "ax.list", object{}, nil)
			var transport *transportError
			if !errors.As(err, &transport) || transport.submitted != (phase != "writer") {
				t.Fatalf("wrong submission evidence: %v", err)
			}
			if time.Since(start) > time.Second {
				t.Fatal("request exceeded its deadline")
			}
			if phase != "writer" {
				select {
				case <-c.done:
				default:
					t.Fatal("timed out connection was not retired")
				}
			}
		})
	}
}

func TestClosedClientDoesNotSubmit(t *testing.T) {
	x, y := net.Pipe()
	defer y.Close()
	c := newClient(x)
	c.close()
	err := c.call("ax.send", object{}, nil)
	var transport *transportError
	if !errors.As(err, &transport) || transport.submitted {
		t.Fatalf("closed connection submitted a request: %v", err)
	}
}

func TestSendRecoversLostResponseWithoutDuplicate(t *testing.T) {
	store, dir := localBroker(t)
	s, owner, wire := endpoint(t, store, "web", testMesh)
	_, _, _ = endpoint(t, store, "api", testMesh)
	// Use an already-bound Claude fixture so the test does not depend on a
	// running harness. All tool operations still pass through the real broker.
	s.Host, s.Started = "claude", true
	file := filepath.Join(dir, "session.json")
	if err := saveSession(file, s); err != nil {
		t.Fatal(err)
	}
	bridge := &bridge{ctx: context.Background(), dir: dir, file: file, session: s, c: newClient(wire)}
	defer func() {
		bridge.mu.Lock()
		c := bridge.c
		bridge.mu.Unlock()
		if c != nil {
			c.close()
		}
	}()
	done := make(chan error, 1)
	go func() {
		dropped := false
		for {
			p, err := readFrame(owner)
			if err != nil {
				done <- nil
				return
			}
			store.mu.Lock()
			value, err := store.request(owner, p.Method, p.Params)
			store.mu.Unlock()
			if err != nil {
				done <- err
				return
			}
			if p.Method == "ax.send" && !dropped {
				dropped = true
				owner.Close()
				store.disconnect(owner)
				x, y := net.Pipe()
				owner = &serverConn{Conn: x}
				defer owner.Close()
				store.mu.Lock()
				_, err = store.request(owner, "ax.connect", raw(object{"version": "1", "agent_id": s.ID, "secret": s.Secret}))
				store.mu.Unlock()
				if err != nil {
					y.Close()
					done <- err
					return
				}
				bridge.mu.Lock()
				bridge.c = newClient(y)
				bridge.changedLocked()
				bridge.mu.Unlock()
				continue
			}
			if err = writeFrame(owner, packet{ID: p.ID, Result: raw(value)}); err != nil {
				done <- err
				return
			}
			if p.Method == "ax.send" {
				done <- nil
				return
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var m Message
	err := bridge.toolCall(ctx, nil, "ax.send", object{"target": "api", "text": "review", "client_message_id": "stable_key"}, &m)
	if err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	var count int
	if err = store.db.QueryRow("SELECT count(*) FROM messages WHERE sender=?", s.ID).Scan(&count); err != nil || count != 1 || m.ID == "" {
		t.Fatalf("lost response duplicated mail: count=%d message=%+v error=%v", count, m, err)
	}
}

func TestToolSharesDeadlineAcrossSetup(t *testing.T) {
	x, y := net.Pipe()
	defer y.Close()
	c := newClient(x)
	defer c.close()
	dir := testDir(t)
	s := Session{Host: "claude", Native: uuid(), Started: true}
	file := filepath.Join(dir, "session.json")
	if err := saveSession(file, s); err != nil {
		t.Fatal(err)
	}
	b := &bridge{ctx: context.Background(), dir: dir, file: file, session: s, c: c}
	go func() {
		for {
			p, err := readFrame(y)
			if err != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
			if writeFrame(y, packet{ID: p.ID, Result: raw(object{})}) != nil {
				return
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := b.toolCall(ctx, nil, "ax.send", object{"client_message_id": "stable"}, nil)
	failure := toolFailure(err, object{"client_message_id": "stable"})
	if time.Since(start) > time.Second || failure["submission"] != "not_submitted" || failure["client_message_id"] != "stable" {
		t.Fatalf("wrong deadline or submission result: %v", failure)
	}
}

func TestBrokerRejectionDoesNotReconnect(t *testing.T) {
	x, y := net.Pipe()
	defer y.Close()
	c := newClient(x)
	defer c.close()
	go func() {
		p, _ := readFrame(y)
		writeFrame(y, packet{ID: p.ID, Error: &rpcError{Code: -32000, Message: "invalid credential"}})
	}()
	b := &bridge{dir: testDir(t), c: c}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := b.toolCall(ctx, json.RawMessage(`{}`), "ax.list", object{}, nil)
	var rpc *rpcError
	if !errors.As(err, &rpc) || b.c != c {
		t.Fatalf("broker rejection retried: %v", err)
	}
	select {
	case <-c.done:
		t.Fatal("healthy connection closed on broker rejection")
	default:
	}
}
