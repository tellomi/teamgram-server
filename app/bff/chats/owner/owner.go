// Package owner serves messages.editChatCreator and
// messages.getFutureChatCreatorAfterLeave.
//
// https://core.telegram.org/method/messages.editChatCreator
// https://core.telegram.org/method/messages.getFutureChatCreatorAfterLeave
package owner

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/pkg/accesshash"
)

// Seat is one basic-group member used to pick the next creator.
type Seat struct {
	UserID int64
	Admin  bool
	Date   int64
	Active bool
}

// PasswordMissing is true when the client did not submit an SRP proof.
// Cloud password verifiers are not stored, so that case is PASSWORD_MISSING.
func PasswordMissing(p *mtproto.InputCheckPasswordSRP) bool {
	if p == nil {
		return true
	}
	if p.GetPredicateName() == mtproto.Predicate_inputCheckPasswordEmpty || p.Constructor == mtproto.CRC32_inputCheckPasswordEmpty {
		return true
	}
	return len(p.GetA()) == 0
}

// Successor is the admin, or else the oldest other member, who becomes
// creator when the current creator leaves.
func Successor(self int64, seats []Seat) (int64, error) {
	var adminID, memberID int64
	var adminDate, memberDate int64 = 1<<62 - 1, 1<<62 - 1
	for _, seat := range seats {
		if !seat.Active || seat.UserID == 0 || seat.UserID == self {
			continue
		}
		if seat.Admin && seat.Date < adminDate {
			adminDate = seat.Date
			adminID = seat.UserID
		}
		if seat.Date < memberDate {
			memberDate = seat.Date
			memberID = seat.UserID
		}
	}
	if adminID != 0 {
		return adminID, nil
	}
	if memberID == 0 {
		return 0, mtproto.ErrChatAdminRequired
	}
	return memberID, nil
}

func inputChannel(peer *mtproto.PeerUtil) *mtproto.InputChannel {
	return mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  peer.PeerId,
		AccessHash: peer.AccessHash,
	}).To_InputChannel()
}

// EditChannel transfers a channel or supergroup. Basic groups are transferred
// by the chat service.
func EditChannel(ctx context.Context, self, auth int64, peer *mtproto.PeerUtil, userID int64, password *mtproto.InputCheckPasswordSRP) (*mtproto.Updates, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if peer == nil || !peer.IsChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if userID == 0 || userID == self {
		return nil, mtproto.ErrUserIdInvalid
	}
	return channel.Shared().EditCreator(ctx, self, auth, inputChannel(peer), userID, password)
}

// FutureChannel is the user id who would become creator of a channel.
func FutureChannel(ctx context.Context, self int64, peer *mtproto.PeerUtil) (int64, error) {
	if self == 0 {
		return 0, mtproto.ErrAuthKeyUnregistered
	}
	if peer == nil || !peer.IsChannel() {
		return 0, mtproto.ErrPeerIdInvalid
	}
	return channel.Shared().FutureCreator(ctx, self, inputChannel(peer))
}

// User is the user object messages.getFutureChatCreatorAfterLeave returns.
func User(self, id int64) *mtproto.User {
	u := mtproto.MakeTLUser(&mtproto.User{Id: id, Self: id == self}).To_User()
	accesshash.PaintUser(context.Background(), u)
	return u
}
