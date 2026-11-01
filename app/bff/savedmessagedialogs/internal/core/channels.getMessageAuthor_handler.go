package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ChannelsGetMessageAuthor
// channels.getMessageAuthor#ece2a0e6 channel:InputChannel id:int = User;
// https://core.telegram.org/method/channels.getMessageAuthor
//
// Non-bot admins of a monoforum receive the user who sent the message.
func (c *SavedMessageDialogsCore) ChannelsGetMessageAuthor(in *mtproto.TLChannelsGetMessageAuthor) (*mtproto.User, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	sender, err := channel.Shared().MonoforumAuthor(c.ctx, c.MD.UserId, in.GetChannel(), in.GetId())
	if err != nil {
		c.Logger.Errorf("channels.getMessageAuthor - error: %v", err)
		return nil, err
	}
	user, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: sender})
	if err != nil || user == nil {
		return mtproto.MakeTLUserEmpty(&mtproto.User{Id: sender}).To_User(), nil
	}
	return user.ToUser(c.MD.UserId), nil
}
