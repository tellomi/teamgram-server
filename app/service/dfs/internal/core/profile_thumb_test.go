package core

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
)

func TestStrippedPhotoSizeRoundTrip(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(y * 5), B: 40, A: 255})
		}
	}
	var jpeg bytes.Buffer
	if err := imaging.EncodeJpeg(&jpeg, img); err != nil {
		t.Fatal(err)
	}
	sz := strippedPhotoSize(jpeg.Bytes())
	if sz.GetPredicateName() != mtproto.Predicate_photoStrippedSize || sz.GetType() != mtproto.PhotoSZStrippedType || len(sz.GetBytes()) == 0 {
		t.Fatalf("stripped %#v", sz)
	}
	sizes := appendStrippedSize(nil, jpeg.Bytes())
	if len(sizes) != 1 {
		t.Fatalf("sizes %d", len(sizes))
	}
	again := appendStrippedSize(sizes, jpeg.Bytes())
	if len(again) != 1 {
		t.Fatal("stripped size must be added once")
	}

	photo := mtproto.MakeTLPhoto(&mtproto.Photo{
		Id:    42,
		DcId:  1,
		Sizes: sizes,
	}).To_Photo()
	userPhoto := mtproto.MakeUserProfilePhotoByPhoto(photo)
	if len(userPhoto.GetStrippedThumb()) == 0 || userPhoto.GetPhotoId() != 42 {
		t.Fatalf("user photo %#v", userPhoto)
	}
}

func TestPeerPhotoSizeOrder(t *testing.T) {
	small := peerPhotoSizeOrder(false)
	if small[0] != mtproto.PhotoSZAType || small[len(small)-1] != mtproto.PhotoSZDType {
		t.Fatalf("small %v", small)
	}
	big := peerPhotoSizeOrder(true)
	if big[0] != mtproto.PhotoSZCType || big[len(big)-1] != mtproto.PhotoSZAType {
		t.Fatalf("big %v", big)
	}
}
