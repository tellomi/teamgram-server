package channel

import "testing"

func TestRestrictSponsoredMessagesShowsOnFullChat(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.SetSponsoredRestricted(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil || !full.FullChat.GetRestrictedSponsored() {
		t.Fatalf("flag %#v %v", full.GetFullChat().GetRestrictedSponsored(), err)
	}
}
