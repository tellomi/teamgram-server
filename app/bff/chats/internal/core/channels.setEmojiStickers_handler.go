package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// ChannelsSetEmojiStickers
// channels.setEmojiStickers#3cd930b7 channel:InputChannel stickerset:InputStickerSet = Bool;
func (c *ChatsCore) ChannelsSetEmojiStickers(in *mtproto.TLChannelsSetEmojiStickers) (*mtproto.Bool, error) {
	ok, err := channel.Shared().SetEmojiStickers(c.ctx, c.MD.UserId, in.GetChannel(), in.GetStickerset())
	if err != nil {
		c.Logger.Errorf("channels.setEmojiStickers - error: %v", err)
		return nil, err
	}
	return ok, nil
}
