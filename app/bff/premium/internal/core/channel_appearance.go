package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// ChannelsUpdateColor sets a channel name color or profile color.
func (c *PremiumCore) ChannelsUpdateColor(in *mtproto.TLChannelsUpdateColor) (*mtproto.Updates, error) {
	if c.MD == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return channel.Shared().UpdateColor(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, in)
}

// ChannelsUpdateEmojiStatus sets or clears a channel emoji status.
func (c *PremiumCore) ChannelsUpdateEmojiStatus(in *mtproto.TLChannelsUpdateEmojiStatus) (*mtproto.Updates, error) {
	if c.MD == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return channel.Shared().UpdateEmojiStatus(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, in.GetChannel(), in.GetEmojiStatus())
}
