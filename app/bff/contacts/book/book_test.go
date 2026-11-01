package book

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestNoteRoundTripAndBounds(t *testing.T) {
	b := New(nil)
	ctx := context.Background()
	note := mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
		Text: "hi",
		Entities: []*mtproto.MessageEntity{
			{PredicateName: mtproto.Predicate_messageEntityBold, Offset: 0, Length: 2},
		},
	}).To_TextWithEntities()
	if err := b.PutNote(ctx, 1, 2, note); err != nil {
		t.Fatal(err)
	}
	got := b.Note(ctx, 1, 2)
	if got.GetText() != "hi" || len(got.GetEntities()) != 1 || got.GetEntities()[0].GetLength() != 2 {
		t.Fatalf("note %#v", got)
	}
	if b.Note(ctx, 2, 1) != nil {
		t.Fatal("notes are private to the owner")
	}
	if err := b.PutNote(ctx, 1, 2, &mtproto.TextWithEntities{}); err != nil {
		t.Fatal(err)
	}
	if b.Note(ctx, 1, 2) != nil {
		t.Fatal("empty note should clear")
	}

	long := make([]rune, NoteLimit+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateNote(&mtproto.TextWithEntities{Text: string(long)}); err != mtproto.ErrMessageTooLong {
		t.Fatalf("length: %v", err)
	}
	if err := ValidateNote(&mtproto.TextWithEntities{
		Text:     "ab",
		Entities: []*mtproto.MessageEntity{{Offset: 1, Length: 2}},
	}); err != mtproto.ErrEntityBoundsInvalid {
		t.Fatalf("bounds: %v", err)
	}
}

func TestTopPeersRankResetAndDisable(t *testing.T) {
	b := New(nil)
	b.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	ctx := context.Background()
	hits := []Hit{
		{Peer: Peer{Type: mtproto.PEER_USER, ID: 5}, Date: 100, Bot: false},
		{Peer: Peer{Type: mtproto.PEER_USER, ID: 6}, Date: 50, Bot: false},
		{Peer: Peer{Type: mtproto.PEER_USER, ID: 7}, Date: 80, Bot: true},
		{Peer: Peer{Type: mtproto.PEER_CHAT, ID: 9}, Date: 40},
		{Peer: Peer{Type: mtproto.PEER_CHANNEL, ID: 11}, Date: 30, Megagroup: true},
		{Peer: Peer{Type: mtproto.PEER_CHANNEL, ID: 12}, Date: 20},
	}
	ranked := b.Rank(ctx, 1, []string{CatCorrespondents, CatBotsPM, CatGroups, CatChannels}, hits, 0, 10)
	if len(ranked) != 4 || ranked[0].Peers[0].Peer.ID != 5 || ranked[0].Total != 2 {
		t.Fatalf("correspondents %#v", ranked)
	}
	if ranked[1].Cat != CatBotsPM || ranked[1].Peers[0].Peer.ID != 7 {
		t.Fatalf("bots %#v", ranked[1])
	}
	if ranked[2].Total != 2 || ranked[3].Peers[0].Peer.ID != 12 {
		t.Fatalf("groups/channels %#v", ranked[2:])
	}
	page := b.Rank(ctx, 1, []string{CatCorrespondents}, hits, 1, 1)
	if page[0].Total != 2 || len(page[0].Peers) != 1 || page[0].Peers[0].Peer.ID != 6 {
		t.Fatalf("page %#v", page)
	}
	hash := HashRanks(page)
	if hash == 0 {
		t.Fatal("hash")
	}
	if err := b.ResetTop(ctx, 1, CatCorrespondents, Peer{Type: mtproto.PEER_USER, ID: 5}); err != nil {
		t.Fatal(err)
	}
	after := b.Rank(ctx, 1, []string{CatCorrespondents}, hits, 0, 10)
	if after[0].Total != 1 || after[0].Peers[0].Peer.ID != 6 {
		t.Fatalf("reset %#v", after)
	}
	if err := b.Bump(ctx, 1, CatCorrespondents, Peer{Type: mtproto.PEER_USER, ID: 5}); err != nil {
		t.Fatal(err)
	}
	back := b.Rank(ctx, 1, []string{CatCorrespondents}, hits, 0, 10)
	if back[0].Total != 2 {
		t.Fatalf("bump should restore dialog score %#v", back)
	}
	if err := b.Bump(ctx, 1, CatPhoneCalls, Peer{Type: mtproto.PEER_USER, ID: 5}); err != nil {
		t.Fatal(err)
	}
	calls := b.Rank(ctx, 1, []string{CatPhoneCalls}, nil, 0, 10)
	if len(calls) != 1 || calls[0].Peers[0].Rating < 1 {
		t.Fatalf("calls %#v", calls)
	}
	if err := b.SetTopEnabled(ctx, 1, false); err != nil || b.TopEnabled(ctx, 1) {
		t.Fatal("disable")
	}
	if err := b.ResetTop(ctx, 1, "nope", Peer{ID: 1}); err != mtproto.ErrPeerIdInvalid {
		t.Fatal(err)
	}
}

func TestStoryReplaceAndLocated(t *testing.T) {
	b := New(nil)
	ctx := context.Background()
	added, removed, err := b.ReplaceStories(ctx, 3, []Peer{{Type: mtproto.PEER_USER, ID: 8}, {Type: mtproto.PEER_USER, ID: 9}}, 10)
	if err != nil || len(added) != 2 || len(removed) != 0 || !b.StoryBlocked(ctx, 3, 8) {
		t.Fatalf("add %#v %#v %v", added, removed, err)
	}
	added, removed, err = b.ReplaceStories(ctx, 3, []Peer{{Type: mtproto.PEER_USER, ID: 9}}, 11)
	if err != nil || len(added) != 0 || len(removed) != 1 || removed[0].ID != 8 || b.StoryBlocked(ctx, 3, 8) {
		t.Fatalf("replace %#v %#v", added, removed)
	}
	rows := b.StoryList(ctx, 3)
	if len(rows) != 1 || rows[0].Peer.ID != 9 || rows[0].Date != 10 {
		t.Fatalf("kept date %#v", rows)
	}

	if _, _, err := ParseGeo(nil); err != mtproto.ErrGeoPointInvalid {
		t.Fatal(err)
	}
	if _, _, err := ParseGeo(&mtproto.InputGeoPoint{PredicateName: mtproto.Predicate_inputGeoPoint, Lat: 100, Long: 0}); err != mtproto.ErrGeoPointInvalid {
		t.Fatal(err)
	}
	lat, long, err := ParseGeo(&mtproto.InputGeoPoint{PredicateName: mtproto.Predicate_inputGeoPoint, Lat: 31.2, Long: 121.5})
	if err != nil {
		t.Fatal(err)
	}
	now := int32(1_700_000_000)
	if err := b.Publish(ctx, 4, lat, long, ExpiresUnix(now, 60)); err != nil {
		t.Fatal(err)
	}
	if err := b.Publish(ctx, 5, lat+0.01, long, ExpiresUnix(now, 60)); err != nil {
		t.Fatal(err)
	}
	if err := b.Publish(ctx, 6, lat+2, long, ExpiresUnix(now, 60)); err != nil {
		t.Fatal(err)
	}
	near := b.Nearby(ctx, 4, lat, long, now)
	if len(near) != 1 || near[0].UserID != 5 || near[0].Distance <= 0 {
		t.Fatalf("nearby %#v", near)
	}
	if _, ok := b.SelfExpires(ctx, 4, now); !ok {
		t.Fatal("self")
	}
	if err := b.Hide(ctx, 4); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.SelfExpires(ctx, 4, now+10); ok {
		t.Fatal("hidden")
	}
	if got := ExpiresUnix(now, math.MaxInt32); got != int32(0x7fffffff) {
		t.Fatalf("never %d", got)
	}
}

func TestCategoryOf(t *testing.T) {
	cat := mtproto.MakeTLTopPeerCategoryPhoneCalls(&mtproto.TopPeerCategory{}).To_TopPeerCategory()
	if CategoryOf(cat) != CatPhoneCalls || !KnownCategory(CatBotsGuest) {
		t.Fatalf("%s", CategoryOf(cat))
	}
}
