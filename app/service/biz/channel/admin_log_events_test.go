package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestAdminLogRecordsJoinLeavePinEditDeleteInviteForum(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Log", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "logroom"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Join(ctx, 2, 0, in); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Leave(ctx, 2, 0, in); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 1, Message: &mtproto.Message{Message: "pin me", Date: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	if _, err := svc.PinMessage(ctx, 1, 0, ch.Id, msgID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Edit(ctx, 1, ch.Id, 0, msgID, func(m *mtproto.Message) { m.Message = "edited" }); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Delete(ctx, 1, ch.Id, 0, []int32{msgID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExportInvite(ctx, 1, in, ExportRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleForum(ctx, 1, in, true); err != nil {
		t.Fatal(err)
	}
	log, err := svc.GetAdminLog(ctx, 1, in, "", nil, nil, 0, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, ev := range log.GetEvents() {
		if ev.GetAction() != nil {
			got[ev.GetAction().GetPredicateName()] = true
		}
	}
	for _, name := range []string{
		mtproto.Predicate_channelAdminLogEventActionParticipantJoin,
		mtproto.Predicate_channelAdminLogEventActionParticipantLeave,
		mtproto.Predicate_channelAdminLogEventActionUpdatePinned,
		mtproto.Predicate_channelAdminLogEventActionEditMessage,
		mtproto.Predicate_channelAdminLogEventActionDeleteMessage,
		mtproto.Predicate_channelAdminLogEventActionExportedInviteEdit,
		mtproto.Predicate_channelAdminLogEventActionToggleForum,
	} {
		if !got[name] {
			t.Fatalf("missing %s in %v", name, got)
		}
	}
}
