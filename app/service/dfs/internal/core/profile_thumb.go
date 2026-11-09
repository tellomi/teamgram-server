package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
)

// strippedPhotoSize is the inline "i" thumb official clients paint in the
// dialog list, chat header, and message avatars before upload.getFile
// returns. The bytes are a Telegram stripped JPEG, not a normal file.
func strippedPhotoSize(jpeg []byte) *mtproto.PhotoSize {
	raw, err := imaging.StrippedJPEG(jpeg)
	if err != nil || len(raw) == 0 {
		return nil
	}
	return mtproto.MakeTLPhotoStrippedSize(&mtproto.PhotoSize{
		Type:  mtproto.PhotoSZStrippedType,
		Bytes: raw,
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
