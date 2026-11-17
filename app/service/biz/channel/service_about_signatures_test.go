package channel

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestEditAboutAndAdminLog(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true, About: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditAbout(ctx, 2, 0, in, "nope"); err != mtproto.ErrChatAdminRequired {
		t.Fatalf("member edit: %v", err)
	}
	if _, err := svc.EditAbout(ctx, 1, 0, in, strings.Repeat("a", 256)); err != mtproto.ErrChatAboutTooLong {
		t.Fatalf("too long: %v", err)
	}
	if _, err := svc.EditAbout(ctx, 1, 0, in, "hello"); err != mtproto.ErrChatAboutNotModified {
		t.Fatalf("same text: %v", err)
	}
	if _, err := svc.EditAbout(ctx, 1, 0, in, "updated"); err != nil {
		t.Fatal(err)
	}
	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil || full.FullChat.GetAbout() != "updated" {
		t.Fatalf("about %q %v", full.GetFullChat().GetAbout(), err)
	}
	if _, err := svc.EditAbout(ctx, 1, 0, in, ""); err != nil {
		t.Fatal(err)
	}
	full, _ = svc.GetFullChannel(ctx, 1, in)
	if full.FullChat.GetAbout() != "" {
		t.Fatalf("cleared about %q", full.FullChat.GetAbout())
	}

	log, err := svc.GetAdminLog(ctx, 1, in, "", nil, nil, 0, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	var sawAbout bool
	for _, ev := range log.GetEvents() {
		if ev.GetAction().GetPredicateName() == mtproto.Predicate_channelAdminLogEventActionChangeAbout &&
			ev.GetAction().GetNewValue_STRING() == "updated" &&
			ev.GetAction().GetPrevValue_STRING() == "hello" {
			sawAbout = true
		}
	}
	if !sawAbout {
		t.Fatalf("admin log missing changeAbout: %#v", log.GetEvents())
	}
}

func TestToggleSignaturesFlagsAndLog(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, false, false); status.Convert(err).Message() != "CHAT_NOT_MODIFIED" {
		t.Fatalf("unchanged: %v", err)
	}
	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, true, true); err != nil {
		t.Fatal(err)
	}
	visible := svc.ChatFor(ctx, 1, mustChannel(t, svc, ctx, ch.Id))
	if !visible.GetSignatures() || !visible.GetSignatureProfiles() {
		t.Fatalf("flags signatures=%v profiles=%v", visible.GetSignatures(), visible.GetSignatureProfiles())
	}
	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, true, false); err != nil {
		t.Fatal(err)
	}
	visible = svc.ChatFor(ctx, 1, mustChannel(t, svc, ctx, ch.Id))
	if !visible.GetSignatures() || visible.GetSignatureProfiles() {
		t.Fatalf("after profiles off signatures=%v profiles=%v", visible.GetSignatures(), visible.GetSignatureProfiles())
	}
	log, err := svc.GetAdminLog(ctx, 1, in, "", nil, nil, 0, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	var sig, prof bool
	for _, ev := range log.GetEvents() {
		switch ev.GetAction().GetPredicateName() {
		case mtproto.Predicate_channelAdminLogEventActionToggleSignatures:
			sig = mtproto.FromBool(ev.GetAction().GetNewValue_BOOL())
		case mtproto.Predicate_channelAdminLogEventActionToggleSignatureProfiles:
			if mtproto.FromBool(ev.GetAction().GetNewValue_BOOL()) {
				prof = true
			}
		}
	}
	if !sig || !prof {
		t.Fatalf("admin log signatures=%v profiles=%v events=%d", sig, prof, len(log.GetEvents()))
	}
}

func TestSignedChannelPostSetsPostAuthor(t *testing.T) {
	svc, ctx := newTest(t)
	prev := LookupPosterName
	LookupPosterName = func(_ context.Context, userID int64) string {
		if userID == 1 {
			return FormatPosterName("Ada", "Lovelace")
		}
		return ""
	}
	t.Cleanup(func() { LookupPosterName = prev })

	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, true, false); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 7,
		Message: &mtproto.Message{
			Message:    "hello",
			Date:       10,
			PostAuthor: &wrapperspb.StringValue{Value: "spoof"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msg := sent.Updates[0].GetMessage_MESSAGE()
	if msg.GetPostAuthor().GetValue() != "Ada Lovelace" {
		t.Fatalf("post_author = %q", msg.GetPostAuthor().GetValue())
	}
	if msg.GetFromId().GetUserId() != 1 {
		t.Fatalf("from_id = %#v", msg.GetFromId())
	}

	edited, err := svc.Edit(ctx, 1, ch.Id, 0, msg.GetId(), func(m *mtproto.Message) {
		m.Message = "hello!"
		m.PostAuthor = &wrapperspb.StringValue{Value: "edited-spoof"}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := edited.Updates[0].GetMessage_MESSAGE().GetPostAuthor().GetValue(); got != "Ada Lovelace" {
		t.Fatalf("edited post_author = %q", got)
	}

	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, false, false); err != nil {
		t.Fatal(err)
	}
	plain, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 8,
		Message:  &mtproto.Message{Message: "unsigned", Date: 11, PostAuthor: &wrapperspb.StringValue{Value: "spoof"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if plain.Updates[0].GetMessage_MESSAGE().GetPostAuthor() != nil {
		t.Fatalf("unsigned post_author = %#v", plain.Updates[0].GetMessage_MESSAGE().GetPostAuthor())
	}
}

func TestSignedPostIgnoresClientAuthorWhenNameIsEmpty(t *testing.T) {
	svc, ctx := newTest(t)
	prev := LookupPosterName
	LookupPosterName = func(context.Context, int64) string { return "" }
	t.Cleanup(func() { LookupPosterName = prev })

	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, true, false); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 7,
		Message:  &mtproto.Message{Message: "hello", Date: 10, PostAuthor: &wrapperspb.StringValue{Value: "spoof"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msg := sent.Updates[0].GetMessage_MESSAGE()
	if msg.GetPostAuthor() != nil {
		t.Fatalf("empty name kept %q", msg.GetPostAuthor().GetValue())
	}
	if msg.GetFromId().GetUserId() != 1 {
		t.Fatalf("from_id = %#v", msg.GetFromId())
	}
}

func TestAdminRankReplacesNameAndEditKeepsOriginalAuthor(t *testing.T) {
	svc, ctx := newTest(t)
	prev := LookupPosterName
	LookupPosterName = func(_ context.Context, userID int64) string {
		switch userID {
		case 1:
			return FormatPosterName("Ada", "Lovelace")
		case 2:
			return FormatPosterName("Grace", "Hopper")
		default:
			return ""
		}
	}
	t.Cleanup(func() { LookupPosterName = prev })

	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		PostMessages: true,
		EditMessages: true,
	}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 2, rights, "Editor"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, true, false); err != nil {
		t.Fatal(err)
	}

	sent, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{
		RandomID: 9,
		Message:  &mtproto.Message{Message: "desk", Date: 12, PostAuthor: &wrapperspb.StringValue{Value: "spoof"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msg := sent.Updates[0].GetMessage_MESSAGE()
	if msg.GetPostAuthor().GetValue() != "Editor" {
		t.Fatalf("rank signature = %q", msg.GetPostAuthor().GetValue())
	}

	byName, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 10,
		Message: &mtproto.Message{
			Message:    "named",
			Date:       13,
			PostAuthor: &wrapperspb.StringValue{Value: "spoof"},
			FwdFrom: mtproto.MakeTLMessageFwdHeader(&mtproto.MessageFwdHeader{
				PostAuthor: &wrapperspb.StringValue{Value: "Old Name"},
				Date:       1,
			}).To_MessageFwdHeader(),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	fwd := byName.Updates[0].GetMessage_MESSAGE()
	if fwd.GetPostAuthor().GetValue() != "Ada Lovelace" {
		t.Fatalf("forwarder signature = %q", fwd.GetPostAuthor().GetValue())
	}
	if fwd.GetFwdFrom().GetPostAuthor().GetValue() != "Old Name" {
		t.Fatalf("fwd_from.post_author = %q", fwd.GetFwdFrom().GetPostAuthor().GetValue())
	}

	edited, err := svc.Edit(ctx, 2, ch.Id, 0, fwd.GetId(), func(m *mtproto.Message) {
		m.Message = "named!"
		m.PostAuthor = &wrapperspb.StringValue{Value: "editor-spoof"}
	})
	if err != nil {
		t.Fatal(err)
	}
	got := edited.Updates[0].GetMessage_MESSAGE()
	if got.GetPostAuthor().GetValue() != "Ada Lovelace" {
		t.Fatalf("edit kept %q", got.GetPostAuthor().GetValue())
	}
	if got.GetFwdFrom().GetPostAuthor().GetValue() != "Old Name" {
		t.Fatalf("edit fwd_from = %q", got.GetFwdFrom().GetPostAuthor().GetValue())
	}
}

func TestEditKeepsSignatureAfterAuthorRename(t *testing.T) {
	svc, ctx := newTest(t)
	name := "Ada Lovelace"
	prev := LookupPosterName
	LookupPosterName = func(context.Context, int64) string { return name }
	t.Cleanup(func() { LookupPosterName = prev })

	ch, _ := signedChannel(t, svc, ctx)
	msg := sendSigned(t, svc, ctx, ch.Id, "hello", 10)
	if msg.GetPostAuthor().GetValue() != "Ada Lovelace" || msg.GetFromId().GetUserId() != 1 {
		t.Fatalf("sent author %q from %d", msg.GetPostAuthor().GetValue(), msg.GetFromId().GetUserId())
	}
	name = "Renamed"
	edited, err := svc.Edit(ctx, 1, ch.Id, 0, msg.GetId(), func(m *mtproto.Message) {
		m.Message = "hello!"
		m.PostAuthor = &wrapperspb.StringValue{Value: "spoof"}
		m.FromId = mtproto.MakePeerUser(9)
	})
	if err != nil {
		t.Fatal(err)
	}
	got := edited.Updates[0].GetMessage_MESSAGE()
	if got.GetMessage() != "hello!" || got.GetPostAuthor().GetValue() != "Ada Lovelace" || got.GetFromId().GetUserId() != 1 {
		t.Fatalf("after rename message %q author %q from %d", got.GetMessage(), got.GetPostAuthor().GetValue(), got.GetFromId().GetUserId())
	}
}

func TestEditKeepsSignatureAfterSignaturesTurnedOff(t *testing.T) {
	svc, ctx := newTest(t)
	prev := LookupPosterName
	LookupPosterName = func(context.Context, int64) string { return "Ada Lovelace" }
	t.Cleanup(func() { LookupPosterName = prev })

	ch, in := signedChannel(t, svc, ctx)
	msg := sendSigned(t, svc, ctx, ch.Id, "hello", 10)
	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, false, false); err != nil {
		t.Fatal(err)
	}
	edited, err := svc.Edit(ctx, 1, ch.Id, 0, msg.GetId(), func(m *mtproto.Message) {
		m.Message = "hello!"
		m.PostAuthor = nil
		m.FromId = nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := edited.Updates[0].GetMessage_MESSAGE()
	if got.GetPostAuthor().GetValue() != "Ada Lovelace" || got.GetFromId().GetUserId() != 1 {
		t.Fatalf("signatures off then edit: author %q from %#v", got.GetPostAuthor().GetValue(), got.GetFromId())
	}
}

func TestEditLeavesUnsignedPostUnsignedAfterSignaturesTurnedOn(t *testing.T) {
	svc, ctx := newTest(t)
	prev := LookupPosterName
	LookupPosterName = func(context.Context, int64) string { return "Ada Lovelace" }
	t.Cleanup(func() { LookupPosterName = prev })

	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 21,
		Message:  &mtproto.Message{Message: "plain", Date: 10, PostAuthor: &wrapperspb.StringValue{Value: "spoof"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msg := sent.Updates[0].GetMessage_MESSAGE()
	if msg.GetPostAuthor() != nil || msg.GetFromId() != nil {
		t.Fatalf("unsigned send author %#v from %#v", msg.GetPostAuthor(), msg.GetFromId())
	}
	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, true, false); err != nil {
		t.Fatal(err)
	}
	edited, err := svc.Edit(ctx, 1, ch.Id, 0, msg.GetId(), func(m *mtproto.Message) {
		m.Message = "plain!"
		m.PostAuthor = &wrapperspb.StringValue{Value: "Ada Lovelace"}
		m.FromId = mtproto.MakePeerUser(1)
	})
	if err != nil {
		t.Fatal(err)
	}
	got := edited.Updates[0].GetMessage_MESSAGE()
	if got.GetMessage() != "plain!" || got.GetPostAuthor() != nil || got.GetFromId() != nil {
		t.Fatalf("unsigned stayed author %#v from %#v", got.GetPostAuthor(), got.GetFromId())
	}
}

func TestRewriteBodyKeepsSignature(t *testing.T) {
	svc, ctx := newTest(t)
	name := "Ada Lovelace"
	prev := LookupPosterName
	LookupPosterName = func(context.Context, int64) string { return name }
	t.Cleanup(func() { LookupPosterName = prev })

	ch, in := signedChannel(t, svc, ctx)
	msg := sendSigned(t, svc, ctx, ch.Id, "list", 10)
	name = "Renamed"
	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, false, false); err != nil {
		t.Fatal(err)
	}
	body := &mtproto.Message{
		Message:    "list!",
		Date:       msg.GetDate(),
		PostAuthor: &wrapperspb.StringValue{Value: "spoof"},
		FromId:     mtproto.MakePeerUser(9),
	}
	rewritten, err := svc.RewriteBody(ctx, 1, ch.Id, 0, msg.GetId(), body)
	if err != nil {
		t.Fatal(err)
	}
	got := rewritten.Updates[0].GetMessage_MESSAGE()
	if got.GetMessage() != "list!" || got.GetPostAuthor().GetValue() != "Ada Lovelace" || got.GetFromId().GetUserId() != 1 {
		t.Fatalf("rewrite author %q from %d message %q", got.GetPostAuthor().GetValue(), got.GetFromId().GetUserId(), got.GetMessage())
	}
}

func signedChannel(t *testing.T, svc *Service, ctx context.Context) (*mtproto.Chat, *mtproto.InputChannel) {
	t.Helper()
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, true, false); err != nil {
		t.Fatal(err)
	}
	return ch, in
}

func sendSigned(t *testing.T, svc *Service, ctx context.Context, channelID int64, text string, date int32) *mtproto.Message {
	t.Helper()
	sent, err := svc.Send(ctx, 1, channelID, 0, []Outgoing{{
		RandomID: int64(date),
		Message: &mtproto.Message{
			Message:    text,
			Date:       date,
			PostAuthor: &wrapperspb.StringValue{Value: "spoof"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return sent.Updates[0].GetMessage_MESSAGE()
}

func TestFormatPosterName(t *testing.T) {
	if FormatPosterName("Ada", "Lovelace") != "Ada Lovelace" || FormatPosterName("Ada", "") != "Ada" || FormatPosterName("", "Lovelace") != "Lovelace" || FormatPosterName(" ", " ") != "" {
		t.Fatalf("names %q %q %q %q", FormatPosterName("Ada", "Lovelace"), FormatPosterName("Ada", ""), FormatPosterName("", "Lovelace"), FormatPosterName(" ", " "))
	}
}

func TestPromoteAndBanWriteAdminLog(t *testing.T) {
	svc, ctx := newTest(t)
	created, _ := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Mods", Megagroup: true})
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{DeleteMessages: true}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 2, rights, "mod"); err != nil {
		t.Fatal(err)
	}
	ban := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{SendMessages: true}).To_ChatBannedRights()
	if _, err := svc.EditBanned(ctx, 1, 0, in, 2, ban); err != nil {
		t.Fatal(err)
	}
	log, err := svc.GetAdminLog(ctx, 1, in, "", nil, nil, 0, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	var promote, banned bool
	for _, ev := range log.GetEvents() {
		switch ev.GetAction().GetPredicateName() {
		case mtproto.Predicate_channelAdminLogEventActionParticipantToggleAdmin:
			promote = true
		case mtproto.Predicate_channelAdminLogEventActionParticipantToggleBan:
			banned = true
		}
	}
	if !promote || !banned {
		t.Fatalf("promote=%v ban=%v", promote, banned)
	}
}
