package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	mediaclient "github.com/teamgram/teamgram-server/app/service/media/client"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestGetUserPhotosPrivacyAllowDenyAndFallback(t *testing.T) {
	users := &photoUser{allow: true, album: []int64{11, 12}, fallback: 77}
	media := &photoMedia{}
	c := photoCore(users, media)
	in := &mtproto.TLPhotosGetUserPhotos{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2, AccessHash: 1}).To_InputUser(),
	}

	allowed, err := c.PhotosGetUserPhotos(in)
	if err != nil {
		t.Fatal(err)
	}
	if ids := photoIDs(allowed.GetPhotos()); len(ids) != 2 || ids[0] != 11 || ids[1] != 12 {
		t.Fatalf("allowed album %v", ids)
	}

	users.allow = false
	hidden, err := c.PhotosGetUserPhotos(in)
	if err != nil {
		t.Fatal(err)
	}
	if ids := photoIDs(hidden.GetPhotos()); len(ids) != 1 || ids[0] != 77 {
		t.Fatalf("fallback album %v", ids)
	}

	users.fallback = 0
	empty, err := c.PhotosGetUserPhotos(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.GetPhotos()) != 0 {
		t.Fatalf("empty album %v", photoIDs(empty.GetPhotos()))
	}
	if media.gets != 2 {
		t.Fatalf("media loads %d, hidden pages must not fetch the private album", media.gets)
	}
}

func photoCore(users *photoUser, media *photoMedia) *UserChannelProfilesCore {
	ctx := context.Background()
	return &UserChannelProfilesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: users, MediaClient: media}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
}

func photoIDs(photos []*mtproto.Photo) []int64 {
	out := make([]int64, 0, len(photos))
	for _, p := range photos {
		out = append(out, p.GetId())
	}
	return out
}

type photoUser struct {
	userclient.UserClient
	allow    bool
	album    []int64
	fallback int64
}

func (u *photoUser) UserGetProfilePhotos(context.Context, *user.TLUserGetProfilePhotos) (*user.Vector_Long, error) {
	return &user.Vector_Long{Datas: append([]int64{}, u.album...)}, nil
}

func (u *photoUser) UserGetFallbackPhoto(context.Context, *user.TLUserGetProfilePhotos) (*mtproto.Photo, error) {
	if u.fallback == 0 {
		return mtproto.MakeTLPhotoEmpty(&mtproto.Photo{}).To_Photo(), nil
	}
	return &mtproto.Photo{Id: u.fallback, PredicateName: mtproto.Predicate_photo}, nil
}

func (u *photoUser) UserCheckPrivacy(context.Context, *user.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	if u.allow {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

type photoMedia struct {
	mediaclient.MediaClient
	gets int
}

func (m *photoMedia) MediaGetPhoto(_ context.Context, in *mediapb.TLMediaGetPhoto) (*mtproto.Photo, error) {
	m.gets++
	return &mtproto.Photo{Id: in.GetPhotoId(), PredicateName: mtproto.Predicate_photo}, nil
}
