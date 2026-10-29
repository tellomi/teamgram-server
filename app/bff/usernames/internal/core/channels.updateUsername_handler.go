package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ChannelsUpdateUsername
// channels.updateUsername#3514b3de channel:InputChannel username:string = Bool;
func (c *UsernamesCore) ChannelsUpdateUsername(in *mtproto.TLChannelsUpdateUsername) (*mtproto.Bool, error) {
	ok, err := channel.Shared().UpdateUsername(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, in.GetChannel(), in.GetUsername())
	if err != nil {
		c.Logger.Errorf("channels.updateUsername - error: %v", err)
		return nil, err
	}
	channelID := in.GetChannel().GetChannelId()
	if in.GetUsername() == "" {
		_, _ = c.svcCtx.Dao.UserClient.UserDeleteUsernameByPeer(c.ctx, &user.TLUserDeleteUsernameByPeer{
			PeerType: mtproto.PEER_CHANNEL,
			PeerId:   channelID,
		})
	} else {
		_, err = c.svcCtx.Dao.UserClient.UserUpdateUsernameByPeer(c.ctx, &user.TLUserUpdateUsernameByPeer{
			PeerType: mtproto.PEER_CHANNEL,
			PeerId:   channelID,
			Username: in.GetUsername(),
		})
		if err != nil {
			c.Logger.Errorf("channels.updateUsername - user service: %v", err)
		}
	}
	return ok, nil
}
