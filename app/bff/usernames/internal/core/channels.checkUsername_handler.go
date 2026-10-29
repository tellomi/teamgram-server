package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ChannelsCheckUsername
// channels.checkUsername#10e6bd2c channel:InputChannel username:string = Bool;
func (c *UsernamesCore) ChannelsCheckUsername(in *mtproto.TLChannelsCheckUsername) (*mtproto.Bool, error) {
	ok, err := channel.Shared().CheckUsername(c.ctx, c.MD.UserId, in.GetChannel(), in.GetUsername())
	if err != nil {
		c.Logger.Errorf("channels.checkUsername - error: %v", err)
		return nil, usernameRPCError(err)
	}
	checked, err := c.svcCtx.Dao.UserClient.UserCheckChannelUsername(c.ctx, &user.TLUserCheckChannelUsername{
		ChannelId: in.GetChannel().GetChannelId(),
		Username:  in.GetUsername(),
	})
	if err != nil {
		c.Logger.Errorf("channels.checkUsername - user service: %v", err)
		return ok, nil
	}
	switch checked.GetPredicateName() {
	case user.Predicate_usernameExisted, user.Predicate_usernameExistedNotMe:
		return nil, mtproto.ErrUsernameOccupied
	default:
		return ok, nil
	}
}
