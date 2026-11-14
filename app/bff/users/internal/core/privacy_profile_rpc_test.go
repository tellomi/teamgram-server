package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/users/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/users/internal/svc"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestGetFullUserHidesAboutBirthdayAndPhoto(t *testing.T) {
	users := &fullUserAPI{allow: map[int32]bool{
		mtproto.ABOUT:          false,
		mtproto.BIRTHDAY:       false,
		mtproto.PROFILE_PHOTO:  false,
		mtproto.PHONE_CALL:     true,
		mtproto.VOICE_MESSAGES: true,
	}}
	c := fullUserCore(users)

	denied, err := c.UsersGetFullUser(fullUserRequest(2))
	if err != nil {
		t.Fatal(err)
	}
	if denied.GetFullUser().GetAbout().GetValue() != "" || denied.GetFullUser().GetBirthday() != nil {
		t.Fatalf("hidden profile leaked about %q birthday %v", denied.GetFullUser().GetAbout().GetValue(), denied.GetFullUser().GetBirthday())
	}
	if denied.GetFullUser().GetProfilePhoto().GetId() != 0 {
		t.Fatalf("hidden profile photo %d", denied.GetFullUser().GetProfilePhoto().GetId())
	}

	users.allow[mtproto.ABOUT] = true
	users.allow[mtproto.BIRTHDAY] = true
	users.allow[mtproto.PROFILE_PHOTO] = true
	shown, err := c.UsersGetFullUser(fullUserRequest(2))
	if err != nil {
		t.Fatal(err)
	}
	if shown.GetFullUser().GetAbout().GetValue() != "hello" || shown.GetFullUser().GetBirthday().GetDay() != 2 {
		t.Fatalf("visible profile about %q birthday %+v", shown.GetFullUser().GetAbout().GetValue(), shown.GetFullUser().GetBirthday())
	}
	if shown.GetFullUser().GetProfilePhoto().GetId() != 200 {
		t.Fatalf("main photo %d", shown.GetFullUser().GetProfilePhoto().GetId())
	}
}

func TestGetFullUserUsesFallbackPhoto(t *testing.T) {
	users := &fullUserAPI{
		allow:    map[int32]bool{mtproto.PROFILE_PHOTO: false, mtproto.PHONE_CALL: true, mtproto.VOICE_MESSAGES: true},
		fallback: sizedPhoto(77),
	}
	got, err := fullUserCore(users).UsersGetFullUser(fullUserRequest(2))
	if err != nil {
		t.Fatal(err)
	}
	if got.GetFullUser().GetProfilePhoto().GetId() != 77 || got.GetFullUser().GetFallbackPhoto().GetId() != 77 {
		t.Fatalf("profile %d fallback %d", got.GetFullUser().GetProfilePhoto().GetId(), got.GetFullUser().GetFallbackPhoto().GetId())
	}
	if len(got.GetUsers()) != 1 || got.GetUsers()[0].GetPhoto().GetPhotoId() != 77 {
		t.Fatalf("small photo %+v", got.GetUsers()[0].GetPhoto())
	}
}

func TestGetFullUserBlockedOwnerHidesStatusAndPhoto(t *testing.T) {
	users := &fullUserAPI{
		allow:        map[int32]bool{mtproto.PROFILE_PHOTO: true, mtproto.ABOUT: true, mtproto.PHONE_CALL: true, mtproto.VOICE_MESSAGES: true},
		ownerBlocked: true,
		fallback:     &mtproto.Photo{Id: 77, PredicateName: mtproto.Predicate_photo},
	}
	got, err := fullUserCore(users).UsersGetFullUser(fullUserRequest(2))
	if err != nil {
		t.Fatal(err)
	}
	user := got.GetUsers()[0]
	if user.GetStatus().GetPredicateName() != mtproto.Predicate_userStatusEmpty || user.GetPhoto() != nil {
		t.Fatalf("status %s photo %+v", user.GetStatus().GetPredicateName(), user.GetPhoto())
	}
	if got.GetFullUser().GetProfilePhoto().GetId() != 0 || got.GetFullUser().GetFallbackPhoto().GetId() != 0 {
		t.Fatal("blocked owner still exposed a profile photo")
	}
	if got.GetFullUser().GetAbout().GetValue() != "hello" {
		t.Fatalf("about %q", got.GetFullUser().GetAbout().GetValue())
	}
}

func fullUserCore(users *fullUserAPI) *UsersCore {
	ctx := context.Background()
	return &UsersCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient:   users,
			ChatClient:   stubChat{},
			DialogClient: stubDialog{},
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
}

func fullUserRequest(id int64) *mtproto.TLUsersGetFullUser {
	return &mtproto.TLUsersGetFullUser{
		Id: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: id, AccessHash: 1}).To_InputUser(),
	}
}

type fullUserAPI struct {
	userclient.UserClient
	allow        map[int32]bool
	ownerBlocked bool
	fallback     *mtproto.Photo
}

func (u *fullUserAPI) UserGetMutableUsersV2(context.Context, *user.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	return &mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{
		{User: &mtproto.UserData{Id: 1}, LastSeenAt: time.Now().Unix()},
		profilePeer(),
	}}, nil
}

func sizedPhoto(id int64) *mtproto.Photo {
	return &mtproto.Photo{
		Id:            id,
		PredicateName: mtproto.Predicate_photo,
		Sizes:         []*mtproto.PhotoSize{{PredicateName: mtproto.Predicate_photoSize, Type: "a"}},
	}
}

func profilePeer() *mtproto.ImmutableUser {
	return &mtproto.ImmutableUser{
		User: &mtproto.UserData{
			Id:       2,
			About:    mtproto.MakeFlagsString("hello"),
			Birthday: "1991-03-02",
			ProfilePhoto: &mtproto.Photo{
				Id:            200,
				PredicateName: mtproto.Predicate_photo,
				Sizes:         []*mtproto.PhotoSize{{PredicateName: mtproto.Predicate_photoSize, Type: "a"}},
			},
		},
		LastSeenAt: time.Now().Unix(),
	}
}

func (u *fullUserAPI) UserBlockedByUser(_ context.Context, in *user.TLUserBlockedByUser) (*mtproto.Bool, error) {
	if u.ownerBlocked && in.GetUserId() == 2 && in.GetPeerUserId() == 1 {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

func (u *fullUserAPI) UserGetPeerSettings(context.Context, *user.TLUserGetPeerSettings) (*mtproto.PeerSettings, error) {
	return mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{}).To_PeerSettings(), nil
}

func (u *fullUserAPI) UserGetNotifySettings(context.Context, *user.TLUserGetNotifySettings) (*mtproto.PeerNotifySettings, error) {
	return mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{}).To_PeerNotifySettings(), nil
}

func (u *fullUserAPI) UserCheckPrivacy(_ context.Context, in *user.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	if u.allow[in.GetKeyType()] {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

func (u *fullUserAPI) UserGetFallbackPhoto(context.Context, *user.TLUserGetProfilePhotos) (*mtproto.Photo, error) {
	if u.fallback == nil {
		return mtproto.MakeTLPhotoEmpty(&mtproto.Photo{}).To_Photo(), nil
	}
	return u.fallback, nil
}

type stubChat struct{ chatclient.ChatClient }

func (stubChat) ChatGetUsersChatIdList(context.Context, *chatpb.TLChatGetUsersChatIdList) (*chatpb.Vector_UserChatIdList, error) {
	return nil, nil
}

type stubDialog struct{ dialogclient.DialogClient }

func (stubDialog) DialogGetDialogById(context.Context, *dialog.TLDialogGetDialogById) (*dialog.DialogExt, error) {
	return nil, nil
}
