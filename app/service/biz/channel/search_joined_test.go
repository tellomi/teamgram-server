package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
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

	joined, err := svc.SearchJoined(ctx, 1, "chn019", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(joined) != 1 || joined[0].GetMessage().GetMessage() != "chn019" || joined[0].PeerId != mineID {
		t.Fatalf("joined chn019 %#v", textsOf(joined))
	}
	hello, err := svc.SearchJoined(ctx, 1, "HELLO", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hello) != 1 || hello[0].GetMessage().GetMessage() != "Hello there" {
		t.Fatalf("case %#v", textsOf(hello))
	}
	short, err := svc.SearchJoined(ctx, 1, "He", 20, 0)
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
	first, err := svc.SearchJoined(ctx, 1, "hit", 200, 0)
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
	next, err := svc.SearchJoined(ctx, 1, "hit", 5, 1000-199)
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

func textsOf(boxes []*mtproto.MessageBox) []string {
	var out []string
	for _, box := range boxes {
		if box != nil && box.Message != nil {
			out = append(out, box.Message.Message)
		}
	}
	return out
}
