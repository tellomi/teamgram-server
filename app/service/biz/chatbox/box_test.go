package chatbox

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestScheduleHistorySendAndDelete(t *testing.T) {
	b := newBook(nil)
	b.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	when := int32(b.now().Add(time.Hour).Unix())
	ups, err := b.Schedule(ctx, 3, peer, when, []*mtproto.Message{{
		Message: "later",
	}})
	if err != nil || len(ups.Updates) != 1 || ups.Updates[0].GetMessage_MESSAGE().GetDate() != when {
		t.Fatalf("schedule %+v %v", ups, err)
	}
	if _, err := b.Schedule(ctx, 3, peer, int32(b.now().Unix())-1, []*mtproto.Message{{Message: "past"}}); err != mtproto.ErrScheduleDateInvalid {
		t.Fatalf("past date: %v", err)
	}
	page, err := b.ScheduledHistory(ctx, 3, peer, 0)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].GetMessage() != "later" {
		t.Fatalf("history %+v %v", page, err)
	}
	again, err := b.ScheduledHistory(ctx, 3, peer, hashMessages(page.Messages))
	if err != nil || again.GetPredicateName() != mtproto.Predicate_messages_messagesNotModified {
		t.Fatalf("hash %+v %v", again, err)
	}
	id := page.Messages[0].GetId()
	got, err := b.ScheduledByID(ctx, 3, peer, []int32{id, 99})
	if err != nil || got.Messages[0].GetId() != id || got.Messages[1].GetPredicateName() != mtproto.Predicate_messageEmpty {
		t.Fatalf("by id %+v %v", got, err)
	}
	taken, err := b.TakeScheduled(ctx, 3, peer, []int32{id})
	if err != nil || taken[0].GetId() != id || taken[0].GetMessage() != "later" {
		t.Fatalf("take %+v %v", taken, err)
	}
	if _, err := b.TakeScheduled(ctx, 3, peer, []int32{id}); err != mtproto.ErrMessageIdInvalid {
		t.Fatalf("second take: %v", err)
	}
	b.RestoreScheduled(ctx, 3, peer, taken)
	if _, err := b.DeleteScheduled(ctx, 3, peer, []int32{id}); err != nil {
		t.Fatal(err)
	}
	empty, err := b.ScheduledHistory(ctx, 3, peer, 0)
	if err != nil || len(empty.Messages) != 0 {
		t.Fatalf("deleted %+v %v", empty, err)
	}
}

func TestPollVoteUnreadAndOpenAnswer(t *testing.T) {
	b := newBook(nil)
	ctx := context.Background()
	media, err := b.CreatePoll(ctx, 1, &mtproto.Poll{
		PublicVoters: true,
		OpenAnswers:  true,
		Question:     "Pick",
		Answers: []*mtproto.PollAnswer{
			mtproto.MakePollAnswerWithText("a", []byte("0")),
			mtproto.MakePollAnswerWithText("b", []byte("1")),
		},
	}, nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	pollID := media.GetPoll().GetId()
	b.BindPoll(ctx, mtproto.PEER_USER, 2, 7, pollID)
	ups, err := b.Vote(ctx, 4, mtproto.PEER_USER, 2, 7, 0, [][]byte{[]byte("0")})
	if err != nil || ups.Updates[0].GetResults().GetTotalVoters().GetValue() != 1 {
		t.Fatalf("vote %+v %v", ups, err)
	}
	if !ups.Updates[0].GetResults().GetResults()[0].GetChosen() {
		t.Fatal("chosen not set")
	}
	unread, err := b.UnreadPollMessages(ctx, 1, mtproto.PEER_USER, 2, 0, 0, 10, 0, 0)
	if err != nil || len(unread) != 1 || unread[0].GetId() != 7 {
		t.Fatalf("unread %+v %v", unread, err)
	}
	if err := b.ReadPollVotes(ctx, 1, mtproto.PEER_USER, 2); err != nil {
		t.Fatal(err)
	}
	unread, _ = b.UnreadPollMessages(ctx, 1, mtproto.PEER_USER, 2, 0, 0, 10, 0, 0)
	if len(unread) != 0 {
		t.Fatalf("still unread %+v", unread)
	}
	list, err := b.PollVotes(ctx, 1, mtproto.PEER_USER, 2, 7, pollID, []byte("0"), "", 10)
	if err != nil || list.GetCount() != 1 || list.GetVotes_VECTORMESSAGEPEERVOTE()[0].GetPeer().GetUserId() != 4 {
		t.Fatalf("votes %+v %v", list, err)
	}
	if _, err := b.Vote(ctx, 4, mtproto.PEER_USER, 2, 7, pollID, [][]byte{[]byte("9")}); err != mtproto.ErrPollOptionInvalid {
		t.Fatalf("bad option: %v", err)
	}
	if _, err := b.AddPollAnswer(ctx, 4, mtproto.PEER_USER, 2, 7, pollID, mtproto.MakePollAnswerWithText("c", []byte("2"))); err != nil {
		t.Fatal(err)
	}
	if _, err := b.DeletePollAnswer(ctx, 4, mtproto.PEER_USER, 2, 7, pollID, []byte("2")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreatePoll(ctx, 1, &mtproto.Poll{Question: ""}, nil, "", nil); err != mtproto.ErrPollQuestionInvalid {
		t.Fatal("empty question accepted")
	}
}

func TestPrivateUnreadReactionsAndDefault(t *testing.T) {
	b := newBook(nil)
	b.now = func() time.Time { return time.Unix(1_700_000_100, 0) }
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 8)
	other := mtproto.MakePeerUtil(mtproto.PEER_USER, 2)
	if _, err := b.React(ctx, 2, peer, 4, []string{"👍"}, nil, false); err != nil {
		t.Fatal(err)
	}
	unread, err := b.UnreadReactions(ctx, 8, other, 0, 0, 10, 0, 0)
	if err != nil || len(unread) != 1 || unread[0].GetReactions().GetResults()[0].GetCount() != 1 {
		t.Fatalf("unread %+v %v", unread, err)
	}
	own, _ := b.UnreadReactions(ctx, 2, peer, 0, 0, 10, 0, 0)
	if len(own) != 0 {
		t.Fatalf("own reaction counted as unread %+v", own)
	}
	if err := b.ReadReactions(ctx, 8, other); err != nil {
		t.Fatal(err)
	}
	unread, _ = b.UnreadReactions(ctx, 8, other, 0, 0, 10, 0, 0)
	if len(unread) != 0 {
		t.Fatalf("read did not clear %+v", unread)
	}
	if err := b.SetDefaultReaction(ctx, 1, mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "❤"}).To_Reaction()); err != nil {
		t.Fatal(err)
	}
	if got := b.DefaultReaction(ctx, 1); got.GetEmoticon() != "❤" {
		t.Fatalf("default %#v", got)
	}
	if err := b.SetDefaultReaction(ctx, 1, nil); err != mtproto.ErrReactionEmpty {
		t.Fatalf("empty reaction: %v", err)
	}
	emo, doc, err := ParseReaction(mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "❤️"}).To_Reaction())
	if err != nil || emo != "❤" || doc != 0 {
		t.Fatalf("parse heart %q %d %v", emo, doc, err)
	}
	kept, _, err := ParseReaction(mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "❤️‍🔥"}).To_Reaction())
	if err != nil || kept != "❤️‍🔥" {
		t.Fatalf("parse zwj %q %v", kept, err)
	}
}

func TestWebPagePreview(t *testing.T) {
	restore := stubWebFetch(t, pageMeta{Title: "Hello", Description: "A page", Site: "Tellomi"})
	defer restore()
	page, err := WebPage("https://tellomi.example/a", 0)
	if err != nil || page.GetUrl_STRING() == "" || page.GetPredicateName() != mtproto.Predicate_webPage {
		t.Fatalf("page %+v %v", page, err)
	}
	if page.GetTitle().GetValue() != "Hello" || page.GetSiteName().GetValue() != "Tellomi" {
		t.Fatalf("meta title %q site %q", page.GetTitle().GetValue(), page.GetSiteName().GetValue())
	}
	same, err := WebPage("https://tellomi.example/a", page.GetHash())
	if err != nil || same.GetPredicateName() != mtproto.Predicate_webPageNotModified {
		t.Fatalf("not modified %+v %v", same, err)
	}
	if _, err := WebPage("not a url", 0); err != mtproto.ErrWebpageCurlFailed {
		t.Fatalf("bad url: %v", err)
	}
	media := WebPagePreview("see https://tellomi.example/a today", nil)
	if media.GetPredicateName() != mtproto.Predicate_messageMediaWebPage {
		t.Fatalf("preview %#v", media)
	}
	empty := WebPagePreview("no link", nil)
	if empty.GetPredicateName() != mtproto.Predicate_messageMediaEmpty {
		t.Fatalf("empty preview %#v", empty)
	}
}

func TestInputWebPageIsDecodableCard(t *testing.T) {
	restore := stubWebFetch(t, pageMeta{Title: "Apple", Site: "Apple"})
	defer restore()
	in := mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{
		Url:             "https://apple.com",
		ForceLargeMedia: true,
	}).To_InputMedia()
	media, err := MediaForInputWebPage(in)
	if err != nil {
		t.Fatal(err)
	}
	if media.GetPredicateName() != mtproto.Predicate_messageMediaWebPage {
		t.Fatalf("predicate %s", media.GetPredicateName())
	}
	if mtproto.GetClazzID(media.GetPredicateName(), mtproto.Layer) == 0 {
		t.Fatal("layer 232 would rewrite this media to messageMediaUnsupported")
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := media.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if len(raw) < 12 {
		t.Fatalf("short encode %d", len(raw))
	}
	mediaCtor := binary.LittleEndian.Uint32(raw[0:4])
	if mediaCtor != 0xddf10c3b {
		t.Fatalf("messageMedia constructor %08x, want messageMediaWebPage#ddf10c3b (not messageMediaUnsupported#9f84f49e)", mediaCtor)
	}
	pageCtor := binary.LittleEndian.Uint32(raw[8:12])
	if pageCtor != 0xe89c45b2 {
		t.Fatalf("webPage constructor %08x, want webPage#e89c45b2", pageCtor)
	}
	decoded := &mtproto.MessageMedia{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if decoded.GetPredicateName() != mtproto.Predicate_messageMediaWebPage {
		t.Fatalf("decoded %s", decoded.GetPredicateName())
	}
	page := decoded.GetWebpage()
	if page.GetPredicateName() != mtproto.Predicate_webPage || page.GetUrl_STRING() != "https://apple.com" {
		t.Fatalf("page %+v", page)
	}
	if !decoded.GetForceLargeMedia() {
		t.Fatal("force_large_media dropped")
	}

	skipped, err := MediaForInputWebPage(mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{
		Url:      "not a url",
		Optional: true,
	}).To_InputMedia())
	if err != nil || skipped != nil {
		t.Fatalf("optional skip %+v %v", skipped, err)
	}
	if _, err := MediaForInputWebPage(mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{
		Url: "not a url",
	}).To_InputMedia()); err != mtproto.ErrWebpageCurlFailed {
		t.Fatalf("required bad url: %v", err)
	}
}
