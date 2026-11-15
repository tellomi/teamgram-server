package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestPaidMediaAndSuggestedPostRejected checks unopened paid media and
// suggested posts are rejected instead of stored as a normal message.
// Phones 9996610766–9996610767, code 11111, layer 232.
func TestPaidMediaAndSuggestedPostRejected(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(90 * time.Second)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610766", "MediaA")
	_, bSelf := dialPhone(t, ctx, cfg, "9996610767", "MediaB")
	peer := inputPeerUser(bSelf.ID, bSelf.AccessHash)

	_, err := a.Invoke(ctx, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_330e77f,
		Peer:        peer,
		Media: mtproto.MakeTLInputMediaPaidMedia(&mtproto.InputMedia{
			StarsAmount: 1,
		}).To_InputMedia(),
		Message:  "paid",
		RandomId: randomID(),
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "MEDIA_INVALID" {
		t.Fatalf("paid media: %v", err)
	}

	_, err = a.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        peer,
		Message:     "suggest",
		RandomId:    randomID(),
		SuggestedPost: mtproto.MakeTLSuggestedPost(&mtproto.SuggestedPost{
			Price: mtproto.MakeTLStarsAmount(&mtproto.StarsAmount{Amount: 1}).To_StarsAmount(),
		}).To_SuggestedPost(),
	})
	rpc, ok = asRPCError(err)
	if !ok || rpc.Message != "SUGGESTED_POST_PEER_INVALID" {
		t.Fatalf("suggested post: %v", err)
	}
}
