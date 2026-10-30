package botadmin

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func selfPeer() *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
}

func TestStarRefConnect(t *testing.T) {
	Reset()
	ctx := context.Background()
	bot, err := CreateBot(ctx, 7, true, "News", "newsbot", manager())
	if err != nil {
		t.Fatal(err)
	}
	in := asUser(bot.GetId(), bot.GetAccessHash().GetValue())
	peer := selfPeer()
	if _, err := ConnectStarRefBot(8, peer, in); codeOf(err) != codeStarRefMissing {
		t.Fatal(codeOf(err))
	}
	if _, err := UpdateStarRefProgram(ctx, 7, in, 100, wrapperspb.Int32(3)); err != nil {
		t.Fatal(err)
	}
	got, err := ConnectStarRefBot(8, peer, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetCount() != 1 || got.GetConnectedBots()[0].GetCommissionPermille() != 100 || got.GetConnectedBots()[0].GetRevoked() {
		t.Fatalf("%+v", got)
	}
	if len(got.GetUsers()) != 1 || got.GetUsers()[0].GetId() != bot.GetId() {
		t.Fatal(got.GetUsers())
	}
	one, err := ConnectedStarRefBot(8, peer, in)
	if err != nil || one.GetCount() != 1 {
		t.Fatal(err, one)
	}
	link := got.GetConnectedBots()[0].GetUrl()
	revoked, err := EditConnectedStarRefBot(8, peer, link, true)
	if err != nil || !revoked.GetConnectedBots()[0].GetRevoked() {
		t.Fatal(err, revoked)
	}
	if _, err := EditConnectedStarRefBot(8, peer, "https://t.me/missing", true); codeOf(err) != codeStarRefLink {
		t.Fatal(codeOf(err))
	}
	suggested, err := SuggestedStarRefBots(8, &mtproto.TLPaymentsGetSuggestedStarRefBots{Peer: peer, Limit: 10})
	if err != nil || suggested.GetCount() != 1 || suggested.GetNextOffset() != nil {
		t.Fatal(err, suggested)
	}
	page, err := ConnectedStarRefBots(8, &mtproto.TLPaymentsGetConnectedStarRefBots{
		Peer: peer, Limit: 1, OffsetDate: wrapperspb.Int32(1), OffsetLink: wrapperspb.String("nope"),
	})
	if err == nil || codeOf(err) != codeOffsetInvalid {
		t.Fatal(err, page)
	}
	empty, err := ConnectedStarRefBot(8, peer, asUser(99, 1))
	if codeOf(err) != "BOT_INVALID" {
		t.Fatal(err, empty)
	}
}
