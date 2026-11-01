// Package joinhide serves messages.hideAllChatJoinRequests for channels.
// Basic groups are dismissed by chat.hideChatJoinRequests with no user id.
//
// https://core.telegram.org/method/messages.hideAllChatJoinRequests
package joinhide

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// HideChannel dismisses every pending join request on a channel.
func HideChannel(ctx context.Context, self, auth int64, peer *mtproto.PeerUtil, approved bool, link string) (*mtproto.Updates, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if peer == nil || !peer.IsChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	in := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  peer.PeerId,
		AccessHash: peer.AccessHash,
	}).To_InputChannel()
	return channel.Shared().HideAllJoins(ctx, self, auth, in, approved, link)
}
