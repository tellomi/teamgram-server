package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestReactionLimitAndPaidClosed covers ISSUE-S-REACTIONS-LIMIT-001 and
// ISSUE-S-PAIDREACT-001. Phones 9996610476–9996610477.
func TestReactionLimitAndPaidClosed(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610476", "ReactA")
	other, _ := dialPhone(t, ctx, cfg, "9996610477", "ReactB")
	otherID, otherHash, err := owner.ImportPhone(ctx, "9996610477", "ReactB")
	if err != nil {
		t.Fatal(err)
	}
	groupID, groupHash, err := owner.CreateMegagroup(ctx, "ReactCap")
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(groupID, groupHash)
	if err = owner.InviteToChannel(ctx, channel, inputUser(otherID, otherHash)); err != nil {
		t.Fatal(err)
	}
	var memberHash int64
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		hash, ok, err := other.ChatAccess(ctx, groupID)
		if err != nil || !ok {
			return false, err
		}
		memberHash = hash
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChannel(groupID, groupHash)
	if _, err = owner.Invoke(ctx, &mtproto.TLMessagesSetChatAvailableReactions{
		Constructor:                      mtproto.CRC32_messages_setChatAvailableReactions_864b2581,
		Peer:                             peer,
		AvailableReactions_CHATREACTIONS: mtproto.MakeTLChatReactionsAll(&mtproto.ChatReactions{}).To_ChatReactions(),
		ReactionsLimit:                   wrapperspb.Int32(1),
	}); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprintf("cap-%d", time.Now().UnixNano())
	msgID, err := owner.SendText(ctx, peer, text, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.SendReaction(ctx, peer, msgID, "👍"); err != nil {
		t.Fatal(err)
	}
	var otherIDMsg int32
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := other.History(ctx, inputPeerChannel(groupID, memberHash), 20)
		if err != nil {
			return false, err
		}
		otherIDMsg = idOfText(msgs, text)
		return otherIDMsg != 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err = other.SendReaction(ctx, inputPeerChannel(groupID, memberHash), otherIDMsg, "❤️")
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "REACTIONS_TOO_MANY" {
		t.Fatalf("second emoji: %v", err)
	}
	if _, err = other.SendReaction(ctx, inputPeerChannel(groupID, memberHash), otherIDMsg, "👍"); err != nil {
		t.Fatalf("same emoji: %v", err)
	}

	_, err = owner.Invoke(ctx, &mtproto.TLMessagesSendPaidReaction{
		Constructor: mtproto.CRC32_messages_sendPaidReaction_58bbcb50,
		Peer:        inputPeerSelf(),
		MsgId:       1,
		Count:       1,
		RandomId:    5,
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "PAYMENT_UNSUPPORTED" {
		t.Fatalf("private paid: %v", err)
	}
	_, err = owner.Invoke(ctx, &mtproto.TLMessagesSendPaidReaction{
		Constructor: mtproto.CRC32_messages_sendPaidReaction_58bbcb50,
		Peer:        peer,
		MsgId:       msgID,
		Count:       1,
		RandomId:    time.Now().Unix()<<32 | 7,
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "PAYMENT_UNSUPPORTED" {
		t.Fatalf("channel paid: %v", err)
	}
}
