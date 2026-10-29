package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// ChannelsToggleJoinRequest
// channels.toggleJoinRequest#ecc2618 flags:# channel:InputChannel enabled:Bool = Updates;
func (c *ChatInvitesCore) ChannelsToggleJoinRequest(in *mtproto.TLChannelsToggleJoinRequest) (*mtproto.Updates, error) {
	up, err := channel.Shared().ToggleJoinRequest(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, in.GetChannel(), mtproto.FromBool(in.GetEnabled()))
	if err != nil {
		c.Logger.Errorf("channels.toggleJoinRequest - error: %v", err)
		return nil, err
	}
	return up, nil
}
