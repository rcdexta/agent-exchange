package ax

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const maxFrame = 512 << 10 // JSON escaping can expand a 64 KiB text body sixfold.
const maxText = 64 << 10

type object = map[string]any

type packet struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }
func raw(v any) json.RawMessage   { b, _ := json.Marshal(v); return b }
func readFrame(r io.Reader) (packet, error) {
	var n [4]byte
	if _, e := io.ReadFull(r, n[:]); e != nil {
		return packet{}, e
	}
	size := binary.BigEndian.Uint32(n[:])
	if size == 0 || size > maxFrame {
		return packet{}, errors.New("invalid frame size")
	}
	b := make([]byte, int(size))
	if _, e := io.ReadFull(r, b); e != nil {
		return packet{}, e
	}
	var p packet
	if e := json.Unmarshal(b, &p); e != nil {
		return p, e
	}
	if p.JSONRPC != "2.0" {
		return p, errors.New("JSON-RPC 2.0 required")
	}
	return p, nil
}
func writeFrame(w io.Writer, p packet) error {
	p.JSONRPC = "2.0"
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	if len(b) > maxFrame {
		return errors.New("frame too large")
	}
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(b)))
	for _, buf := range [][]byte{n[:], b} {
		for len(buf) > 0 {
			n, e := w.Write(buf)
			if e != nil {
				return e
			}
			if n == 0 {
				return io.ErrShortWrite
			}
			buf = buf[n:]
		}
	}
	return nil
}

type client struct {
	conn    net.Conn
	mu      sync.Mutex
	writeMu sync.Mutex
	next    int
	pending map[string]chan packet
	offers  chan Message
	done    chan struct{}
}

func dial(path string) (*client, error) {
	conn, e := net.DialTimeout("unix", path, time.Second)
	if e != nil {
		return nil, e
	}
	c := &client{conn: conn, pending: map[string]chan packet{}, offers: make(chan Message, 8), done: make(chan struct{})}
	go func() {
		defer close(c.done)
		defer conn.Close()
		for {
			p, e := readFrame(conn)
			if e != nil {
				return
			}
			if p.Method == "ax.delivery.offer" {
				var m Message
				if json.Unmarshal(p.Params, &m) != nil {
					return
				}
				select {
				case c.offers <- m:
				default:
					return
				}
			} else {
				c.mu.Lock()
				ch := c.pending[string(p.ID)]
				c.mu.Unlock()
				if ch != nil {
					select {
					case ch <- p:
					default:
					}
				}
			}
		}
	}()
	return c, nil
}
func (c *client) close() { c.conn.Close() }
func (c *client) call(method string, args any, out any) error {
	c.mu.Lock()
	c.next++
	id := fmt.Sprint(c.next)
	ch := make(chan packet, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	c.writeMu.Lock()
	c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	e := writeFrame(c.conn, packet{ID: json.RawMessage(id), Method: method, Params: raw(args)})
	c.writeMu.Unlock()
	if e != nil {
		return e
	}
	select {
	case p := <-ch:
		if p.Error != nil {
			return p.Error
		}
		if out != nil {
			return json.Unmarshal(p.Result, out)
		}
		return nil
	case <-c.done:
		return errors.New("broker connection closed")
	case <-time.After(10 * time.Second):
		return errors.New("broker response timed out; retry sends using the same client_message_id")
	}
}
