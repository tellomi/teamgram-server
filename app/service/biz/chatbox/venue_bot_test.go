package chatbox

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/location"
)

func TestVenueBotInlineRoundTrip(t *testing.T) {
	b := newBook(nil)
	ctx := location.WithClient(context.Background(), location.New(location.Options{}))
	point := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9087, Long: 116.3975}).To_InputGeoPoint()
	got, err := b.InlineResults(ctx, 3, userIn(mtproto.BotVenueId), peerIn(9), "咖啡", "", point)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetGallery() || got.GetUsers()[0].GetId() != mtproto.BotVenueId || !got.GetUsers()[0].GetBotInlineGeo() {
		t.Fatalf("gallery %v user %d", got.GetGallery(), got.GetUsers()[0].GetId())
	}
	if len(got.GetResults()) != 0 {
		t.Fatal("unset key returned rows")
	}

	ctx = location.WithClient(ctx, location.New(location.Options{AMapKey: "k", AMapBase: "http://127.0.0.1:1"}))
	// The client has a key but no server. Search must still return an empty
	// list rather than an RPC error, and the query stays so a later send can
	// look the result up once a row exists.
	got, err = b.InlineResults(ctx, 3, userIn(mtproto.BotVenueId), peerIn(9), "咖啡", "", point)
	if err != nil || len(got.GetResults()) != 0 {
		t.Fatalf("failure %v results %d", err, len(got.GetResults()))
	}
}

func TestTakeInlineVenue(t *testing.T) {
	b := newBook(nil)
	ctx := context.Background()
	raw := mtproto.MakeTLBotInlineResult(&mtproto.BotInlineResult{
		Id:   "B001",
		Type: "venue",
		SendMessage: mtproto.MakeTLBotInlineMessageMediaVenue(&mtproto.BotInlineMessage{
			Geo:       mtproto.MakeTLGeoPoint(&mtproto.GeoPoint{Lat: 39.9, Long: 116.4}).To_GeoPoint(),
			Title:     "星巴克",
			Address:   "北京市东城区建国路",
			Provider:  "amap",
			VenueId:   "B001",
			VenueType: "food/default",
		}).To_BotInlineMessage(),
	}).To_BotInlineResult()
	packed, err := packResults([]*mtproto.BotInlineResult{raw})
	if err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	q := b.storeQueryLocked(ctx, 3, mtproto.BotVenueId, mustPeer(t, 3, peerIn(9)), "咖啡", &cachedAnswer{
		CacheTime: 300,
		Results:   packed,
	})
	b.mu.Unlock()
	req, ready, err := b.TakeInlineResult(ctx, 3, peerIn(9), q.ID, "B001", 11, false, false, false, 0, nil)
	if err != nil || ready != nil {
		t.Fatalf("send %v %v", err, ready)
	}
	media := req.Message.GetMedia()
	if media.GetPredicateName() != mtproto.Predicate_messageMediaVenue {
		t.Fatalf("media %s", media.GetPredicateName())
	}
	if media.GetProvider() != "amap" || media.GetTitle() != "星巴克" || media.GetVenueType() != "food/default" {
		t.Fatalf("venue %+v", media)
	}
	if media.GetGeo().GetAccessHash() == 0 {
		t.Fatal("access_hash")
	}
	if req.Message.GetViaBotId().GetValue() != mtproto.BotVenueId {
		t.Fatalf("via %d", req.Message.GetViaBotId().GetValue())
	}
}

func mustPeer(t *testing.T, self int64, in *mtproto.InputPeer) *mtproto.PeerUtil {
	t.Helper()
	p, err := resolvePeer(self, in)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
