package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/contactphoto"
)

// PhotosUploadContactProfilePhoto
// photos.uploadContactProfilePhoto#e14c4a71 flags:# suggest:flags.3?true save:flags.4?true user_id:InputUser file:flags.0?InputFile video:flags.1?InputFile video_start_ts:flags.2?double video_emoji_markup:flags.5?VideoSize = photos.Photo;
// https://core.telegram.org/method/photos.uploadContactProfilePhoto
func (c *UserChannelProfilesCore) PhotosUploadContactProfilePhoto(in *mtproto.TLPhotosUploadContactProfilePhoto) (*mtproto.Photos_Photo, error) {
	var self int64
	if c.MD != nil {
		self = c.MD.UserId
	}
	return contactphoto.Upload(self, in)
}
