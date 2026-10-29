package stickers_helper

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
)

// Thumb resolves a sticker-set thumbnail to the document bytes in object storage.
type Thumb struct{}

func (Thumb) GetStickerSetThumbFileLocation(ctx context.Context, userID int64, stickerset *mtproto.InputStickerSet, version int32) (*mtproto.InputFileLocation, error) {
	return catalog.Must().ThumbLocation(stickerset)
}
