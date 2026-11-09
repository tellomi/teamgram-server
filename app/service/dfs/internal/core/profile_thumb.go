package core

import (
	"bytes"
	"image"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
)

// strippedPhotoSize is the inline "i" thumb official clients paint in the
// dialog list, chat header, and message avatars before upload.getFile
// returns. The bytes are a Telegram stripped JPEG, not a normal file.
func strippedPhotoSize(jpeg []byte) *mtproto.PhotoSize {
	if len(jpeg) == 0 {
		return nil
	}
	img, err := imaging.Decode(bytes.NewReader(jpeg))
	if err != nil || img == nil {
		return nil
	}
	b := img.Bounds()
	var small image.Image
	if b.Dx() >= b.Dy() {
		small = imaging.Resize(img, 40, 0)
	} else {
		small = imaging.Resize(img, 0, 40)
	}
	if small == nil || small.Bounds().Dx() <= 0 || small.Bounds().Dy() <= 0 {
		return nil
	}
	var buf bytes.Buffer
	if err = imaging.EncodeStripped(&buf, small, 30); err != nil || buf.Len() == 0 {
		return nil
	}
	return mtproto.MakeTLPhotoStrippedSize(&mtproto.PhotoSize{
		Type:  mtproto.PhotoSZStrippedType,
		Bytes: buf.Bytes(),
	}).To_PhotoSize()
}

func appendStrippedSize(sizes []*mtproto.PhotoSize, jpeg []byte) []*mtproto.PhotoSize {
	for _, sz := range sizes {
		if sz.GetPredicateName() == mtproto.Predicate_photoStrippedSize || sz.GetType() == mtproto.PhotoSZStrippedType {
			return sizes
		}
	}
	thumb := strippedPhotoSize(jpeg)
	if thumb == nil {
		return sizes
	}
	return append(sizes, thumb)
}
