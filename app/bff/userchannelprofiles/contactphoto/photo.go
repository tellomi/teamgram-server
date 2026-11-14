// Package contactphoto serves photos.uploadContactProfilePhoto.
//
// Telegram Desktop (PeerPhoto) sets Flag::f_save together with a file to
// store a picture only the caller sees on that contact, and calls the same
// method with Flag::f_save and no file to clear it. Flag::f_suggest sends
// messageActionSuggestProfilePhoto into the private chat. Neither flag on
// an older client is treated as save.
//
// https://core.telegram.org/method/photos.uploadContactProfilePhoto
// https://core.telegram.org/constructor/messageActionSuggestProfilePhoto
package contactphoto

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
)

type key struct {
	owner   int64
	contact int64
	suggest bool
}

// Clients are the services the RPC handler already has. Nil fields keep the
// in-memory path used by package tests.
type Clients struct {
	Media interface {
		MediaUploadProfilePhotoFile(ctx context.Context, in *mediapb.TLMediaUploadProfilePhotoFile) (*mtproto.Photo, error)
	}
	Users interface {
		UserCheckContact(ctx context.Context, in *userpb.TLUserCheckContact) (*mtproto.Bool, error)
		UserGetImmutableUser(ctx context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error)
	}
	Msg interface {
		MsgSendMessageV2(ctx context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error)
	}
	Sync interface {
		SyncPushUpdates(ctx context.Context, in *syncpb.TLSyncPushUpdates) (*mtproto.Void, error)
	}
}

// Upload is the in-memory entry the package tests call.
func Upload(self int64, in *mtproto.TLPhotosUploadContactProfilePhoto) (*mtproto.Photos_Photo, error) {
	return UploadWith(context.Background(), &metadata.RpcMetadata{UserId: self}, Clients{}, in)
}

// UploadWith stores a personal photo, clears one, or suggests one.
func UploadWith(ctx context.Context, md *metadata.RpcMetadata, clients Clients, in *mtproto.TLPhotosUploadContactProfilePhoto) (*mtproto.Photos_Photo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var self int64
	if md != nil {
		self = md.UserId
	}
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if md != nil && md.IsBot {
		return nil, mtproto.ErrBotMethodInvalid
	}
	if in == nil || in.GetUserId() == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	user := mtproto.FromInputUser(self, in.GetUserId())
	if user.PeerType != mtproto.PEER_USER || user.PeerId == 0 || user.PeerId == self {
		return nil, mtproto.ErrUserIdInvalid
	}
	if err := mustContact(ctx, clients, self, user.PeerId); err != nil {
		return nil, err
	}

	suggest := in.GetSuggest()
	save := in.GetSave() || !suggest
	if !hasMedia(in) {
		if suggest || !save {
			return nil, mtproto.ErrPhotoFileMissing
		}
		// Desktop clearPersonal: save set, no file, video, or emoji markup.
		if err := active().del(ctx, self, user.PeerId); err != nil {
			return nil, err
		}
		photo := clearedPhoto(ctx, clients, user.PeerId)
		return photosPhoto(ctx, clients, self, user.PeerId, photo), nil
	}
	if err := exclusiveMedia(in); err != nil {
		return nil, err
	}
	photo, err := makePhoto(ctx, md, clients, in)
	if err != nil {
		return nil, err
	}
	if save {
		if err = active().put(ctx, self, user.PeerId, photo); err != nil {
			return nil, err
		}
		pushUser(ctx, clients, self)
	}
	if suggest {
		rememberSuggested(self, user.PeerId, photo)
		if err = sendSuggest(ctx, md, clients, user.PeerId, photo); err != nil {
			return nil, err
		}
	}
	return photosPhoto(ctx, clients, self, user.PeerId, photo), nil
}

// ForViewer is the personal photo viewer set for contact. The contact does
// not see it on their own profile. users.getFullUser copies it onto
// userFull.personal_photo.
func ForViewer(ctx context.Context, viewer, contact int64) *mtproto.Photo {
	if viewer == 0 || contact == 0 || viewer == contact {
		return nil
	}
	photo, ok, err := active().get(ctx, viewer, contact)
	if err != nil || !ok || photo.GetId() == 0 {
		return nil
	}
	return photo
}

// Saved returns the photo the caller saved for the contact.
func Saved(self, contact int64) *mtproto.Photo {
	return ForViewer(context.Background(), self, contact)
}

// Suggested returns the photo the caller last suggested to the contact.
func Suggested(self, contact int64) *mtproto.Photo {
	suggestMu.Lock()
	defer suggestMu.Unlock()
	return suggested[key{self, contact, true}]
}

func rememberSuggested(self, contact int64, photo *mtproto.Photo) {
	suggestMu.Lock()
	suggested[key{self, contact, true}] = photo
	suggestMu.Unlock()
}

func mustContact(ctx context.Context, clients Clients, self, contact int64) error {
	if clients.Users == nil {
		return nil
	}
	ok, err := clients.Users.UserCheckContact(ctx, &userpb.TLUserCheckContact{UserId: self, Id: contact})
	if err != nil {
		return err
	}
	if !mtproto.FromBool(ok) {
		return mtproto.ErrContactMissing
	}
	return nil
}

func hasMedia(in *mtproto.TLPhotosUploadContactProfilePhoto) bool {
	return in.GetFile() != nil || in.GetVideo() != nil || in.GetVideoEmojiMarkup() != nil
}

func exclusiveMedia(in *mtproto.TLPhotosUploadContactProfilePhoto) error {
	n := 0
	if in.GetFile() != nil {
		n++
		if in.GetFile().GetParts() < 1 {
			return mtproto.ErrFilePartsInvalid
		}
	}
	if in.GetVideo() != nil {
		n++
		if in.GetVideo().GetParts() < 1 {
			return mtproto.ErrFilePartsInvalid
		}
	}
	if in.GetVideoEmojiMarkup() != nil {
		n++
	}
	if n != 1 {
		return mtproto.ErrMediaInvalid
	}
	if in.GetVideoStartTs() != nil && in.GetVideo() == nil && in.GetVideoEmojiMarkup() == nil {
		return mtproto.ErrMediaInvalid
	}
	return nil
}

func makePhoto(ctx context.Context, md *metadata.RpcMetadata, clients Clients, in *mtproto.TLPhotosUploadContactProfilePhoto) (*mtproto.Photo, error) {
	if clients.Media != nil {
		owner := md.GetUserId()
		if md.GetPermAuthKeyId() != 0 {
			owner = md.GetPermAuthKeyId()
		}
		return clients.Media.MediaUploadProfilePhotoFile(ctx, &mediapb.TLMediaUploadProfilePhotoFile{
			OwnerId:          owner,
			File:             in.GetFile(),
			Video:            in.GetVideo(),
			VideoStartTs:     in.GetVideoStartTs(),
			VideoEmojiMarkup: in.GetVideoEmojiMarkup(),
		})
	}
	return photoFromFile(in), nil
}

func photoFromFile(in *mtproto.TLPhotosUploadContactProfilePhoto) *mtproto.Photo {
	var id int64
	if in.GetFile() != nil {
		id = in.GetFile().GetId_INT64()
	}
	if id == 0 && in.GetVideo() != nil {
		id = in.GetVideo().GetId_INT64()
	}
	if id == 0 {
		id = 1
	}
	return mtproto.MakeTLPhoto(&mtproto.Photo{
		Id:         id,
		AccessHash: id ^ 0x5f3759df,
		Date:       int32(time.Now().Unix()),
		DcId:       1,
	}).To_Photo()
}

func clearedPhoto(ctx context.Context, clients Clients, contact int64) *mtproto.Photo {
	if clients.Users != nil {
		if user, err := clients.Users.UserGetImmutableUser(ctx, &userpb.TLUserGetImmutableUser{Id: contact}); err == nil && user != nil && user.User.GetProfilePhoto().GetId() != 0 {
			return user.User.GetProfilePhoto()
		}
	}
	return mtproto.MakeTLPhotoEmpty(&mtproto.Photo{}).To_Photo()
}

func photosPhoto(ctx context.Context, clients Clients, self, contact int64, photo *mtproto.Photo) *mtproto.Photos_Photo {
	return mtproto.MakeTLPhotosPhoto(&mtproto.Photos_Photo{
		Photo: photo,
		Users: responseUsers(ctx, clients, self, contact),
	}).To_Photos_Photo()
}

func responseUsers(ctx context.Context, clients Clients, self, contact int64) []*mtproto.User {
	if clients.Users == nil {
		return []*mtproto.User{
			mtproto.MakeTLUser(&mtproto.User{Id: self, Self: true}).To_User(),
			mtproto.MakeTLUser(&mtproto.User{Id: contact}).To_User(),
		}
	}
	var out []*mtproto.User
	if me, err := clients.Users.UserGetImmutableUser(ctx, &userpb.TLUserGetImmutableUser{Id: self}); err == nil && me != nil {
		out = append(out, me.ToSelfUser())
	}
	if peer, err := clients.Users.UserGetImmutableUser(ctx, &userpb.TLUserGetImmutableUser{Id: contact}); err == nil && peer != nil {
		out = append(out, peer.ToUser(self))
	}
	if len(out) == 0 {
		return []*mtproto.User{
			mtproto.MakeTLUser(&mtproto.User{Id: self, Self: true}).To_User(),
			mtproto.MakeTLUser(&mtproto.User{Id: contact}).To_User(),
		}
	}
	return out
}

func sendSuggest(ctx context.Context, md *metadata.RpcMetadata, clients Clients, contact int64, photo *mtproto.Photo) error {
	if clients.Msg == nil {
		return nil
	}
	message := mtproto.MakeTLMessageService(&mtproto.Message{
		Out:    true,
		FromId: mtproto.MakePeerUser(md.UserId),
		PeerId: mtproto.MakePeerUser(contact),
		Date:   int32(time.Now().Unix()),
		Action: mtproto.MakeTLMessageActionSuggestProfilePhoto(&mtproto.MessageAction{
			Photo: photo,
		}).To_MessageAction(),
	}).To_Message()
	_, err := clients.Msg.MsgSendMessageV2(ctx, &msgpb.TLMsgSendMessageV2{
		UserId:    md.UserId,
		AuthKeyId: md.PermAuthKeyId,
		PeerType:  mtproto.PEER_USER,
		PeerId:    contact,
		Message: []*msgpb.OutboxMessage{
			msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
				NoWebpage: true,
				RandomId:  time.Now().UnixNano(),
				Message:   message,
			}).To_OutboxMessage(),
		},
	})
	return err
}

func pushUser(ctx context.Context, clients Clients, self int64) {
	if clients.Sync == nil || clients.Users == nil {
		return
	}
	me, err := clients.Users.UserGetImmutableUser(ctx, &userpb.TLUserGetImmutableUser{Id: self})
	if err != nil || me == nil {
		return
	}
	_, _ = clients.Sync.SyncPushUpdates(ctx, &syncpb.TLSyncPushUpdates{
		UserId: self,
		Updates: mtproto.MakeUpdatesByUpdatesUsers(
			[]*mtproto.User{me.ToSelfUser()},
			mtproto.MakeTLUpdateUser(&mtproto.Update{UserId: self}).To_Update()),
	})
}
