package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestEditDefaultBannedBlocksMembers(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Quiet")
	in := inCh(ch)
	_, _ = svc.Invite(ctx, 1, 0, in, []int64{2}, nil)

	noSend := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{SendMessages: true, SendPlain: true}).To_ChatBannedRights()
	if _, err := svc.EditDefaultBanned(ctx, 2, 0, in, noSend); err == nil {
		t.Fatal("plain member changed default rights")
	}
	if _, err := svc.EditDefaultBanned(ctx, 1, 0, in, noSend); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditDefaultBanned(ctx, 1, 0, in, noSend); err == nil {
		t.Fatal("unchanged rights should be CHAT_NOT_MODIFIED")
	}
	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 1, Message: &mtproto.Message{Message: "x", Date: 1}}}); err == nil {
		t.Fatal("member sent despite default ban")
	}
	if _, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 2, Message: &mtproto.Message{Message: "y", Date: 1}}}); err != nil {
		t.Fatalf("creator blocked: %v", err)
	}
	open := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{}).To_ChatBannedRights()
	if _, err := svc.EditDefaultBanned(ctx, 1, 0, in, open); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 3, Message: &mtproto.Message{Message: "z", Date: 1}}}); err != nil {
		t.Fatalf("member still blocked after lifting: %v", err)
	}
}
