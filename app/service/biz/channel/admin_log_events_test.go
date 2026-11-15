package channel

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/brand"
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
	exported, err := svc.ExportInvite(ctx, 1, in, ExportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	title := "named"
	if _, err := svc.EditInvite(ctx, 1, in, EditRequest{Link: exported.GetLink(), Title: &title}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteInvite(ctx, 1, in, exported.GetLink()); err != nil {
		t.Fatal(err)
	}
	joinLink, err := svc.ExportInvite(ctx, 1, in, ExportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.TrimPrefix(joinLink.GetLink(), brand.LinkBase()+"/+")
	if _, err := svc.ImportInvite(ctx, 3, 0, hash); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.UnpinAll(ctx, 1, 0, ch.Id); err != nil {
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
		action := ev.GetAction()
		if action == nil {
			continue
		}
		got[action.GetPredicateName()] = true
		buf := mtproto.NewEncodeBuf(1024)
		if err := action.Encode(buf, 232); err != nil {
			t.Fatalf("encode %s: %v", action.GetPredicateName(), err)
		}
	}
	for _, name := range []string{
		mtproto.Predicate_channelAdminLogEventActionParticipantJoin,
		mtproto.Predicate_channelAdminLogEventActionParticipantLeave,
		mtproto.Predicate_channelAdminLogEventActionUpdatePinned,
		mtproto.Predicate_channelAdminLogEventActionEditMessage,
		mtproto.Predicate_channelAdminLogEventActionDeleteMessage,
		mtproto.Predicate_channelAdminLogEventActionExportedInviteEdit,
		mtproto.Predicate_channelAdminLogEventActionExportedInviteDelete,
		mtproto.Predicate_channelAdminLogEventActionParticipantJoinByInvite,
		mtproto.Predicate_channelAdminLogEventActionToggleForum,
	} {
		if !got[name] {
			t.Fatalf("missing %s in %v", name, got)
		}
	}
	forums, err := svc.GetAdminLog(ctx, 1, in, "", &mtproto.ChannelAdminLogEventsFilter{Forums: true}, nil, 0, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(forums.GetEvents()) == 0 {
		t.Fatal("forums filter empty")
	}
	for _, ev := range forums.GetEvents() {
		if ev.GetAction().GetPredicateName() != mtproto.Predicate_channelAdminLogEventActionToggleForum {
			t.Fatalf("forums filter %s", ev.GetAction().GetPredicateName())
		}
	}
	invites, err := svc.GetAdminLog(ctx, 1, in, "", &mtproto.ChannelAdminLogEventsFilter{Invites: true}, nil, 0, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(invites.GetEvents()) == 0 {
		t.Fatal("invites filter empty")
	}
	for _, ev := range invites.GetEvents() {
		switch ev.GetAction().GetPredicateName() {
		case mtproto.Predicate_channelAdminLogEventActionExportedInviteEdit, mtproto.Predicate_channelAdminLogEventActionExportedInviteDelete:
		default:
			t.Fatalf("invites filter %s", ev.GetAction().GetPredicateName())
		}
	}
}
