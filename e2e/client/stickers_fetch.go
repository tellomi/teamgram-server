package e2e

import (
	"context"
	"fmt"

	"github.com/teamgram/proto/mtproto"
)

func stickerDocumentCtx(ctx context.Context, c *Client) (*mtproto.Document, error) {
	sets := []mtproto.TLObject{
		&mtproto.TLMessagesGetStickerSet{
			Constructor: mtproto.CRC32_messages_getStickerSet_c8a0ec74,
			Stickerset:  mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{ShortName: "Twemoji"}).To_InputStickerSet(),
		},
		&mtproto.TLMessagesGetStickerSet{
			Constructor: mtproto.CRC32_messages_getStickerSet_c8a0ec74,
			Stickerset:  mtproto.MakeTLInputStickerSetAnimatedEmoji(nil).To_InputStickerSet(),
		},
		&mtproto.TLMessagesGetStickerSet{
			Constructor: mtproto.CRC32_messages_getStickerSet_c8a0ec74,
			Stickerset:  mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{ShortName: "AnimatedEmoji"}).To_InputStickerSet(),
		},
	}
	var last error
	for _, req := range sets {
		res, err := c.Invoke(ctx, req)
		if err != nil {
			last = err
			continue
		}
		set, ok := res.(*mtproto.TLMessagesStickerSet)
		if !ok {
			continue
		}
		for _, doc := range set.GetDocuments() {
			if doc != nil && doc.GetId() != 0 {
				return doc, nil
			}
		}
	}
	if last != nil {
		return nil, last
	}
	return nil, fmt.Errorf("sticker sets have no documents yet")
}
