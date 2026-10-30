package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	paymentshelper "github.com/teamgram/teamgram-server/app/bff/payments"
)

func (BoostsService) PremiumGetBoostsList(ctx context.Context, in *mtproto.TLPremiumGetBoostsList) (*mtproto.Premium_BoostsList, error) {
	self, _ := actor(ctx)
	return paymentshelper.BoostsList(self, in)
}

func (BoostsService) PremiumGetMyBoosts(ctx context.Context, _ *mtproto.TLPremiumGetMyBoosts) (*mtproto.Premium_MyBoosts, error) {
	self, _ := actor(ctx)
	return paymentshelper.MyBoosts(self)
}

func (BoostsService) PremiumApplyBoost(ctx context.Context, in *mtproto.TLPremiumApplyBoost) (*mtproto.Premium_MyBoosts, error) {
	self, _ := actor(ctx)
	var peer *mtproto.InputPeer
	var slots []int32
	if in != nil {
		peer = in.GetPeer()
		slots = in.GetSlots()
	}
	return paymentshelper.ApplyBoost(self, peer, slots, len(slots) > 0)
}

func (BoostsService) PremiumGetBoostsStatus(ctx context.Context, in *mtproto.TLPremiumGetBoostsStatus) (*mtproto.Premium_BoostsStatus, error) {
	self, _ := actor(ctx)
	var peer *mtproto.InputPeer
	if in != nil {
		peer = in.GetPeer()
	}
	return paymentshelper.BoostsStatus(self, peer)
}

func (BoostsService) PremiumGetUserBoosts(ctx context.Context, in *mtproto.TLPremiumGetUserBoosts) (*mtproto.Premium_BoostsList, error) {
	self, _ := actor(ctx)
	var peer *mtproto.InputPeer
	var user *mtproto.InputUser
	if in != nil {
		peer = in.GetPeer()
		user = in.GetUserId()
	}
	return paymentshelper.UserBoosts(self, peer, user)
}
