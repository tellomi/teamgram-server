package channel

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/searchorder"
)

func TestSearchJoinedCaseChannelAndNotPublic(t *testing.T) {
	svc, ctx := newTest(t)
	mine, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "mine", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	mineID := mine.Chats[0].Id
	other, err := svc.Create(ctx, CreateRequest{CreatorID: 2, Title: "public news", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	otherID := other.Chats[0].Id
	otherIn := input(otherID, other.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.UpdateUsername(ctx, 2, 0, otherIn, "publicnews"); err != nil {
		t.Fatal(err)
	}
	send := func(who, channelID int64, text string, random int64) {
		t.Helper()
		if _, err := svc.Send(ctx, who, channelID, 0, []Outgoing{{
			RandomID: random,
			Message:  &mtproto.Message{Message: text, Date: int32(100 + random)},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	send(1, mineID, "chn019", 1)
	send(1, mineID, "Hello there", 2)
	send(2, otherID, "chn019", 3)

	joined, err := svc.SearchJoined(ctx, 1, "chn019", 20, searchorder.Key{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(joined) != 1 || joined[0].GetMessage().GetMessage() != "chn019" || joined[0].PeerId != mineID {
		t.Fatalf("joined chn019 %#v", textsOf(joined))
	}
	hello, err := svc.SearchJoined(ctx, 1, "HELLO", 20, searchorder.Key{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(hello) != 1 || hello[0].GetMessage().GetMessage() != "Hello there" {
		t.Fatalf("case %#v", textsOf(hello))
	}
	short, err := svc.SearchJoined(ctx, 1, "He", 20, searchorder.Key{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(short) != 1 || short[0].GetMessage().GetMessage() != "Hello there" {
		t.Fatalf("short %#v", textsOf(short))
	}
	all, err := svc.SearchPosts(ctx, 1, PostQuery{Needle: "chn019", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Messages) < 2 {
		t.Fatalf("public post search should still see both, got %d", len(all.Messages))
	}
}

func TestSearchJoinedReachesPastTheFirstWindow(t *testing.T) {
	svc, ctx := newTest(t)
	mine, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "deep", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	id := mine.Chats[0].Id
	const n = 201
	for i := 0; i < n; i++ {
		if _, err := svc.Send(ctx, 1, id, 0, []Outgoing{{
			RandomID: int64(i + 1),
			Message:  &mtproto.Message{Message: "hit", Date: int32(1000 - i)},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := svc.SearchJoined(ctx, 1, "hit", 200, searchorder.Key{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 200 {
		t.Fatalf("first window %d", len(first))
	}
	for _, box := range first {
		if box.GetMessage().GetDate() == 1000-(n-1) {
			t.Fatal("the 201st post was inside the first 200")
		}
	}
	// The 200th newest has date 1000-199. The next page must still see 1000-200.
	edge := first[len(first)-1]
	next, err := svc.SearchJoined(ctx, 1, "hit", 5, searchorder.Key{
		Date:     edge.GetMessage().GetDate(),
		PeerType: mtproto.PEER_CHANNEL,
		PeerID:   edge.GetPeerId(),
		MsgID:    edge.GetMessageId(),
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, box := range next {
		if box.GetMessage().GetDate() == 1000-(n-1) {
			saw = true
		}
		if box.GetMessage().GetDate() > 1000-199 {
			t.Fatalf("newer than the cursor: %d", box.GetMessage().GetDate())
		}
	}
	if !saw {
		t.Fatalf("201st post missing from %#v", textsOf(next))
	}
}

func TestSearchJoinedEqualDatePagesDoNotSkip(t *testing.T) {
	svc, ctx := newTest(t)
	a, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "a", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "b", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ids := []int64{a.Chats[0].Id, b.Chats[0].Id}
	const per = 40
	for n := 0; n < per; n++ {
		for _, id := range ids {
			if _, err := svc.Send(ctx, 1, id, 0, []Outgoing{{
				RandomID: int64(id)*1000 + int64(n) + 1,
				Message:  &mtproto.Message{Message: "same", Date: 50},
			}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	var got []searchorder.Key
	var cur searchorder.Key
	on := false
	for page := 0; page < 30; page++ {
		boxes, err := svc.SearchJoined(ctx, 1, "same", 7, cur, on)
		if err != nil {
			t.Fatal(err)
		}
		if len(boxes) == 0 {
			break
		}
		for _, box := range boxes {
			key := searchorder.Key{Date: box.GetMessage().GetDate(), PeerType: mtproto.PEER_CHANNEL, PeerID: box.GetPeerId(), MsgID: box.GetMessageId()}
			if len(got) > 0 && !searchorder.Newer(got[len(got)-1], key) && got[len(got)-1] != key {
				t.Fatalf("out of order %#v then %#v", got[len(got)-1], key)
			}
			for _, prev := range got {
				if prev == key {
					t.Fatalf("duplicate %#v", key)
				}
			}
			got = append(got, key)
		}
		last := boxes[len(boxes)-1]
		cur = searchorder.Key{Date: last.GetMessage().GetDate(), PeerType: mtproto.PEER_CHANNEL, PeerID: last.GetPeerId(), MsgID: last.GetMessageId()}
		on = true
		if len(boxes) < 7 {
			break
		}
	}
	if len(got) != per*2 {
		t.Fatalf("pages returned %d of %d", len(got), per*2)
	}
}

func TestGlobalChannelSQLOrdersLikeTheMerge(t *testing.T) {
	if !strings.Contains(globalChannelOrderSQL, "date2 DESC, m.channel_id ASC, m.message_id DESC") {
		t.Fatal(globalChannelOrderSQL)
	}
}

func textsOf(boxes []*mtproto.MessageBox) []string {
	var out []string
	for _, box := range boxes {
		if box != nil && box.Message != nil {
			out = append(out, box.Message.Message)
		}
	}
	return out
}
