package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UpdatesGetChannelDifference
// updates.getChannelDifference#3173d78 flags:# force:flags.0?true channel:InputChannel filter:ChannelMessagesFilter pts:int limit:int = updates.ChannelDifference;
func (c *UpdatesCore) UpdatesGetChannelDifference(in *mtproto.TLUpdatesGetChannelDifference) (*mtproto.Updates_ChannelDifference, error) {
	diff, err := channel.Shared().Difference(c.ctx, c.MD.UserId, in.GetChannel(), in.GetPts(), in.GetLimit())
	if err != nil {
		c.Logger.Errorf("updates.getChannelDifference - error: %v", err)
		return nil, err
	}
	var ids []int64
	for _, m := range diff.GetNewMessages() {
		if m.GetFromId().GetUserId() != 0 {
			ids = append(ids, m.GetFromId().GetUserId())
		}
	}
	for _, m := range diff.GetMessages() {
		if m.GetFromId().GetUserId() != 0 {
			ids = append(ids, m.GetFromId().GetUserId())
		}
	}
	if len(ids) > 0 {
		users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: ids})
		if err == nil && users != nil {
			diff.Users = users.GetUserListByIdList(c.MD.UserId, ids...)
		}
	}
	mtproto.SanitizeOutgoingObject(diff)
	return diff, nil
}
