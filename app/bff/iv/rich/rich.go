// Package rich loads the channel message behind messages.getRichMessage.
// Android names that constructor iv.getRichMessage. Private and basic-group
// histories are not stored on this path.
package rich

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// Load returns the channel message, or a documented 400.
func Load(ctx context.Context, self int64, peer *mtproto.InputPeer, id int32) (*mtproto.Messages_Messages, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if id <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	p := mtproto.FromInputPeer2(self, peer)
	if p.PeerType != mtproto.PEER_CHANNEL || p.PeerId == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	in := &mtproto.InputChannel{
		PredicateName: mtproto.Predicate_inputChannel,
		ChannelId:     p.PeerId,
		AccessHash:    p.AccessHash,
	}
	page, err := channel.Shared().GetMessages(ctx, self, in, []int32{id})
	if err != nil {
		return nil, err
	}
	if len(page.GetMessages()) == 0 || page.Messages[0].GetPredicateName() == mtproto.Predicate_messageEmpty {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return page, nil
}
