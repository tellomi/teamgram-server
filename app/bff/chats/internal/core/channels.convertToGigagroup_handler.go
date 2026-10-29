package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// ChannelsConvertToGigagroup
// channels.convertToGigagroup#b290c69 channel:InputChannel = Updates;
func (c *ChatsCore) ChannelsConvertToGigagroup(in *mtproto.TLChannelsConvertToGigagroup) (*mtproto.Updates, error) {
	up, err := channel.Shared().ConvertToGigagroup(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, in.GetChannel())
	if err != nil {
		c.Logger.Errorf("channels.convertToGigagroup - error: %v", err)
		return nil, err
	}
	return up, nil
}
