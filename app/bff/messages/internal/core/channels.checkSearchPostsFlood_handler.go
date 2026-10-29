package core

import (
	"github.com/teamgram/proto/mtproto"
)

// ChannelsCheckSearchPostsFlood
// channels.checkSearchPostsFlood#22567115 flags:# query:flags.0?string = SearchPostsFlood;
func (c *MessagesCore) ChannelsCheckSearchPostsFlood(in *mtproto.TLChannelsCheckSearchPostsFlood) (*mtproto.SearchPostsFlood, error) {
	return mtproto.MakeTLSearchPostsFlood(&mtproto.SearchPostsFlood{
		QueryIsFree: true,
	}).To_SearchPostsFlood(), nil
}
