package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	channelpb "github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// forwardChats is the channel card list for a private-message update.
// A forwarded channel post has to name the channel or the other client
// cannot show "forwarded from".
func forwardChats(ctx context.Context, viewer int64, ids []int64) []*mtproto.Chat {
	if len(ids) == 0 {
		return []*mtproto.Chat{}
	}
	cards := channelpb.Shared().ForwardCards(ctx, viewer, ids...)
	if len(cards) == 0 {
		return []*mtproto.Chat{}
	}
	return cards
}
