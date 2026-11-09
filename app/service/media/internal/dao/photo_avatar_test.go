package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/media/internal/dal/dataobject"
)

func TestToPhotoKeepsVideoOnlyAvatar(t *testing.T) {
	c := &CachePhotoData{
		Id: 8,
		Photo: &dataobject.PhotosDO{
			PhotoId:    8,
			AccessHash: 3,
			HasVideo:   true,
			DcId:       1,
		},
		VideoSizeList: []*dataobject.VideoSizesDO{
			{VideoSizeId: 8, SizeType: "v", Width: 32, Height: 32, FileSize: 10, VideoStartTs: 0},
		},
	}
	photo := c.ToPhoto()
	if photo.GetPredicateName() != mtproto.Predicate_photo || photo.GetId() != 8 || len(photo.GetVideoSizes()) != 1 {
		t.Fatalf("video-only photo %#v", photo)
	}
	userPhoto := mtproto.MakeUserProfilePhotoByPhoto(photo)
	if userPhoto.GetPhotoId() != 8 || !userPhoto.GetHasVideo() {
		t.Fatalf("profile %#v", userPhoto)
	}

	empty := (&CachePhotoData{Id: 9, Photo: &dataobject.PhotosDO{PhotoId: 9}}).ToPhoto()
	if empty.GetPredicateName() != mtproto.Predicate_photoEmpty {
		t.Fatalf("no sizes %#v", empty)
	}
}
