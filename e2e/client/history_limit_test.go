package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestHistoryLimit100 is the production probe: after more than 50 messages,
// messages.getHistory limit=100 must return the whole page, and the same
// ids must come back from channels.getMessages. offset_id, add_offset,
// min_id, and max_id still select the window.
func TestHistoryLimit100(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610095", "Hist")
	chID, hash, err := c.CreateMegagroup(ctx, fmt.Sprintf("hist-%d", time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChannel(chID, hash)
	const n = 60
	for i := 0; i < n; i++ {
		if _, err := c.SendText(ctx, peer, fmt.Sprintf("hist-%02d", i), nil); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	page := historyPage(t, c, ctx, &mtproto.TLMessagesGetHistory{
		Constructor: mtproto.CRC32_messages_getHistory,
		Peer:        peer,
		Limit:       100,
	})
	if len(page) < n {
		t.Fatalf("limit 100 returned %d messages", len(page))
	}
	ids := make([]int32, 0, len(page))
	for _, m := range page {
		if m.GetId() != 0 {
			ids = append(ids, m.GetId())
		}
	}
	if len(ids) < n {
		t.Fatalf("ids %d", len(ids))
	}
	// Newest first. The first id is the newest.
	newest, oldest := ids[0], ids[len(ids)-1]
	if newest <= oldest {
		t.Fatalf("order %d .. %d", newest, oldest)
	}

	offset := ids[10]
	older := historyPage(t, c, ctx, &mtproto.TLMessagesGetHistory{
		Constructor: mtproto.CRC32_messages_getHistory,
		Peer:        peer,
		OffsetId:    offset,
		Limit:       100,
	})
	for _, m := range older {
		if m.GetId() >= offset {
			t.Fatalf("offset_id %d kept %d", offset, m.GetId())
		}
	}
	skipped := historyPage(t, c, ctx, &mtproto.TLMessagesGetHistory{
		Constructor: mtproto.CRC32_messages_getHistory,
		Peer:        peer,
		OffsetId:    offset,
		AddOffset:   5,
		Limit:       10,
	})
	if len(skipped) == 0 || len(older) < 6 {
		t.Fatalf("add_offset page %d older %d", len(skipped), len(older))
	}
	if skipped[0].GetId() != older[5].GetId() {
		t.Fatalf("add_offset %d want %d", skipped[0].GetId(), older[5].GetId())
	}

	minID := ids[len(ids)/2]
	above := historyPage(t, c, ctx, &mtproto.TLMessagesGetHistory{
		Constructor: mtproto.CRC32_messages_getHistory,
		Peer:        peer,
		MinId:       minID,
		Limit:       100,
	})
	if len(above) == 0 {
		t.Fatal("min_id returned nothing")
	}
	for _, m := range above {
		if m.GetId() <= minID {
			t.Fatalf("min_id %d kept %d", minID, m.GetId())
		}
	}
	maxID := ids[len(ids)/2]
	below := historyPage(t, c, ctx, &mtproto.TLMessagesGetHistory{
		Constructor: mtproto.CRC32_messages_getHistory,
		Peer:        peer,
		MaxId:       maxID,
		Limit:       100,
	})
	if len(below) == 0 {
		t.Fatal("max_id returned nothing")
	}
	for _, m := range below {
		if m.GetId() >= maxID {
			t.Fatalf("max_id %d kept %d", maxID, m.GetId())
		}
	}

	var inputIDs []*mtproto.InputMessage
	for _, id := range ids {
		inputIDs = append(inputIDs, mtproto.MakeTLInputMessageID(&mtproto.InputMessage{Id: id}).To_InputMessage())
	}
	got := historyPage(t, c, ctx, &mtproto.TLChannelsGetMessages{
		Constructor:           mtproto.CRC32_channels_getMessages_ad8c9a23,
		Channel:               inputChannel(chID, hash),
		Id_VECTORINPUTMESSAGE: inputIDs,
	})
	seen := map[int32]bool{}
	for _, m := range got {
		if m.GetPredicateName() == mtproto.Predicate_messageEmpty {
			continue
		}
		seen[m.GetId()] = true
	}
	if len(seen) < n {
		t.Fatalf("channels.getMessages returned %d of %d", len(seen), n)
	}
}

func historyPage(t *testing.T, c *Client, ctx context.Context, req mtproto.TLObject) []*mtproto.Message {
	t.Helper()
	res := invokeOK(t, c, req)
	msgs, ok := messagesOf(res)
	if !ok {
		t.Fatalf("%T", res)
	}
	return msgs
}
