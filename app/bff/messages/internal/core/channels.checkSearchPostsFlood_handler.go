package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/postsearch"
)

// ChannelsCheckSearchPostsFlood
// channels.checkSearchPostsFlood#22567115 flags:# query:flags.0?string = SearchPostsFlood;
//
// The query does not change the price. Search is free: stars_amount is 0 and
// query_is_free is true. total_daily and remains come from the Shanghai-day
// Redis counter (TELLOMI_POST_SEARCH_DAILY, default 100).
func (c *MessagesCore) ChannelsCheckSearchPostsFlood(in *mtproto.TLChannelsCheckSearchPostsFlood) (*mtproto.SearchPostsFlood, error) {
	_ = in
	return postsearch.Shared().Peek(c.ctx, c.MD.UserId), nil
}
