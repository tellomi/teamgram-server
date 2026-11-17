package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// reactionBlockAPI is the block lookup messages.sendReaction uses for a
// private chat. A transport error is not a block: the reaction still goes
// through, the same way other block checks fail open.
type reactionBlockAPI interface {
	UserIsBlockedByUser(ctx context.Context, in *user.TLUserIsBlockedByUser) (*mtproto.Bool, error)
}

// privateReactionBlocked stops a private-chat reaction that either side of
// a block would otherwise store and push.
//
// core.telegram.org/method/messages.sendReaction lists 400 USER_IS_BLOCKED
// ("You were blocked by this user"). The reaction is rejected before it is
// stored, so the author is not pushed updateMessageReactions and does not
// see it on the message. A silent accept would leave the reaction on the
// message for the next history load.
//
// The reverse direction is 400 YOU_BLOCKED_USER ("You blocked this user"),
// the same error messages.sendMessage returns when the sender is the one
// who blocked the peer. The reaction is not stored and not pushed, because
// the other person would see it on their own message. A mutual block
// reports USER_IS_BLOCKED: the peer blocked the sender.
func privateReactionBlocked(ctx context.Context, api reactionBlockAPI, self int64, peer *mtproto.PeerUtil) error {
	if api == nil || peer == nil || peer.PeerType != mtproto.PEER_USER || peer.PeerId == 0 || peer.PeerId == self {
		return nil
	}
	blocked, err := api.UserIsBlockedByUser(ctx, &user.TLUserIsBlockedByUser{
		UserId:     peer.PeerId,
		PeerUserId: self,
	})
	if err == nil && mtproto.FromBool(blocked) {
		return mtproto.ErrUserIsBlocked
	}
	blocked, err = api.UserIsBlockedByUser(ctx, &user.TLUserIsBlockedByUser{
		UserId:     self,
		PeerUserId: peer.PeerId,
	})
	if err == nil && mtproto.FromBool(blocked) {
		return mtproto.ErrYouBlockedUser
	}
	return nil
}
