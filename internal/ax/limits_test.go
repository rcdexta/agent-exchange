package ax

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type pipeListener struct {
	next chan net.Conn
	done chan struct{}
	once sync.Once
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.next:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}
func (l *pipeListener) Close() error   { l.once.Do(func() { close(l.done) }); return nil }
func (l *pipeListener) Addr() net.Addr { return &net.UnixAddr{Name: "fixture", Net: "unix"} }

func TestConnectionLimitPreservesActiveConnection(t *testing.T) {
	base := &pipeListener{next: make(chan net.Conn, 2), done: make(chan struct{})}
	l := limitListener(base, 1)
	defer l.Close()
	a, b := net.Pipe()
	defer b.Close()
	c, d := net.Pipe()
	defer d.Close()
	base.next <- a
	base.next <- c
	first, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	accepted := make(chan net.Conn, 1)
	go func() { conn, _ := l.Accept(); accepted <- conn }()
	select {
	case conn := <-accepted:
		if conn != nil {
			conn.Close()
		}
		t.Fatal("exceeded connection cap")
	case <-time.After(20 * time.Millisecond):
	}
	go b.Write([]byte("still alive"))
	buf := make([]byte, 11)
	if _, err := io.ReadFull(first, buf); err != nil || string(buf) != "still alive" {
		t.Fatal("closed active connection", err)
	}
	first.Close()
	select {
	case conn := <-accepted:
		if conn == nil {
			t.Fatal("lost admission")
		}
		conn.Close()
	case <-time.After(time.Second):
		t.Fatal("connection slot was not released")
	}
	// Closing the listener also releases an Accept waiting for capacity.
	base.next <- a
	first, _ = l.Accept()
	go func() { conn, _ := l.Accept(); accepted <- conn }()
	l.Close()
	select {
	case conn := <-accepted:
		if conn != nil {
			conn.Close()
			t.Fatal("accepted after closing")
		}
	case <-time.After(time.Second):
		t.Fatal("close left Accept blocked")
	}
	first.Close()
}

func TestAdapterWakeCapacityAndCancellation(t *testing.T) {
	dir := testDir(t)
	file := filepath.Join(dir, "session.json")
	s := Session{Native: uuid(), Started: true}
	if err := saveSession(file, s); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 8)
	server := adapterServer(context.Background(), dir, file, make(chan error, 1), func(ctx context.Context, _, _ string) error {
		entered <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/wake", bytes.NewReader(raw(adapterWake{Native: s.Native, Text: "wake"}))).WithContext(ctx)
			server.Handler.ServeHTTP(httptest.NewRecorder(), r)
		}()
	}
	for i := 0; i < 8; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("fixture wake did not start")
		}
	}
	r := httptest.NewRequest("POST", "/wake", bytes.NewReader(raw(adapterWake{Native: s.Native, Text: "overflow"})))
	w := httptest.NewRecorder()
	server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("wake capacity was not bounded", w.Code)
	}
	cancel()
	wg.Wait()
	nextCtx, nextCancel := context.WithCancel(context.Background())
	nextCancel()
	r = httptest.NewRequest("POST", "/wake", bytes.NewReader(raw(adapterWake{Native: s.Native, Text: "after"}))).WithContext(nextCtx)
	server.Handler.ServeHTTP(httptest.NewRecorder(), r)
	select {
	case <-entered:
	default:
		t.Fatal("cancelled wake leaked its slot")
	}
	if server.ReadHeaderTimeout == 0 || server.IdleTimeout == 0 || server.ReadTimeout == 0 {
		t.Fatal("missing HTTP timeouts")
	}
}

func TestNativeLogSurvivesLauncherDescriptorClose(t *testing.T) {
	dir := testDir(t)
	log, err := openDiagnosticLog(dir, "native.log")
	if err != nil {
		t.Fatal(err)
	}
	// Native output must keep working after the launching process closes its
	// copy of the log descriptor, just as when that launcher exits.
	cmd := exec.Command("sh", "-c", "read signal; printf 'still alive'; exit 0")
	cmd.Stdout, cmd.Stderr = log, log
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	log.Close()
	io.WriteString(in, "continue\n")
	in.Close()
	if err = cmd.Wait(); err != nil {
		t.Fatal("launcher log closure affected native host", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "native.log"))
	if err != nil || string(data) != "still alive" {
		t.Fatal("lost native output", string(data), err)
	}
}

func TestBrokerLogTrimKeepsChildDescriptor(t *testing.T) {
	dir := testDir(t)
	f, err := os.OpenFile(filepath.Join(dir, "broker.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	before, _ := f.Stat()
	if err = f.Truncate(maxLogBytes + 1); err != nil {
		t.Fatal(err)
	}
	trimDiagnosticLog(f, filepath.Join(dir, "broker.log.lock"))
	f.Write([]byte("after trim"))
	after, _ := f.Stat()
	if !os.SameFile(before, after) || after.Size() != 10 {
		t.Fatal("trim replaced or failed to bound the open descriptor")
	}
}
