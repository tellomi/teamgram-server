package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestMegagroupOwnerCannotEditMemberMessage is ISSUE-S-MG-EDIT-OTHERS-001
// (表格行 155). In a megagroup, messages.editMessage by anyone other than
// the author returns 403 MESSAGE_AUTHOR_REQUIRED. The author can still edit
// their own message. Phones 9996610640 and 9996610641, code 11111, layer 232.
func TestMegagroupOwnerCannotEditMemberMessage(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610640", "EditA")
	b, _ := dialPhone(t, ctx, cfg, "9996610641", "EditB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610641", "EditB")
	if err != nil {
		t.Fatal(err)
	}
	title := fmt.Sprintf("EditRoom-%d", time.Now().Unix())
	chID, chHash, err := a.CreateMegagroup(ctx, title)
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(chID, chHash)
	if err = a.InviteToChannel(ctx, channel, inputUser(bID, bHash)); err != nil {
		t.Fatal(err)
	}
	var memberHash int64
	if err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		got, ok, err := b.ChatAccess(ctx, chID)
		if err != nil || !ok {
			return false, err
		}
		memberHash = got
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	peerB := inputPeerChannel(chID, memberHash)
	text := fmt.Sprintf("author-line-%d", time.Now().UnixNano())
	msgID, err := b.SendText(ctx, peerB, text, nil)
	if err != nil {
		t.Fatal(err)
	}
	peerA := inputPeerChannel(chID, chHash)
	_, err = a.Invoke(ctx, &mtproto.TLMessagesEditMessage{
		Constructor: mtproto.CRC32_messages_editMessage_48f71778,
		Peer:        peerA,
		Id:          msgID,
		Message:     &wrapperspb.StringValue{Value: "owner-rewrite"},
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Code != 403 || rpc.Message != "MESSAGE_AUTHOR_REQUIRED" {
		t.Fatalf("owner edit: %v", err)
	}
	edited, err := b.Invoke(ctx, &mtproto.TLMessagesEditMessage{
		Constructor: mtproto.CRC32_messages_editMessage_48f71778,
		Peer:        peerB,
		Id:          msgID,
		Message:     &wrapperspb.StringValue{Value: text + "-edited"},
	})
	if err != nil {
		t.Fatalf("author edit: %v", err)
	}
	if !hasText(allMessages(edited), text+"-edited") {
		hist, herr := b.History(ctx, peerB, 20)
		if herr != nil || !hasText(hist, text+"-edited") {
			t.Fatalf("author edit not stored: %T %v", edited, herr)
		}
	}
}
