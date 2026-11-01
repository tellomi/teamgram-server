// Package contactphoto serves photos.uploadContactProfilePhoto.
//
// save stores a photo only the caller sees for that contact. suggest stores
// a photo offered to the contact. At least one of the two flags is required
// by current clients; when neither is set the photo is saved.
//
// https://core.telegram.org/method/photos.uploadContactProfilePhoto
package contactphoto

import (
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
)

type key struct {
	owner   int64
	contact int64
	suggest bool
}

type book struct {
	mu    sync.Mutex
	items map[key]*mtproto.Photo
}

var saved = &book{items: map[key]*mtproto.Photo{}}

// Reset drops stored contact photos. Tests call it.
func Reset() {
	saved.mu.Lock()
	saved.items = map[key]*mtproto.Photo{}
	saved.mu.Unlock()
}

// Upload stores the contact photo and returns photos.photo.
func Upload(self int64, in *mtproto.TLPhotosUploadContactProfilePhoto) (*mtproto.Photos_Photo, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	if in.GetUserId() == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	user := mtproto.FromInputUser(self, in.GetUserId())
	if user.PeerType != mtproto.PEER_USER || user.PeerId == 0 || user.PeerId == self {
		return nil, mtproto.ErrUserIdInvalid
	}
	photo, err := photoFrom(in)
	if err != nil {
		return nil, err
	}
	suggest := in.GetSuggest()
	save := in.GetSave() || !suggest
	saved.mu.Lock()
	if save {
		saved.items[key{self, user.PeerId, false}] = photo
	}
	if suggest {
		saved.items[key{self, user.PeerId, true}] = photo
	}
	saved.mu.Unlock()
	return mtproto.MakeTLPhotosPhoto(&mtproto.Photos_Photo{
		Photo: photo,
		Users: []*mtproto.User{
			mtproto.MakeTLUser(&mtproto.User{Id: self, Self: true}).To_User(),
			mtproto.MakeTLUser(&mtproto.User{Id: user.PeerId}).To_User(),
		},
	}).To_Photos_Photo(), nil
}

// Saved returns the photo the caller saved for the contact.
func Saved(self, contact int64) *mtproto.Photo {
	saved.mu.Lock()
	defer saved.mu.Unlock()
	return saved.items[key{self, contact, false}]
}

// Suggested returns the photo the caller suggested to the contact.
func Suggested(self, contact int64) *mtproto.Photo {
	saved.mu.Lock()
	defer saved.mu.Unlock()
	return saved.items[key{self, contact, true}]
}

func photoFrom(in *mtproto.TLPhotosUploadContactProfilePhoto) (*mtproto.Photo, error) {
	file := in.GetFile()
	video := in.GetVideo()
	emoji := in.GetVideoEmojiMarkup()
	if file == nil && video == nil && emoji == nil {
		return nil, mtproto.ErrPhotoFileMissing
	}
	var id int64
	if file != nil {
		if file.GetParts() < 1 {
			return nil, mtproto.ErrFilePartsInvalid
		}
		id = file.GetId_INT64()
	}
	if video != nil {
		if video.GetParts() < 1 {
			return nil, mtproto.ErrFilePartsInvalid
		}
		if id == 0 {
			id = video.GetId_INT64()
		}
	}
	if id == 0 {
		id = 1
	}
	return mtproto.MakeTLPhoto(&mtproto.Photo{
		Id:         id,
		AccessHash: id ^ 0x5f3759df,
		Date:       int32(time.Now().Unix()),
		DcId:       1,
	}).To_Photo(), nil
}
