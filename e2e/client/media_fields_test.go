package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestAlbumLimitExternalMediaAndLiveGeo covers ISSUE-S-ALBUM-MAX-001,
// ISSUE-S-MEDIA-EXTERNAL-001, and ISSUE-S-GEO-HEADING-001.
// Phone 9996610475.
func TestAlbumLimitExternalMediaAndLiveGeo(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610475", "GeoAda")
	peer := inputPeerSelf()

	items := make([]*mtproto.InputSingleMedia, 11)
	for i := range items {
		items[i] = mtproto.MakeTLInputSingleMedia(&mtproto.InputSingleMedia{
			Media:    mtproto.MakeTLInputMediaEmpty(nil).To_InputMedia(),
			RandomId: int64(i + 1),
		}).To_InputSingleMedia()
	}
	_, err := c.Invoke(ctx, &mtproto.TLMessagesSendMultiMedia{
		Constructor: mtproto.CRC32_messages_sendMultiMedia_f803138f,
		Peer:        peer,
		MultiMedia:  items,
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "MEDIA_INVALID" {
		t.Fatalf("album 11: %v", err)
	}

	_, err = c.Invoke(ctx, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        peer,
		Media: mtproto.MakeTLInputMediaPhotoExternal(&mtproto.InputMedia{
			Url: "https://example.com/a.jpg",
		}).To_InputMedia(),
		Message:  "",
		RandomId: 21,
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "MEDIA_INVALID" {
		t.Fatalf("photo external: %v", err)
	}

	sent, err := c.Invoke(ctx, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        peer,
		Media: mtproto.MakeTLInputMediaGeoLive(&mtproto.InputMedia{
			GeoPoint: mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{
				Lat:            31.2,
				Long:           121.5,
				AccuracyRadius: wrapperspb.Int32(18),
			}).To_InputGeoPoint(),
			Heading:                     wrapperspb.Int32(90),
			Period:                      wrapperspb.Int32(900),
			ProximityNotificationRadius: wrapperspb.Int32(100),
		}).To_InputMedia(),
		Message:  "",
		RandomId: 22,
	})
	if err != nil {
		t.Fatal(err)
	}
	var media *mtproto.MessageMedia
	for _, msg := range allMessages(sent) {
		if msg.GetMedia() != nil {
			media = msg.GetMedia()
		}
	}
	if media == nil || media.GetPredicateName() != mtproto.Predicate_messageMediaGeoLive {
		t.Fatalf("live media %+v", media)
	}
	if media.GetHeading().GetValue() != 90 || media.GetGeo().GetAccuracyRadius().GetValue() != 18 || media.GetProximityNotificationRadius().GetValue() != 100 {
		t.Fatalf("heading/accuracy %+v geo %+v", media, media.GetGeo())
	}
}
