package payments_helper

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/payments/internal/book"
)

// BoostsList serves premium.getBoostsList on RPCBoosts.
func BoostsList(self int64, in *mtproto.TLPremiumGetBoostsList) (*mtproto.Premium_BoostsList, error) {
	return book.Shared().BoostsList(self, in)
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
