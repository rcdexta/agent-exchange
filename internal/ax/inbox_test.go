package ax

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestInboxDoesNotAcknowledgeOrBind(t *testing.T) {
	b, _ := localBroker(t)
	_, sender, _ := endpoint(t, b, "web", testMesh)
	_, receiver, _ := endpoint(t, b, "api", testMesh)
	m := request(t, b, sender, "ax.send", object{"target": "api", "text": "Review this", "client_message_id": "inbox"}).(Message)
	var before int
	if e := b.db.QueryRow("SELECT count(*) FROM events").Scan(&before); e != nil {
		t.Fatal(e)
	}
	observer := &serverConn{}
	items := request(t, b, observer, "ax.inbox", object{}).([]inboxItem)
	if len(items) != 1 || items[0].Sender != "web" || items[0].Recipient != "api" || items[0].State != "queued" {
		t.Fatal(items)
	}
	read := request(t, b, observer, "ax.inbox_message", object{"message_id": m.ID}).(Message)
	if read.Text != m.Text || read.State != "queued" || observer.agent != "" {
		t.Fatal("observer changed delivery or identity", read, observer)
	}
	var after int
	if e := b.db.QueryRow("SELECT count(*) FROM events").Scan(&after); e != nil || before != after {
		t.Fatal("inspection wrote receipts", before, after, e)
	}
	for _, method := range []string{"ax.inbox", "ax.inbox_message"} {
		if _, e := b.request(receiver, method, raw(object{"message_id": m.ID})); e == nil {
			t.Fatal("observer API exposed on a bound agent connection", method)
		}
	}
	b.disconnect(observer)
	if b.peers[sender.agent].conn != sender || b.peers[receiver.agent].conn != receiver {
		t.Fatal("closing observer disconnected an agent")
	}
}

func TestInboxBoundsPayloadAndFiltersBothDirections(t *testing.T) {
	b, _ := localBroker(t)
	a, _, _ := endpoint(t, b, "api", testMesh)
	w, _, _ := endpoint(t, b, "web", testMesh)
	x, _, _ := endpoint(t, b, "other", testMesh)
	// Insert history directly to avoid the intentional model send rate limits.
	for i := range 130 {
		from, to := a, w
		if i%2 == 0 {
			from, to = w, a
		}
		if i == 129 {
			from, to = w, x
		}
		m := Message{ID: fmt.Sprintf("msg_%d", i), Text: strings.Repeat("界", 300), Sender: Agent{ID: from.ID}, Recipient: to.ID, Created: int64(i)}
		_, e := b.db.Exec("INSERT INTO messages(id,sender,recipient,client_id,request_hash,seq,state,data,created,expires) VALUES(?,?,?,?,?,?,'queued',?,?,9999999999999)", m.ID, from.ID, to.ID, m.ID, "hash", i, string(raw(m)), i)
		if e != nil {
			t.Fatal(e)
		}
	}
	items, e := b.inbox("api")
	if e != nil || len(items) != 100 || items[0].ID != "msg_128" || items[99].ID != "msg_29" {
		t.Fatal(items, e)
	}
	for _, item := range items {
		if len([]rune(item.Preview)) != 160 || (item.Sender != "api" && item.Recipient != "api") {
			t.Fatal(item)
		}
	}
	if len(raw(items)) >= maxFrame {
		t.Fatal("inbox snapshot exceeds the transport frame")
	}
	if _, e = b.inbox("missing"); e == nil {
		t.Fatal("unknown filter silently accepted")
	}
}

func TestInboxPreservesSelectionAndSanitizesTerminalText(t *testing.T) {
	v := inboxView{items: []inboxItem{{ID: "a"}, {ID: "b"}}, selected: 1, scroll: 3}
	v.selectItems([]inboxItem{{ID: "new"}, {ID: "a"}, {ID: "b"}})
	if v.selected != 2 || v.scroll != 3 {
		t.Fatal("arrival disturbed selected message", v)
	}
	v.body = Message{ID: "b", Text: "hello\x1b]52;c;c2VjcmV0\a\n世界\rFAKE\u202e\u009b31m"}
	for _, size := range [][2]int{{100, 30}, {40, 10}, {1, 1}} {
		frame := v.render(size[0], size[1], "api\x1b[2J")
		if strings.ContainsAny(inboxText(frame), "\x1b\a\r\u202e\u009b") {
			t.Fatal("terminal controls survived sanitizing")
		}
		if strings.Count(frame, "\x1b[H") != 1 || strings.Contains(frame, "\x1b]52") || strings.Contains(frame, "\x1b[2J") {
			t.Fatal("message controlled terminal", frame)
		}
		lines := strings.Split(inboxText(frame), "\n")
		if len(lines) > size[1] {
			t.Fatal("frame exceeds terminal height")
		}
		// Remove only the renderer's known ANSI sequences before checking width.
		plain := strings.NewReplacer("\x1b[H", "", "\x1b[K", "", "\x1b[J", "", "\r", "").Replace(frame)
		for _, line := range strings.Split(plain, "\n") {
			if runewidth.StringWidth(line) > max(1, size[0]-1) {
				t.Fatal("frame exceeds terminal width", line)
			}
		}
	}
	v.selectItems(nil)
	if v.selected != 0 || v.scroll != 0 {
		t.Fatal("empty history retained invalid selection")
	}
}
