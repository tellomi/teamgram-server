package channel

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestSendPollsBanAndInviteUsersBan(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Bans", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}

	polls := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{SendPolls: true}).To_ChatBannedRights()
	if _, err = svc.EditBanned(ctx, 1, 0, in, 2, polls); err != nil {
		t.Fatal(err)
	}
	if err = svc.ForbidPoll(ctx, 2, ch.Id); err == nil || !strings.Contains(err.Error(), "CHAT_SEND_POLL_FORBIDDEN") {
		t.Fatalf("forbid poll: %v", err)
	}
	pollMsg := mtproto.MakeTLMessage(&mtproto.Message{
		Message: "",
		Media: mtproto.MakeTLMessageMediaPoll(&mtproto.MessageMedia{
			Poll: mtproto.MakeTLPoll(&mtproto.Poll{Id: 9}).To_Poll(),
		}).To_MessageMedia(),
	}).To_Message()
	if _, err = svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 11, Message: pollMsg}}); err == nil || !strings.Contains(err.Error(), "CHAT_SEND_POLL_FORBIDDEN") {
		t.Fatalf("send poll: %v", err)
	}
	if _, err = svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 12, Message: &mtproto.Message{Message: "still text"}}}); err != nil {
		t.Fatalf("text while polls banned: %v", err)
	}
	if err = svc.ForbidPoll(ctx, 3, ch.Id); err != nil {
		t.Fatalf("unrestricted forbid: %v", err)
	}

	inviteBan := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{InviteUsers: true}).To_ChatBannedRights()
	if _, err = svc.EditBanned(ctx, 1, 0, in, 2, inviteBan); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Invite(ctx, 2, 0, in, []int64{4}, nil); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("banned invite: %v", err)
	}
	if _, err = svc.Invite(ctx, 3, 0, in, []int64{4}, nil); err != nil {
		t.Fatalf("peer invite: %v", err)
	}
	parts, err := svc.GetParticipants(ctx, 1, in, nil, 0, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	var saw4 bool
	for _, p := range parts.GetParticipants() {
		if p.GetUserId() == 4 {
			saw4 = true
		}
	}
	if !saw4 {
		t.Fatal("user 4 was not invited")
	}
}
