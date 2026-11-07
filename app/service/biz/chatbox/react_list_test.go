package chatbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestDifferenceReactionsAreCatchUpNotALoop(t *testing.T) {
	b := newBook(nil)
	now := time.Unix(1_700_000_200, 0)
	b.now = func() time.Time { return now }
	ctx := context.Background()
	peer4 := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	if _, err := b.React(ctx, 4, peer4, 10, []string{"👍"}, nil, false); err != nil {
		t.Fatal(err)
	}
	ups := b.DifferenceReactions(ctx, 9, 0)
	if len(ups) != 1 || ups[0].GetPredicateName() != mtproto.Predicate_updateMessageReactions {
		t.Fatalf("catch-up %+v", ups)
	}
	rx := ups[0].GetReactions_MESSAGEREACTIONS()
	if len(rx.GetRecentReactions()) != 1 || !rx.GetRecentReactions()[0].GetUnread() {
		t.Fatalf("recent %#v", rx.GetRecentReactions())
	}
	again := b.DifferenceReactions(ctx, 9, ups[0].GetReactions_MESSAGEREACTIONS().GetRecentReactions()[0].GetDate())
	if len(again) != 0 {
		t.Fatalf("same date was returned again: %d", len(again))
	}
	if mine := b.DifferenceReactions(ctx, 4, 0); len(mine) != 0 {
		t.Fatal("the reactor was given their own reaction as unread")
	}
}

func TestPrivateReactionListAndRefresh(t *testing.T) {
	b := newBook(nil)
	now := time.Unix(1_700_000_100, 0)
	b.now = func() time.Time { return now }
	ctx := context.Background()
	// 4 and 9 are the two sides of one private chat. Both must see the reactions.
	peer4 := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	peer9 := mtproto.MakePeerUtil(mtproto.PEER_USER, 4)

	if _, err := b.React(ctx, 4, peer4, 10, []string{"👍"}, nil, false); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if _, err := b.React(ctx, 9, peer9, 10, []string{"👍", "❤️"}, nil, true); err != nil {
		t.Fatal(err)
	}

	up, err := b.ReactionsFor(ctx, 9, peer9, []int32{10})
	if err != nil {
		t.Fatal(err)
	}
	rx := up.Updates[0].GetReactions_MESSAGEREACTIONS()
	if !rx.GetCanSeeList() || rx.GetReactionsAsTags() || len(rx.GetResults()) != 2 {
		t.Fatalf("summary %#v", rx)
	}
	if len(rx.GetRecentReactions()) != 3 {
		t.Fatalf("recent %d", len(rx.GetRecentReactions()))
	}
	var thumb, heart *mtproto.ReactionCount
	for _, rc := range rx.GetResults() {
		switch rc.GetReaction_STRING() {
		case "👍":
			thumb = rc
		case "❤️":
			heart = rc
		}
	}
	if thumb.GetCount() != 2 || thumb.GetChosenOrder().GetValue() != 0 {
		t.Fatalf("thumb %#v", thumb)
	}
	if heart.GetCount() != 1 || heart.GetChosenOrder().GetValue() != 1 {
		t.Fatalf("heart %#v", heart)
	}

	emoji := mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "❤️"}).To_Reaction()
	filtered, err := b.ReactionList(ctx, 9, peer9, 10, emoji, "", 10)
	if err != nil || filtered.Count != 1 || filtered.Reactions[0].GetPeerId().GetUserId() != 9 || !filtered.Reactions[0].GetMy() || !filtered.Reactions[0].GetBig() {
		t.Fatalf("filter %#v %v", filtered, err)
	}

	page, err := b.ReactionList(ctx, 4, peer4, 10, nil, "", 1)
	if err != nil || page.Count != 3 || page.GetNextOffset().GetValue() != "1" || len(page.Reactions) != 1 {
		t.Fatalf("page %#v %v", page, err)
	}
	if page.Reactions[0].GetPeerId().GetUserId() != 9 || !page.Reactions[0].GetUnread() {
		t.Fatalf("first row %#v", page.Reactions[0])
	}
	rest, err := b.ReactionList(ctx, 4, peer4, 10, nil, page.GetNextOffset().GetValue(), 10)
	if err != nil || rest.Count != 3 || rest.NextOffset != nil || len(rest.Reactions) != 2 {
		t.Fatalf("rest %#v %v", rest, err)
	}
	if _, err := b.ReactionList(ctx, 4, peer4, 10, nil, "nope", 10); err != mtproto.ErrOffsetInvalid {
		t.Fatalf("offset: %v", err)
	}
	if _, err := b.ReactionList(ctx, 4, peer4, 10, nil, strings.Repeat("1", 65), 10); err != mtproto.ErrNextOffsetInvalid {
		t.Fatalf("long offset: %v", err)
	}
	if _, err := b.ReactionList(ctx, 4, peer4, 0, nil, "", 10); err != mtproto.ErrMsgIdInvalid {
		t.Fatalf("msg: %v", err)
	}
	if _, err := b.ReactionsFor(ctx, 4, peer4, nil); err != mtproto.ErrMsgIdInvalid {
		t.Fatalf("empty ids: %v", err)
	}

	saved := &mtproto.PeerUtil{PeerType: mtproto.PEER_SELF, PeerId: 3, SelfId: 3}
	sent, err := b.React(ctx, 3, saved, 2, []string{"🔥"}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	tags := sent.Updates[0].GetReactions_MESSAGEREACTIONS()
	if !tags.GetReactionsAsTags() || !tags.GetCanSeeList() {
		t.Fatalf("saved tags %#v", tags)
	}
}
