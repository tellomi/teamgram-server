package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestSendReactionsBanBlocksMembers(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "ReactRoom")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{BanUsers: true}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 3, rights, ""); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 51,
		Message:  &mtproto.Message{Message: "hi", Date: 10},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sentChannelMessage(t, sent).GetId()

	noReact := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{SendReactions: true}).To_ChatBannedRights()
	if _, err := svc.EditDefaultBanned(ctx, 1, 0, in, noReact); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.React(ctx, 2, ch.Id, msgID, []string{"👍"}, false); err != mtproto.ErrChatSendReactionsForbidden {
		t.Fatalf("default ban: %v", err)
	}
	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{
		RandomID: 52,
		Message:  &mtproto.Message{Message: "text still works", Date: 11},
	}}); err != nil {
		t.Fatalf("text under reaction ban: %v", err)
	}
	if _, err := svc.React(ctx, 1, ch.Id, msgID, []string{"👍"}, false); err != nil {
		t.Fatalf("creator reaction: %v", err)
	}
	if _, err := svc.React(ctx, 3, ch.Id, msgID, []string{"❤"}, false); err != nil {
		t.Fatalf("admin reaction: %v", err)
	}

	open := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{}).To_ChatBannedRights()
	if _, err := svc.EditDefaultBanned(ctx, 1, 0, in, open); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditBanned(ctx, 1, 0, in, 2, noReact); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.React(ctx, 2, ch.Id, msgID, []string{"👍"}, false); err != mtproto.ErrChatSendReactionsForbidden {
		t.Fatalf("per-user ban: %v", err)
	}
	if _, err := svc.React(ctx, 2, ch.Id, msgID, nil, false); err != nil {
		t.Fatalf("clearing a reaction while banned: %v", err)
	}
}
