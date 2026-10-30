package chatbox

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestQuickReplyCaptureListAndSend(t *testing.T) {
	ResetShared()
	b := Shared()
	ctx := context.Background()
	self := mtproto.MakePeerUtil(mtproto.PEER_SELF, 7)
	shortcut := mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{
		Shortcut: "Hello",
	}).To_InputQuickReplyShortcut()
	ups, err := b.CaptureQuickReply(ctx, 7, self, shortcut, []*mtproto.Message{{
		Message: "hello",
	}})
	if err != nil || len(ups.Updates) != 2 {
		t.Fatalf("capture %+v %v", ups, err)
	}
	page, err := b.QuickReplies(ctx, 7, 0)
	if err != nil || len(page.GetQuickReplies()) != 1 || page.GetQuickReplies()[0].GetShortcut() != "hello" {
		t.Fatalf("list %+v %v", page, err)
	}
	id := page.GetQuickReplies()[0].GetShortcutId()
	if page.GetQuickReplies()[0].GetCount() != 1 || page.GetMessages()[0].GetMessage() != "hello" {
		t.Fatalf("top %+v", page)
	}
	again, err := b.QuickReplies(ctx, 7, hashQuick(page.GetQuickReplies(), page.GetMessages()))
	if err != nil || again.GetPredicateName() != mtproto.Predicate_messages_quickRepliesNotModified {
		t.Fatalf("hash %+v %v", again, err)
	}
	free, err := b.CheckQuickReplyShortcut(ctx, 7, "hello")
	if err != nil || mtproto.FromBool(free) {
		t.Fatalf("taken %+v %v", free, err)
	}
	msgs, err := b.QuickReplyMessages(ctx, 7, id, nil, 0)
	if err != nil || len(msgs.GetMessages()) != 1 || msgs.GetMessages()[0].GetQuickReplyShortcutId().GetValue() != id {
		t.Fatalf("messages %+v %v", msgs, err)
	}
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	out, err := b.PrepareQuickReplySend(ctx, 7, peer, id, []int32{msgs.GetMessages()[0].GetId()}, []int64{44})
	if err != nil || out[0].GetMessage() != "hello" || out[0].GetQuickReplyShortcutId() != nil {
		t.Fatalf("prepare %+v %v", out, err)
	}
	if _, err := b.EditQuickReplyShortcut(ctx, 7, id, "later"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.EditQuickReplyShortcut(ctx, 7, id, "bad name"); err != mtproto.ErrShortcutInvalid {
		t.Fatalf("bad name: %v", err)
	}
	if _, err := b.DeleteQuickReplyMessages(ctx, 7, id, []int32{msgs.GetMessages()[0].GetId()}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.DeleteQuickReplyShortcut(ctx, 7, id); err != nil {
		t.Fatal(err)
	}
	empty, err := b.QuickReplies(ctx, 7, 0)
	if err != nil || len(empty.GetQuickReplies()) != 0 {
		t.Fatalf("deleted %+v %v", empty, err)
	}
	other := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	if _, err := b.CaptureQuickReply(ctx, 7, other, shortcut, []*mtproto.Message{{Message: "x"}}); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("peer: %v", err)
	}
}

func TestTranslateLexiconAndPeerFlag(t *testing.T) {
	ResetShared()
	b := Shared()
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 4)
	if _, err := TranslateText("hello", nil, "nope"); err != mtproto.ErrToLangInvalid {
		t.Fatalf("lang: %v", err)
	}
	zh, err := TranslateText("hello", nil, "zh-hans")
	if err != nil || zh.GetText() != "你好" {
		t.Fatalf("zh %+v %v", zh, err)
	}
	kept, err := TranslateText("hello", []*mtproto.MessageEntity{{
		PredicateName: mtproto.Predicate_messageEntityBold,
		Offset:        0,
		Length:        5,
	}}, "zh")
	if err != nil || kept.GetText() != "hello" || len(kept.GetEntities()) != 1 {
		t.Fatalf("entities %+v %v", kept, err)
	}
	en, err := TranslateText("你好", nil, "en")
	if err != nil || en.GetText() != "hello" {
		t.Fatalf("en %+v %v", en, err)
	}
	same, err := TranslateText("hello", nil, "fr")
	if err != nil || same.GetText() != "hello" {
		t.Fatalf("fr %+v %v", same, err)
	}
	b.RememberPlain(ctx, 3, peer, 11, "hello", nil)
	text, _, ok := b.LookupPlain(ctx, 3, peer, 11)
	if !ok || text != "hello" {
		t.Fatalf("lookup %q %v", text, ok)
	}
	if err := b.TogglePeerTranslations(ctx, 3, peer, true); err != nil {
		t.Fatal(err)
	}
	if !b.TranslationsDisabled(3, peer) {
		t.Fatal("flag not set")
	}
	if err := b.TogglePeerTranslations(ctx, 3, peer, false); err != nil || b.TranslationsDisabled(3, peer) {
		t.Fatal("flag not cleared")
	}
}

func TestSavedTagsFromSelfReactions(t *testing.T) {
	ResetShared()
	b := Shared()
	ctx := context.Background()
	self := mtproto.MakePeerUtil(mtproto.PEER_SELF, 5)
	reaction := mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction()
	if _, err := b.React(ctx, 5, self, 8, []string{"👍"}, nil, false); err != nil {
		t.Fatal(err)
	}
	tags, err := b.SavedReactionTags(ctx, 5, nil, 0)
	if err != nil || len(tags.GetTags()) != 1 || tags.GetTags()[0].GetCount() != 1 {
		t.Fatalf("tags %+v %v", tags, err)
	}
	if err := b.UpdateSavedReactionTag(ctx, 5, reaction, wrapperspb.String("yes")); err != nil {
		t.Fatal(err)
	}
	titled, err := b.SavedReactionTags(ctx, 5, self, 0)
	if err != nil || titled.GetTags()[0].GetTitle().GetValue() != "yes" {
		t.Fatalf("title %+v %v", titled, err)
	}
	other := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	filtered, err := b.SavedReactionTags(ctx, 5, other, 0)
	if err != nil || len(filtered.GetTags()) != 0 {
		t.Fatalf("filter %+v %v", filtered, err)
	}
	if err := b.UpdateSavedReactionTag(ctx, 5, reaction, nil); err != nil {
		t.Fatal(err)
	}
	cleared, err := b.SavedReactionTags(ctx, 5, nil, 0)
	if err != nil || cleared.GetTags()[0].GetTitle() != nil {
		t.Fatalf("cleared %+v %v", cleared, err)
	}
}

func TestReportTreeAndPaidMedia(t *testing.T) {
	ResetShared()
	b := Shared()
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	choose, err := b.ReportMessages(ctx, 3, peer, []int32{4}, nil, "")
	if err != nil || choose.GetPredicateName() != mtproto.Predicate_reportResultChooseOption || len(choose.GetOptions()) < 2 {
		t.Fatalf("choose %+v %v", choose, err)
	}
	ask, err := b.ReportMessages(ctx, 3, peer, []int32{4}, []byte("other"), "")
	if err != nil || ask.GetPredicateName() != mtproto.Predicate_reportResultAddComment {
		t.Fatalf("comment %+v %v", ask, err)
	}
	done, err := b.ReportMessages(ctx, 3, peer, []int32{4}, []byte("other"), "spam text here")
	if err != nil || done.GetPredicateName() != mtproto.Predicate_reportResultReported {
		t.Fatalf("reported %+v %v", done, err)
	}
	if _, err := b.ReportMessages(ctx, 3, peer, []int32{4}, []byte("nope"), ""); err != mtproto.ErrOptionInvalid {
		t.Fatalf("option: %v", err)
	}
	if err := b.ReportDelivery(ctx, 3, peer, []int32{4}, true); err != nil {
		t.Fatal(err)
	}
	if err := b.ReportMusicListen(ctx, 3, mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 15}).To_InputDocument(), 3); err != nil {
		t.Fatal(err)
	}
	if len(b.Reports()) != 3 {
		t.Fatalf("reports %d", len(b.Reports()))
	}
	media := mtproto.MakeTLMessageExtendedMedia(&mtproto.MessageExtendedMedia{
		Media: mtproto.MakeTLMessageMediaEmpty(&mtproto.MessageMedia{}).To_MessageMedia(),
	}).To_MessageExtendedMedia()
	b.NoteSent(ctx, 3, peer, mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Message_MESSAGE: mtproto.MakeTLMessage(&mtproto.Message{
			Id:      6,
			Message: "hidden",
			Media: mtproto.MakeTLMessageMediaPaidMedia(&mtproto.MessageMedia{
				ExtendedMedia_VECTORMESSAGEEXTENDEDMEDIA: []*mtproto.MessageExtendedMedia{media},
			}).To_MessageMedia(),
		}).To_Message(),
	}).To_Update()))
	got, err := b.ExtendedMedia(ctx, 3, peer, []int32{6})
	if err != nil || got.Updates[0].GetMsgId_INT32() != 6 || len(got.Updates[0].GetExtendedMedia_VECTORMESSAGEEXTENDEDMEDIA()) != 1 {
		t.Fatalf("extended %+v %v", got, err)
	}
	if _, err := b.ExtendedMedia(ctx, 3, peer, []int32{7}); err != mtproto.ErrMessageIdInvalid {
		t.Fatalf("missing: %v", err)
	}
}
