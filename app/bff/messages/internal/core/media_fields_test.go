package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestLiveGeoKeepsHeadingAndAccuracy(t *testing.T) {
	media := mtproto.MakeTLInputMediaGeoLive(&mtproto.InputMedia{
		GeoPoint: mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{
			Lat:            31.2,
			Long:           121.5,
			AccuracyRadius: wrapperspb.Int32(18),
		}).To_InputGeoPoint(),
		Heading:                     wrapperspb.Int32(90),
		Period:                      wrapperspb.Int32(900),
		ProximityNotificationRadius: wrapperspb.Int32(100),
	}).To_InputMedia()
	got, err := (&MessagesCore{}).makeMediaByInputMedia(media)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_messageMediaGeoLive {
		t.Fatalf("media %s", got.GetPredicateName())
	}
	if got.GetHeading().GetValue() != 90 || got.GetPeriod() != 900 || got.GetProximityNotificationRadius().GetValue() != 100 {
		t.Fatalf("live %+v", got)
	}
	if got.GetGeo().GetAccuracyRadius().GetValue() != 18 || got.GetGeo().GetLat() != 31.2 {
		t.Fatalf("geo %+v", got.GetGeo())
	}
}

func TestExternalMediaIsRejected(t *testing.T) {
	for _, pred := range []string{mtproto.Predicate_inputMediaPhotoExternal, mtproto.Predicate_inputMediaDocumentExternal} {
		media := &mtproto.InputMedia{PredicateName: pred, Url: "https://example.com/a"}
		_, err := (&MessagesCore{}).makeMediaByInputMedia(media)
		if status.Convert(err).Message() != "MEDIA_INVALID" {
			t.Fatalf("%s: %v", pred, err)
		}
		_, err = (&dao.Dao{}).GetMessageMedia(context.Background(), 1, 1, media)
		if status.Convert(err).Message() != "MEDIA_INVALID" {
			t.Fatalf("dao %s: %v", pred, err)
		}
	}
}

func TestAlbumRejectsEleventhItem(t *testing.T) {
	items := make([]*mtproto.InputSingleMedia, 11)
	_, err := (&MessagesCore{}).MessagesSendMultiMedia(&mtproto.TLMessagesSendMultiMedia{MultiMedia: items})
	if status.Convert(err).Message() != "MEDIA_INVALID" {
		t.Fatalf("11: %v", err)
	}
}

func TestForwardDisplayNameIsNotUsername(t *testing.T) {
	c := &MessagesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: displayNameUsers{},
		}},
	}
	if got := c.forwardDisplayName(8); got != "Ada Lee" {
		t.Fatalf("named %q", got)
	}
	if got := c.forwardDisplayName(9); got != "Solo" {
		t.Fatalf("no last name %q", got)
	}
}

type displayNameUsers struct {
	userclient.UserClient
}

func (displayNameUsers) UserGetImmutableUser(_ context.Context, in *user.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	switch in.GetId() {
	case 8:
		return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 8, FirstName: "Ada", LastName: "Lee", Username: "ada"}}, nil
	case 9:
		return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 9, FirstName: "Solo", Username: "solo"}}, nil
	default:
		return nil, mtproto.ErrUserIdInvalid
	}
}
