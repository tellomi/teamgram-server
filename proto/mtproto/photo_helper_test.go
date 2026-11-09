package mtproto

import (
	"bytes"
	"testing"
)

func TestUserProfilePhotoCarriesStrippedThumbAndDC(t *testing.T) {
	thumb := []byte{0xff, 0xd8, 0xff, 0xd9}
	photo := MakeTLPhoto(&Photo{
		Id:   99,
		DcId: 1,
		Sizes: []*PhotoSize{
			MakeTLPhotoStrippedSize(&PhotoSize{Type: PhotoSZStrippedType, Bytes: thumb}).To_PhotoSize(),
			MakeTLPhotoSize(&PhotoSize{Type: PhotoSZAType, W: 160, H: 160, Size2: 100}).To_PhotoSize(),
		},
	}).To_Photo()

	got := MakeUserProfilePhotoByPhoto(photo)
	if got.GetPredicateName() != Predicate_userProfilePhoto {
		t.Fatalf("predicate %s", got.GetPredicateName())
	}
	if got.GetPhotoId() != 99 || got.GetDcId() != 1 {
		t.Fatalf("id %d dc %d", got.GetPhotoId(), got.GetDcId())
	}
	if !bytes.Equal(got.GetStrippedThumb(), thumb) {
		t.Fatalf("stripped %v", got.GetStrippedThumb())
	}

	buf := NewEncodeBuf(128)
	if err := got.Encode(buf, Layer); err != nil {
		t.Fatal(err)
	}
	decoded := &UserProfilePhoto{}
	decoded.Constructor = CRC32_userProfilePhoto
	if err := decoded.Decode(NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	if decoded.GetPhotoId() != 99 || !bytes.Equal(decoded.GetStrippedThumb(), thumb) || decoded.GetDcId() != 1 {
		t.Fatalf("wire photo id=%d dc=%d thumb=%v", decoded.GetPhotoId(), decoded.GetDcId(), decoded.GetStrippedThumb())
	}
}

func TestUserProfilePhotoUsesPhotoDCAndVideoOnlyAvatar(t *testing.T) {
	photo := MakeTLPhoto(&Photo{
		Id:   7,
		DcId: 1,
		Sizes: []*PhotoSize{
			MakeTLPhotoSize(&PhotoSize{Type: PhotoSZAType, W: 160, H: 160, Size2: 10}).To_PhotoSize(),
		},
		VideoSizes: []*VideoSize{
			MakeTLVideoSize(&VideoSize{Type: VideoSZVType, W: 800, H: 800, Size2: 20}).To_VideoSize(),
		},
	}).To_Photo()
	got := MakeUserProfilePhotoByPhoto(photo)
	if !got.GetHasVideo() || got.GetStrippedThumb() != nil || got.GetDcId() != 1 {
		t.Fatalf("video avatar %#v", got)
	}

	emoji := MakeTLPhoto(&Photo{
		Id:   8,
		DcId: 0,
		VideoSizes: []*VideoSize{
			MakeTLVideoSizeEmojiMarkup(&VideoSize{EmojiId: 3, BackgroundColors: []int32{1}}).To_VideoSize(),
		},
	}).To_Photo()
	got = MakeUserProfilePhotoByPhoto(emoji)
	if got.GetPhotoId() != 8 || !got.GetHasVideo() || got.GetDcId() != 1 {
		t.Fatalf("emoji avatar %#v", got)
	}

	if MakeUserProfilePhotoByPhoto(MakeTLPhotoEmpty(&Photo{Id: 1}).To_Photo()).GetPredicateName() != Predicate_userProfilePhotoEmpty {
		t.Fatal("empty photo must stay empty")
	}
}

func TestChatPhotoCarriesStrippedThumb(t *testing.T) {
	thumb := []byte{1, 2, 3, 4}
	photo := MakeTLPhoto(&Photo{
		Id: 5,
		Sizes: []*PhotoSize{
			MakeTLPhotoStrippedSize(&PhotoSize{Type: PhotoSZStrippedType, Bytes: thumb}).To_PhotoSize(),
			MakeTLPhotoSize(&PhotoSize{Type: PhotoSZCType, W: 640, H: 640, Size2: 10}).To_PhotoSize(),
		},
	}).To_Photo()
	got := MakeChatPhotoByPhoto(photo)
	if got.GetPhotoId() != 5 || !bytes.Equal(got.GetStrippedThumb(), thumb) || got.GetDcId() != 1 {
		t.Fatalf("chat photo %#v", got)
	}
}
