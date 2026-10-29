package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// ChannelsSearchPosts
// channels.searchPosts#f2c4f24d flags:# hashtag:flags.0?string query:flags.1?string offset_rate:int offset_peer:InputPeer offset_id:int limit:int = messages.Messages;
func (c *MessagesCore) ChannelsSearchPosts(in *mtproto.TLChannelsSearchPosts) (*mtproto.Messages_Messages, error) {
	res, err := channel.Shared().SearchPosts(c.ctx, c.MD.UserId, in.GetQuery().GetValue(), in.GetHashtag_STRING(), in.GetOffsetId(), in.GetLimit())
	if err != nil {
		c.Logger.Errorf("channels.searchPosts - error: %v", err)
		return nil, err
	}
	return res, nil
}
