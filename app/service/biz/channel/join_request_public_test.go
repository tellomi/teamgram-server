package channel

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// A public megagroup with join_request must not add the user when they
// join by username (channels.joinChannel). Official Telegram returns
// INVITE_REQUEST_SENT, lists the user from getChatInviteImporters
// requested=true, lets an admin approve with hideChatJoinRequest, and
// pushes updatePendingJoinRequests to admins.
func TestPublicMegagroupJoinRequestByUsername(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Public", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	in := inCh(ch)
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "publicroom"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleJoinRequest(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	view := svc.ChatFor(ctx, 4, mustChannel(t, svc, ctx, ch.Id))
	if !view.GetJoinRequest() || view.GetUsername().GetValue() != "publicroom" {
		t.Fatalf("channel before join: join_request=%v username=%q", view.GetJoinRequest(), view.GetUsername().GetValue())
	}

	var adminPushes []*mtproto.Updates
	svc.SetHooks(Hooks{
		Push: func(_ context.Context, userID int64, _ []int64, updates *mtproto.Updates) {
			if userID == 1 {
				adminPushes = append(adminPushes, updates)
			}
		},
	})

	if _, err := svc.Join(ctx, 4, 0, in); err == nil || !strings.Contains(err.Error(), "INVITE_REQUEST_SENT") {
		t.Fatalf("join: %v", err)
	}
	parts, err := svc.GetParticipants(ctx, 1, in, nil, 0, 50, 0)
	if err != nil || parts.Count != 1 {
		t.Fatalf("join added the user, count=%d err=%v", parts.Count, err)
	}
	pending, total, err := svc.InviteImporters(ctx, 1, in, true, "", 0, 0, 50)
	if err != nil || total != 1 || len(pending) != 1 || pending[0].GetUserId() != 4 || !pending[0].GetRequested() {
		t.Fatalf("importers total=%d rows=%+v err=%v", total, pending, err)
	}
	if len(adminPushes) != 1 || !hasPendingJoin(adminPushes[0], 4) {
		t.Fatalf("admin push %#v", adminPushes)
	}

	if _, err := svc.Join(ctx, 4, 0, in); err == nil || !strings.Contains(err.Error(), "INVITE_REQUEST_SENT") {
		t.Fatalf("second join: %v", err)
	}
	pending, total, err = svc.InviteImporters(ctx, 1, in, true, "", 0, 0, 50)
	if err != nil || total != 1 || len(pending) != 1 {
		t.Fatalf("duplicate request total=%d rows=%d err=%v", total, len(pending), err)
	}

	if _, err := svc.HideJoinRequest(ctx, 1, 0, in, 4, true); err != nil {
		t.Fatal(err)
	}
	member, err := svc.GetParticipant(ctx, 1, in, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 4}).To_InputPeer())
	if err != nil || member.GetParticipant().GetUserId() != 4 {
		t.Fatalf("approved %+v %v", member, err)
	}
	pending, total, err = svc.InviteImporters(ctx, 1, in, true, "", 0, 0, 50)
	if err != nil || total != 0 || len(pending) != 0 {
		t.Fatalf("request still pending total=%d rows=%d err=%v", total, len(pending), err)
	}
	if _, err := svc.Join(ctx, 4, 0, in); err != nil {
		t.Fatalf("member join: %v", err)
	}
}

func hasPendingJoin(up *mtproto.Updates, userID int64) bool {
	if up == nil {
		return false
	}
	for _, one := range up.GetUpdates() {
		if one.GetPredicateName() != mtproto.Predicate_updatePendingJoinRequests {
			continue
		}
		if err := one.Encode(mtproto.NewEncodeBuf(256), 232); err != nil {
			return false
		}
		for _, id := range one.GetRecentRequesters() {
			if id == userID && one.GetRequestsPending() > 0 {
				return true
			}
		}
	}
	return false
}
