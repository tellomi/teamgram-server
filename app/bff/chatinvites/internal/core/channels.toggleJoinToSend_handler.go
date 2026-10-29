package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// ChannelsToggleJoinToSend
// channels.toggleJoinToSend#e4cb9580 channel:InputChannel enabled:Bool = Updates;
func (c *ChatInvitesCore) ChannelsToggleJoinToSend(in *mtproto.TLChannelsToggleJoinToSend) (*mtproto.Updates, error) {
	up, err := channel.Shared().ToggleJoinToSend(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, in.GetChannel(), mtproto.FromBool(in.GetEnabled()))
	if err != nil {
		c.Logger.Errorf("channels.toggleJoinToSend - error: %v", err)
		return nil, err
	}
	return up, nil
}
