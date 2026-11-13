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

	joined, err := svc.SearchJoined(ctx, 1, "chn019", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(joined) != 1 || joined[0].GetMessage().GetMessage() != "chn019" || joined[0].PeerId != mineID {
		t.Fatalf("joined chn019 %#v", textsOf(joined))
	}
	hello, err := svc.SearchJoined(ctx, 1, "HELLO", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(hello) != 1 || hello[0].GetMessage().GetMessage() != "Hello there" {
		t.Fatalf("case %#v", textsOf(hello))
	}
	short, err := svc.SearchJoined(ctx, 1, "He", 20)
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

func textsOf(boxes []*mtproto.MessageBox) []string {
	var out []string
	for _, box := range boxes {
		if box != nil && box.Message != nil {
			out = append(out, box.Message.Message)
		}
	}
	return out
}
