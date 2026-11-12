package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestDeleteHistoryByDate removes only the messages inside the official
// window and leaves the chat when the window misses. A basic group with
// min_date is rejected. The client loop stops only when offset comes back 0,
// and getHistory then does not return the deleted text.
func TestDeleteHistoryByDate(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "hist_a", "HistA")
	b, _ := loginNamed(t, cfg, ProfileAndroid, "hist_b", "HistB")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	peer := userPeer(t, cfg, a, "hist_b", "HistB")
	_ = b

	now := int32(time.Now().Unix())
	const kept = "history-kept-outside-window"
	const gone = "history-gone-inside-window"
	if _, err := a.SendText(ctx, peer, kept, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SendText(ctx, peer, gone, nil); err != nil {
		t.Fatal(err)
	}

	miss := deleteHistoryUntil(t, ctx, a, &mtproto.TLMessagesDeleteHistory{
		Constructor: mtproto.CRC32_messages_deleteHistory,
		Peer:        peer,
		MaxId:       0,
		MinDate:     &wrapperspb.Int32Value{Value: now + 3600},
		MaxDate:     &wrapperspb.Int32Value{Value: now + 7200},
	})
	if miss.GetOffset() != 0 {
		t.Fatalf("future window offset %d", miss.GetOffset())
	}
	if !historyHas(t, ctx, a, peer, kept) || !historyHas(t, ctx, a, peer, gone) {
		t.Fatal("a future date window deleted messages")
	}

	hit := deleteHistoryUntil(t, ctx, a, &mtproto.TLMessagesDeleteHistory{
		Constructor: mtproto.CRC32_messages_deleteHistory,
		Peer:        peer,
		Revoke:      true,
		MinDate:     &wrapperspb.Int32Value{Value: now - 3600},
		MaxDate:     &wrapperspb.Int32Value{Value: now + 3600},
	})
	if hit.GetOffset() != 0 {
		t.Fatalf("finished date delete offset %d", hit.GetOffset())
	}
	if historyHas(t, ctx, a, peer, kept) || historyHas(t, ctx, a, peer, gone) {
		t.Fatal("date window reported success but the messages are still in history")
	}

	chatID, err := a.CreateChat(ctx, fmt.Sprintf("hist-%d", time.Now().Unix()), inputUser(peer.GetUserId(), peer.GetAccessHash()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Invoke(ctx, &mtproto.TLMessagesDeleteHistory{
		Constructor: mtproto.CRC32_messages_deleteHistory,
		Peer:        inputPeerChat(chatID),
		MinDate:     &wrapperspb.Int32Value{Value: now - 60},
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "CHAT_REVOKE_DATE_UNSUPPORTED" {
		t.Fatalf("group date delete: %v", err)
	}
}

func deleteHistoryUntil(t *testing.T, ctx context.Context, c *Client, req *mtproto.TLMessagesDeleteHistory) *mtproto.Messages_AffectedHistory {
	t.Helper()
	var last *mtproto.Messages_AffectedHistory
	for i := 0; i < 50; i++ {
		obj, err := c.Invoke(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		last = affectedHistoryOf(t, obj)
		if last.GetOffset() == 0 {
			return last
		}
		req.MaxId = last.GetOffset()
	}
	t.Fatal("deleteHistory offset did not reach 0")
	return nil
}

func affectedHistoryOf(t *testing.T, obj mtproto.TLObject) *mtproto.Messages_AffectedHistory {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.Messages_AffectedHistory:
		return v
	case *mtproto.TLMessagesAffectedHistory:
		return v.To_Messages_AffectedHistory()
	default:
		t.Fatalf("affectedHistory %T", obj)
		return nil
	}
}

func historyHas(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, text string) bool {
	t.Helper()
	msgs, err := c.History(ctx, peer, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if m.GetMessage() == text {
			return true
		}
	}
	return false
}
