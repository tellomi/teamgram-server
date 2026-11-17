package payments_helper

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/payments/internal/book"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// BoostsList serves premium.getBoostsList on RPCBoosts.
// The list is for admins of the channel. A member who is not an admin, and
// anyone outside the channel, gets CHAT_ADMIN_REQUIRED.
// https://core.telegram.org/method/premium.getBoostsList
func BoostsList(self int64, in *mtproto.TLPremiumGetBoostsList) (*mtproto.Premium_BoostsList, error) {
	if err := boostListAllowed(self, in); err != nil {
		return nil, err
	}
	return book.Shared().BoostsList(self, in)
}

func boostListAllowed(self int64, in *mtproto.TLPremiumGetBoostsList) error {
	if self == 0 {
		return mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		return mtproto.ErrPeerIdInvalid
	}
	peer := in.GetPeer()
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerChannel:
		ok, err := channel.Shared().MayReadBoosts(context.Background(), self, peer.GetChannelId())
		if err != nil {
			return err
		}
		if !ok {
			return mtproto.ErrChatAdminRequired
		}
		return nil
	case mtproto.Predicate_inputPeerChat:
		// Boost lists are an admin view. A basic group has no in-process
		// admin record on this RPC, so it is not readable here.
		return mtproto.ErrChatAdminRequired
	default:
		return nil
	}
}

// MyBoosts serves premium.getMyBoosts on RPCBoosts.
func MyBoosts(self int64) (*mtproto.Premium_MyBoosts, error) {
	return book.Shared().MyBoosts(self)
}

// ApplyBoost serves premium.applyBoost on RPCBoosts.
func ApplyBoost(self int64, peer *mtproto.InputPeer, slots []int32, hasSlots bool) (*mtproto.Premium_MyBoosts, error) {
	return book.Shared().ApplyBoost(self, peer, slots, hasSlots)
}

// BoostsStatus serves premium.getBoostsStatus on RPCBoosts.
func BoostsStatus(self int64, peer *mtproto.InputPeer) (*mtproto.Premium_BoostsStatus, error) {
	return book.Shared().BoostsStatus(self, peer)
}

// UserBoosts serves premium.getUserBoosts on RPCBoosts.
func UserBoosts(self int64, peer *mtproto.InputPeer, user *mtproto.InputUser) (*mtproto.Premium_BoostsList, error) {
	return book.Shared().UserBoosts(self, peer, user)
}
