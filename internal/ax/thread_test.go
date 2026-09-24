package ax

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func followup(t *testing.T, b *broker, from *serverConn, parent, key, text string) Message {
	t.Helper()
	return request(t, b, from, "ax.follow_up", object{"message_id": parent, "text": text, "client_message_id": key}).(Message)
}

func TestFollowUpRoutesToOriginalPeerAndPreservesThread(t *testing.T) {
	b, _ := localBroker(t)
	_, from, _ := endpoint(t, b, "web", testMesh)
	_, to, _ := endpoint(t, b, "api", testMesh)
	_, stranger, _ := endpoint(t, b, "other", testMesh)
	first := request(t, b, from, "ax.send", object{"target": "api", "text": "review", "client_message_id": "first"}).(Message)
	add := followup(t, b, from, first.ID, "extra", "Also check the boundary.")
	if first.Thread != first.ID || add.Thread != first.ID || add.Parent != first.ID || add.Recipient != to.agent || add.Depth != 1 || add.Seq != first.Seq+1 {
		t.Fatalf("bad follow-up: %+v", add)
	}
	saved, _ := b.message(first.ID)
	if saved.State != "queued" {
		t.Fatal("addendum acknowledged original")
	}
	for _, c := range []*serverConn{to, stranger, {}} {
		if _, err := b.request(c, "ax.follow_up", raw(object{"message_id": first.ID, "text": "forged", "client_message_id": "forged"})); err == nil {
			t.Fatal("foreign follow-up accepted")
		}
	}
	pending := request(t, b, to, "ax.pending", object{}).(pendingPage)
	if len(pending.Messages) != 2 || pending.Messages[1].Thread != first.ID || pending.Messages[1].Parent != first.ID {
		t.Fatal("pending thread missing")
	}
	if err := b.event(first.ID, "content_served", to.epoch); err != nil {
		t.Fatal(err)
	}
	answer := request(t, b, to, "ax.reply", object{"message_id": first.ID, "text": "found a bug", "client_message_id": "answer"}).(Message)
	note := followup(t, b, to, answer.ID, "note", "The exact boundary is fifty.")
	if answer.Thread != first.ID || note.Thread != first.ID || note.Recipient != from.agent || note.Parent != answer.ID {
		t.Fatal("reply-side thread lost")
	}
	page := request(t, b, from, "ax.thread", object{"message_id": add.ID}).(threadPage)
	if len(page.Messages) != 4 || page.Thread != first.ID || page.Messages[0].ID != first.ID || page.Messages[3].ID != note.ID {
		t.Fatalf("incorrect thread: %+v", page)
	}
	unrelated := request(t, b, from, "ax.send", object{"target": "other", "text": "different task", "client_message_id": "different"}).(Message)
	for _, args := range []object{{"message_id": first.ID}, {"message_id": first.ID, "after_message_id": unrelated.ID}} {
		if _, err := b.request(stranger, "ax.thread", raw(args)); err == nil {
			t.Fatal("third party read thread")
		}
	}
	if _, err := b.request(from, "ax.thread", raw(object{"message_id": first.ID, "after_message_id": unrelated.ID})); err == nil {
		t.Fatal("foreign cursor accepted")
	}
}

func TestFollowUpIdempotencyDepthAndPolicy(t *testing.T) {
	b, _ := localBroker(t)
	_, from, _ := endpoint(t, b, "web", testMesh)
	_, to, _ := endpoint(t, b, "api", testMesh)
	first := request(t, b, from, "ax.send", object{"target": "api", "text": "request", "client_message_id": "first"}).(Message)
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.mu.Lock()
			defer b.mu.Unlock()
			ids <- followup(t, b, from, first.ID, "retry", "same addendum").ID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for got := range ids {
		if id != "" && id != got {
			t.Fatal("concurrent retry duplicated")
		}
		id = got
	}
	if _, err := b.request(from, "ax.follow_up", raw(object{"message_id": first.ID, "text": "changed", "client_message_id": "retry"})); err == nil {
		t.Fatal("changed retry accepted")
	}
	parent := first.ID
	for depth := 1; depth <= 8; depth++ {
		m := followup(t, b, from, parent, fmt.Sprint(depth), fmt.Sprint(depth))
		parent = m.ID
		if m.Depth != depth {
			t.Fatal("wrong depth")
		}
	}
	if _, err := b.request(from, "ax.follow_up", raw(object{"message_id": parent, "text": "loop", "client_message_id": "too_deep"})); err == nil {
		t.Fatal("depth bound bypassed")
	}
	b.peers[to.agent].Policy = "refuse"
	if _, err := b.request(from, "ax.follow_up", raw(object{"message_id": first.ID, "text": "new", "client_message_id": "refused"})); err == nil {
		t.Fatal("recipient refusal bypassed")
	}
	b.peers[to.agent].Policy = "accept"
	for _, state := range []string{"expired", "refused", "abandoned"} {
		if err := b.event(first.ID, state, to.epoch); err != nil {
			t.Fatal(err)
		}
		if _, err := b.request(from, "ax.follow_up", raw(object{"message_id": first.ID, "text": "new", "client_message_id": state})); err == nil {
			t.Fatal("follow-up accepted for", state)
		}
	}
}

func TestThreadReadWithholdsUndeliveredTextAndDoesNotAcknowledge(t *testing.T) {
	b, _ := localBroker(t)
	_, from, _ := endpoint(t, b, "web", testMesh)
	_, to, _ := endpoint(t, b, "api", testMesh)
	first := request(t, b, from, "ax.send", object{"target": "api", "text": "private request", "client_message_id": "first"}).(Message)
	add := followup(t, b, from, first.ID, "add", "private addendum")
	for _, state := range []string{"queued", "expired", "refused", "abandoned", "handoff_started", "delivery_uncertain", "content_served", "acknowledged"} {
		if err := b.event(first.ID, state, to.epoch); err != nil {
			t.Fatal(err)
		}
		var before, after int
		b.db.QueryRow("SELECT count(*) FROM events").Scan(&before)
		page := request(t, b, to, "ax.thread", object{"message_id": first.ID}).(threadPage)
		b.db.QueryRow("SELECT count(*) FROM events").Scan(&after)
		wantHidden := state == "queued" || state == "expired" || state == "refused" || state == "abandoned"
		if page.Messages[0].Withheld != wantHidden || (page.Messages[0].Text == "") != wantHidden || !page.Messages[1].Withheld || page.Messages[1].Text != "" || page.Messages[1].ID != add.ID || before != after || dispatchAfter("ax.thread") {
			t.Fatalf("read bypassed delivery: %+v", page)
		}
	}
	for _, control := range []string{"hold", "refuse", "recipient_permission", "sender_permission"} {
		b.peers[to.agent].Policy = "accept"
		b.peers[to.agent].Permission = "read-only"
		b.peers[from.agent].Permission = "read-only"
		switch control {
		case "hold", "refuse":
			b.peers[to.agent].Policy = control
		case "recipient_permission":
			b.peers[to.agent].Permission = "unknown"
		case "sender_permission":
			b.peers[from.agent].Permission = "unknown"
		}
		page := request(t, b, to, "ax.thread", object{"message_id": first.ID}).(threadPage)
		if page.Messages[0].Text != "" || !page.Messages[0].Withheld {
			t.Fatal("history bypassed", control)
		}
	}
}

func TestThreadPaginationRetentionAndLegacyReplies(t *testing.T) {
	b, dir := localBroker(t)
	_, from, _ := endpoint(t, b, "web", testMesh)
	_, _, _ = endpoint(t, b, "api", testMesh)
	first := request(t, b, from, "ax.send", object{"target": "api", "text": "request", "client_message_id": "first"}).(Message)
	ids := []string{first.ID}
	for i := 0; i < 25; i++ {
		ids = append(ids, followup(t, b, from, first.ID, fmt.Sprint(i), fmt.Sprint(i)).ID)
	}
	page := request(t, b, from, "ax.thread", object{"message_id": first.ID}).(threadPage)
	if len(page.Messages) != 20 || page.Next != ids[19] || page.Limited {
		t.Fatalf("bad pagination: %+v", page)
	}
	last := request(t, b, from, "ax.thread", object{"message_id": first.ID, "after_message_id": page.Next}).(threadPage)
	if len(last.Messages) != 6 || last.Messages[0].ID != ids[20] || last.Next != "" {
		t.Fatal("skipped or repeated messages")
	}
	// Existing stored messages need no bulk backfill. A new follow-up can join
	// their legacy parent chain, and the bounded reader includes both formats.
	if _, err := b.db.Exec("UPDATE messages SET data=json_remove(data,'$.thread_id')"); err != nil {
		t.Fatal(err)
	}
	newNote := followup(t, b, from, ids[1], "legacy", "legacy follow-up")
	if newNote.Thread != first.ID {
		t.Fatal("legacy root changed")
	}
	page = request(t, b, from, "ax.thread", object{"message_id": newNote.ID}).(threadPage)
	if len(page.Messages) != 20 || page.Next != ids[19] {
		t.Fatal("legacy replies lost")
	}
	// Pruning an intermediate legacy message must not hide new descendants,
	// whose immutable thread_id survives their parent's removal.
	if _, err := b.db.Exec("DELETE FROM messages WHERE id=?", ids[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := b.request(from, "ax.thread", raw(object{"message_id": newNote.ID, "after_message_id": ids[1]})); err == nil {
		t.Fatal("removed cursor silently reused")
	}
	b.db.Close()
	reopened, err := openBroker(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.db.Close()
	reopened.peers[from.agent].Permission = "read-only"
	got, err := reopened.thread(reopened.peers[from.agent], newNote.ID, ids[19])
	if err != nil || len(got.Messages) != 7 || got.Messages[6].ID != newNote.ID {
		t.Fatalf("restart/pruning lost context: %+v %v", got, err)
	}
}

func TestThreadBoundsBodyAndTraversal(t *testing.T) {
	b, _ := localBroker(t)
	_, from, _ := endpoint(t, b, "web", testMesh)
	_, _, _ = endpoint(t, b, "api", testMesh)
	first := request(t, b, from, "ax.send", object{"target": "api", "text": strings.Repeat("\x01", maxText), "client_message_id": "first"}).(Message)
	second := followup(t, b, from, first.ID, "second", strings.Repeat("界", maxText/3))
	page := request(t, b, from, "ax.thread", object{"message_id": first.ID}).(threadPage)
	if len(page.Messages) != 1 || page.Next != first.ID {
		t.Fatal("text budget exceeded")
	}
	var wire bytes.Buffer
	if err := writeFrame(&wire, packet{ID: raw(1), Result: raw(page)}); err != nil {
		t.Fatal("bounded page exceeds wire frame", err)
	}
	page = request(t, b, from, "ax.thread", object{"message_id": first.ID, "after_message_id": page.Next}).(threadPage)
	if len(page.Messages) != 1 || page.Messages[0].ID != second.ID || page.Messages[0].Text != second.Text {
		t.Fatal("body truncated")
	}
	for i := 0; i < threadScanLimit; i++ {
		// This in-process fixture has no bridge to keep its lease alive.
		request(t, b, from, "ax.heartbeat", object{})
		// Age stored rate-accounting timestamps only; insertion order and contents
		// remain intact. Exercise the real send path without waiting for minutes.
		if _, err := b.db.Exec("UPDATE messages SET created=0"); err != nil {
			t.Fatal(err)
		}
		followup(t, b, from, first.ID, fmt.Sprint(i), fmt.Sprint(i))
	}
	page = request(t, b, from, "ax.thread", object{"message_id": first.ID}).(threadPage)
	if !page.Limited {
		t.Fatal("large history was silently reported complete")
	}
	seen := map[string]bool{}
	after := ""
	for {
		page := request(t, b, from, "ax.thread", object{"message_id": first.ID, "after_message_id": after}).(threadPage)
		for _, m := range page.Messages {
			if seen[m.ID] {
				t.Fatal("page repeated a message")
			}
			seen[m.ID] = true
		}
		if page.Next == "" {
			break
		}
		if page.Next == after {
			t.Fatal("cursor did not advance")
		}
		after = page.Next
	}
	if len(seen) != threadScanLimit+2 {
		t.Fatalf("bounded traversal lost paginated messages: %d", len(seen))
	}
	// Legacy history reports its window limit rather than claiming completeness.
	if _, err := b.db.Exec("UPDATE messages SET data=json_remove(data,'$.thread_id')"); err != nil {
		t.Fatal(err)
	}
	page = request(t, b, from, "ax.thread", object{"message_id": first.ID}).(threadPage)
	if !page.Limited || len(page.Messages) != 1 || page.Next != first.ID {
		t.Fatal("legacy fan-out lost its bounds")
	}
	var specs []object = toolList()
	for _, spec := range specs {
		if spec["name"] == "get_thread" {
			data, _ := json.Marshal(spec)
			if !bytes.Contains(data, []byte(`"readOnlyHint":true`)) {
				t.Fatal("history advertised as a write")
			}
		}
	}
}

func TestThreadMixedLegacyAndIndexedPaginationKeepsOldContext(t *testing.T) {
	b, _ := localBroker(t)
	_, from, _ := endpoint(t, b, "web", testMesh)
	_, _, _ = endpoint(t, b, "api", testMesh)
	first := request(t, b, from, "ax.send", object{"target": "api", "text": "request", "client_message_id": "first"}).(Message)
	legacy := followup(t, b, from, first.ID, "legacy", "old context")
	if _, err := b.db.Exec("UPDATE messages SET data=json_remove(data,'$.thread_id')"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < threadScanLimit+5; i++ {
		request(t, b, from, "ax.heartbeat", object{})
		if _, err := b.db.Exec("UPDATE messages SET created=0"); err != nil {
			t.Fatal(err)
		}
		followup(t, b, from, first.ID, fmt.Sprint(i), fmt.Sprint(i))
	}
	firstPage := request(t, b, from, "ax.thread", object{"message_id": first.ID}).(threadPage)
	if firstPage.Messages[1].ID != legacy.ID {
		t.Fatal("indexed rows starved older legacy context")
	}
	seen := map[string]bool{}
	after := ""
	for {
		page := request(t, b, from, "ax.thread", object{"message_id": first.ID, "after_message_id": after}).(threadPage)
		for _, m := range page.Messages {
			if seen[m.ID] {
				t.Fatal("repeated mixed-history row")
			}
			seen[m.ID] = true
		}
		if page.Next == "" {
			break
		}
		after = page.Next
	}
	if len(seen) != threadScanLimit+7 {
		t.Fatalf("mixed history skipped rows: %d", len(seen))
	}
}
