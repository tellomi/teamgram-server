package book

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestFeedInterestsAndReadCursor(t *testing.T) {
	b := New()
	if _, err := b.Interests(0, 0); status.Convert(err).Message() != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
	cats, err := b.Interests(4, 0)
	if err != nil || len(cats.GetCategories()) != len(Catalog) || len(cats.GetSelected()) != 0 {
		t.Fatalf("catalog %#v %v", cats, err)
	}
	you, err := b.ForYou(4, &mtproto.TLFeedGetForYou{Limit: 10})
	if err != nil || len(you.GetItems()) != 1 || you.Items[0].GetPredicateName() != mtproto.Predicate_feedItemInterestCategories {
		t.Fatalf("picker %#v %v", you, err)
	}
	if _, err := b.SetInterests(4, []int32{99}); status.Convert(err).Message() != "INTEREST_CATEGORY_INVALID" {
		t.Fatal(err)
	}
	if _, err := b.SetInterests(4, []int32{2, 2, 5}); err != nil {
		t.Fatal(err)
	}
	you, err = b.ForYou(4, &mtproto.TLFeedGetForYou{HasQuery: true, Query: "tech", Limit: 10})
	if err != nil || len(you.GetItems()) != 0 {
		t.Fatalf("empty for you %#v %v", you, err)
	}
	if _, err := b.Feed(4, &mtproto.TLFeedGetFeed{FilterId: -1, Limit: 10}); status.Convert(err).Message() != "FILTER_ID_INVALID" {
		t.Fatal(err)
	}
	page, err := b.Feed(4, &mtproto.TLFeedGetFeed{FilterId: 0, Limit: 10})
	if err != nil || page.GetPredicateName() != mtproto.Predicate_feed_feedMessages || len(page.GetMessages()) != 0 || page.GetHasRead() {
		t.Fatalf("empty feed %#v %v", page, err)
	}
	pos := mtproto.MakeFeedPosition(30, 8, mtproto.MakePeerChannel(3))
	up, err := b.Read(4, 0, pos)
	if err != nil || up.GetSeq() != 0 {
		t.Fatalf("read %#v %v", up, err)
	}
	page, err = b.Feed(4, &mtproto.TLFeedGetFeed{FilterId: 0, Limit: 10, OffsetToMaxRead: true})
	if err != nil || !page.GetHasRead() || page.GetReadMaxPosition().GetId() != 8 {
		t.Fatalf("cursor %#v %v", page, err)
	}
}
