package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	mediaclient "github.com/teamgram/teamgram-server/app/service/media/client"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestUpdateProfilePhotoRejectsMissingForeignAndBadHash(t *testing.T) {
	users := &ownerUser{album: []int64{11, 12}, fallback: 77}
	media := &ownerMedia{photos: map[int64]*mtproto.Photo{
		11: ownedPhoto(11, 1100),
		12: ownedPhoto(12, 1200),
		77: ownedPhoto(77, 7700),
		99: ownedPhoto(99, 9900),
	}}
	c := ownerCore(users, media)

	for _, in := range []*mtproto.TLPhotosUpdateProfilePhoto{
		updatePhoto(12345, 1, false),
		updatePhoto(99, 9900, false),
		updatePhoto(11, 1101, false),
		updatePhoto(-4, 1, false),
	} {
		_, err := c.PhotosUpdateProfilePhoto(in)
		if status.Convert(err).Message() != "PHOTO_ID_INVALID" {
			t.Fatalf("id %d hash %d: %v", in.GetId().GetId(), in.GetId().GetAccessHash(), err)
		}
	}
	if len(users.updates) != 0 {
		t.Fatalf("rejected ids wrote the album: %v", users.updates)
	}

	switched, err := c.PhotosUpdateProfilePhoto(updatePhoto(12, 1200, false))
	if err != nil {
		t.Fatal(err)
	}
	if switched.GetPhoto().GetId() != 12 || len(users.updates) != 1 || users.updates[0] != 12 {
		t.Fatalf("switch photo %d updates %v", switched.GetPhoto().GetId(), users.updates)
	}

	fallback, err := c.PhotosUpdateProfilePhoto(updatePhoto(77, 7700, true))
	if err != nil {
		t.Fatal(err)
	}
	if fallback.GetPhoto().GetId() != 77 || users.updates[len(users.updates)-1] != -77 {
		t.Fatalf("fallback photo %d updates %v", fallback.GetPhoto().GetId(), users.updates)
	}
}

func TestUpdateProfilePhotoEmptyStillClears(t *testing.T) {
	users := &ownerUser{album: []int64{11}}
	media := &ownerMedia{photos: map[int64]*mtproto.Photo{11: ownedPhoto(11, 1100)}}
	c := ownerCore(users, media)

	cleared, err := c.PhotosUpdateProfilePhoto(&mtproto.TLPhotosUpdateProfilePhoto{
		Id: mtproto.MakeTLInputPhotoEmpty(&mtproto.InputPhoto{Id: 11, AccessHash: 1}).To_InputPhoto(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.GetPhoto().GetPredicateName() != mtproto.Predicate_photoEmpty {
		t.Fatalf("empty returned %s", cleared.GetPhoto().GetPredicateName())
	}
	if len(users.updates) != 1 || users.updates[0] != 0 {
		t.Fatalf("empty wrote %v", users.updates)
	}
	if media.gets != 0 {
		t.Fatalf("empty looked up %d photos", media.gets)
	}
}

func TestUpdateProfilePhotoMediaErrorDoesNotWrite(t *testing.T) {
	users := &ownerUser{album: []int64{11}}
	media := &ownerMedia{err: errors.New("media down")}
	c := ownerCore(users, media)
	_, err := c.PhotosUpdateProfilePhoto(updatePhoto(11, 1100, false))
	if err == nil || status.Convert(err).Message() == "PHOTO_ID_INVALID" {
		t.Fatalf("media failure became %v", err)
	}
	if len(users.updates) != 0 {
		t.Fatalf("media failure wrote %v", users.updates)
	}
}

func updatePhoto(id, hash int64, fallback bool) *mtproto.TLPhotosUpdateProfilePhoto {
	return &mtproto.TLPhotosUpdateProfilePhoto{
		Fallback: fallback,
		Id: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{
			Id:         id,
			AccessHash: hash,
		}).To_InputPhoto(),
	}
}

func ownedPhoto(id, hash int64) *mtproto.Photo {
	return mtproto.MakeTLPhoto(&mtproto.Photo{
		Id:         id,
		AccessHash: hash,
	}).To_Photo()
}

func ownerCore(users *ownerUser, media *ownerMedia) *UserChannelProfilesCore {
	ctx := context.Background()
	return &UserChannelProfilesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient:  users,
			MediaClient: media,
			SyncClient:  ownerSync{},
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
}

type ownerUser struct {
	userclient.UserClient
	album    []int64
	fallback int64
	updates  []int64
}

func (u *ownerUser) UserGetProfilePhotos(context.Context, *user.TLUserGetProfilePhotos) (*user.Vector_Long, error) {
	return &user.Vector_Long{Datas: append([]int64{}, u.album...)}, nil
}

func (u *ownerUser) UserGetFallbackPhoto(context.Context, *user.TLUserGetProfilePhotos) (*mtproto.Photo, error) {
	if u.fallback == 0 {
		return mtproto.MakeTLPhotoEmpty(&mtproto.Photo{}).To_Photo(), nil
	}
	return mtproto.MakeTLPhoto(&mtproto.Photo{Id: u.fallback}).To_Photo(), nil
}

func (u *ownerUser) UserUpdateProfilePhoto(_ context.Context, in *user.TLUserUpdateProfilePhoto) (*mtproto.Int64, error) {
	u.updates = append(u.updates, in.GetId())
	id := in.GetId()
	if id < 0 {
		id = -id
	}
	return &mtproto.Int64{V: id}, nil
}

func (u *ownerUser) UserGetImmutableUser(_ context.Context, in *user.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: in.GetId()}}, nil
}

type ownerMedia struct {
	mediaclient.MediaClient
	photos map[int64]*mtproto.Photo
	gets   int
	err    error
}

func (m *ownerMedia) MediaGetPhoto(_ context.Context, in *mediapb.TLMediaGetPhoto) (*mtproto.Photo, error) {
	m.gets++
	if m.err != nil {
		return nil, m.err
	}
	if photo := m.photos[in.GetPhotoId()]; photo != nil {
		return photo, nil
	}
	return mtproto.MakeTLPhotoEmpty(&mtproto.Photo{Id: in.GetPhotoId()}).To_Photo(), nil
}

type ownerSync struct {
	syncclient.SyncClient
}

func (ownerSync) SyncPushUpdates(context.Context, *syncpb.TLSyncPushUpdates) (*mtproto.Void, error) {
	return mtproto.EmptyVoid, nil
}
