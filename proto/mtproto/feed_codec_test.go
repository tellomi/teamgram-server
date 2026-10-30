package mtproto

import "testing"

func TestFeedRoundTrip(t *testing.T) {
	peer := MakePeerUser(7)
	pos := MakeFeedPosition(20, 4, peer)
	req := &TLFeedGetFeed{
		FilterId:        1,
		OffsetToMaxRead: true,
		OffsetPosition:  pos,
		AddOffset:       -1,
		Limit:           20,
		Hash:            9,
	}
	raw := encodeAt(t, req, Layer)
	if ctorOf(raw) != crcFeedGetFeed {
		t.Fatalf("ctor 0x%x", ctorOf(raw))
	}
	dec := NewDecodeBuf(raw)
	obj := dec.Object()
	if dec.GetError() != nil {
		t.Fatalf("decode: %v", dec.GetError())
	}
	got, ok := obj.(*TLFeedGetFeed)
	if !ok || !got.GetOffsetToMaxRead() || got.GetFilterId() != 1 || got.GetAddOffset() != -1 || got.GetLimit() != 20 || got.GetHash() != 9 {
		t.Fatalf("getFeed %#v", got)
	}
	if got.GetOffsetPosition().GetId() != 4 || got.GetOffsetPosition().GetPeer().GetUserId() != 7 {
		t.Fatalf("position %#v", got.GetOffsetPosition())
	}

	page := MakeFeedMessages(nil, nil, pos, nil, nil, nil)
	raw = encodeAt(t, page, Layer)
	gotPage, ok := NewDecodeBuf(raw).Object().(*Feed_FeedMessages)
	if !ok || !gotPage.GetHasRead() || gotPage.GetReadMaxPosition().GetDate() != 20 || len(gotPage.GetMessages()) != 0 {
		t.Fatalf("page %#v", gotPage)
	}
	raw = encodeAt(t, MakeFeedMessagesNotModified(), Layer)
	if ctorOf(raw) != crcFeedMessagesNotModified {
		t.Fatalf("not modified 0x%x", ctorOf(raw))
	}

	cats := MakeFeedInterestCategories([]*Feed_InterestCategory{
		MakeFeedInterestCategory(1, "News", "📰"),
	}, []int32{1})
	raw = encodeAt(t, cats, Layer)
	gotCats, ok := NewDecodeBuf(raw).Object().(*Feed_InterestCategories)
	if !ok || len(gotCats.GetCategories()) != 1 || gotCats.Categories[0].GetTitle() != "News" || len(gotCats.GetSelected()) != 1 || gotCats.Selected[0] != 1 {
		t.Fatalf("categories %#v", gotCats)
	}

	you := MakeFeedForYou([]*FeedItem{MakeFeedInterestItem()}, nil, nil)
	raw = encodeAt(t, you, Layer)
	gotYou, ok := NewDecodeBuf(raw).Object().(*Feed_ForYouFeed)
	if !ok || len(gotYou.GetItems()) != 1 || gotYou.Items[0].GetPredicateName() != Predicate_feedItemInterestCategories {
		t.Fatalf("for you %#v", gotYou)
	}

	set := &TLFeedSetInterestCategories{CategoryIds: []int32{2, 3}}
	raw = encodeAt(t, set, Layer)
	gotSet, ok := NewDecodeBuf(raw).Object().(*TLFeedSetInterestCategories)
	if !ok || len(gotSet.GetCategoryIds()) != 2 || gotSet.CategoryIds[1] != 3 {
		t.Fatalf("set %#v", gotSet)
	}
}
