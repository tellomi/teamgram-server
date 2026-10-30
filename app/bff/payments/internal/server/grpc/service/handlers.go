package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

func (s *Service) PaymentsGetStarsTopupOptions(ctx context.Context, _ *mtproto.TLPaymentsGetStarsTopupOptions) (*mtproto.Vector_StarsTopupOption, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.StarsTopupOptions(id)
}

func (s *Service) PaymentsGetStarsStatus(ctx context.Context, in *mtproto.TLPaymentsGetStarsStatus) (*mtproto.Payments_StarsStatus, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	ton := false
	if in != nil {
		peer = in.GetPeer()
		ton = in.GetTon()
	}
	return s.svcCtx.Book.StarsStatus(id, peer, ton)
}

func (s *Service) PaymentsGetStarsTransactions(ctx context.Context, in *mtproto.TLPaymentsGetStarsTransactions) (*mtproto.Payments_StarsStatus, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.StarsTransactions(id, in)
}

func (s *Service) PaymentsSendStarsForm(ctx context.Context, in *mtproto.TLPaymentsSendStarsForm) (*mtproto.Payments_PaymentResult, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var formID int64
	var invoice *mtproto.InputInvoice
	if in != nil {
		formID = in.GetFormId()
		invoice = in.GetInvoice()
	}
	return nil, s.svcCtx.Book.SendStarsForm(id, formID, invoice)
}

func (s *Service) PaymentsRefundStarsCharge(ctx context.Context, in *mtproto.TLPaymentsRefundStarsCharge) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var user *mtproto.InputUser
	charge := ""
	if in != nil {
		user = in.GetUserId()
		charge = in.GetChargeId()
	}
	return nil, s.svcCtx.Book.RefundStarsCharge(id, user, charge)
}

func (s *Service) PaymentsGetStarsRevenueStats(ctx context.Context, in *mtproto.TLPaymentsGetStarsRevenueStats) (*mtproto.Payments_StarsRevenueStats, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	ton := false
	if in != nil {
		peer = in.GetPeer()
		ton = in.GetTon()
	}
	return s.svcCtx.Book.StarsRevenueStats(id, peer, ton)
}

func (s *Service) PaymentsGetStarsRevenueWithdrawalUrl(ctx context.Context, in *mtproto.TLPaymentsGetStarsRevenueWithdrawalUrl) (*mtproto.Payments_StarsRevenueWithdrawalUrl, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return nil, s.svcCtx.Book.StarsRevenueWithdrawalURL(id, in)
}

func (s *Service) PaymentsGetStarsRevenueAdsAccountUrl(ctx context.Context, in *mtproto.TLPaymentsGetStarsRevenueAdsAccountUrl) (*mtproto.Payments_StarsRevenueAdsAccountUrl, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	if in != nil {
		peer = in.GetPeer()
	}
	return nil, s.svcCtx.Book.StarsRevenueAdsAccountURL(id, peer)
}

func (s *Service) PaymentsGetStarsTransactionsByID(ctx context.Context, in *mtproto.TLPaymentsGetStarsTransactionsByID) (*mtproto.Payments_StarsStatus, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.StarsTransactionsByID(id, in)
}

func (s *Service) PaymentsGetStarsGiftOptions(ctx context.Context, in *mtproto.TLPaymentsGetStarsGiftOptions) (*mtproto.Vector_StarsGiftOption, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var user *mtproto.InputUser
	if in != nil {
		user = in.GetUserId()
	}
	return s.svcCtx.Book.StarsGiftOptions(id, user)
}

func (s *Service) PaymentsGetStarsSubscriptions(ctx context.Context, in *mtproto.TLPaymentsGetStarsSubscriptions) (*mtproto.Payments_StarsStatus, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.StarsSubscriptions(id, in)
}

func (s *Service) PaymentsChangeStarsSubscription(ctx context.Context, in *mtproto.TLPaymentsChangeStarsSubscription) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return nil, s.svcCtx.Book.ChangeStarsSubscription(id, in)
}

func (s *Service) PaymentsFulfillStarsSubscription(ctx context.Context, in *mtproto.TLPaymentsFulfillStarsSubscription) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	sub := ""
	if in != nil {
		peer = in.GetPeer()
		sub = in.GetSubscriptionId()
	}
	return nil, s.svcCtx.Book.FulfillStarsSubscription(id, peer, sub)
}

func (s *Service) PaymentsBotCancelStarsSubscription(ctx context.Context, in *mtproto.TLPaymentsBotCancelStarsSubscription) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var user *mtproto.InputUser
	charge := ""
	if in != nil {
		user = in.GetUserId()
		charge = in.GetChargeId_STRING()
		if in.GetChargeId_FLAGSTRING() != nil && in.GetChargeId_FLAGSTRING().GetValue() != "" {
			charge = in.GetChargeId_FLAGSTRING().GetValue()
		}
	}
	return nil, s.svcCtx.Book.BotCancelStarsSubscription(id, user, charge)
}

func (s *Service) PaymentsGetPremiumGiftCodeOptions(ctx context.Context, in *mtproto.TLPaymentsGetPremiumGiftCodeOptions) (*mtproto.Vector_PremiumGiftCodeOption, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	if in != nil {
		peer = in.GetBoostPeer()
	}
	return s.svcCtx.Book.PremiumGiftCodeOptions(id, peer)
}

func (s *Service) PaymentsGetGiveawayInfo(ctx context.Context, in *mtproto.TLPaymentsGetGiveawayInfo) (*mtproto.Payments_GiveawayInfo, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	var msgID int32
	if in != nil {
		peer = in.GetPeer()
		msgID = in.GetMsgId()
	}
	return nil, s.svcCtx.Book.GiveawayInfo(id, peer, msgID)
}

func (s *Service) PaymentsLaunchPrepaidGiveaway(ctx context.Context, in *mtproto.TLPaymentsLaunchPrepaidGiveaway) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	var giveawayID int64
	var purpose *mtproto.InputStorePaymentPurpose
	if in != nil {
		peer = in.GetPeer()
		giveawayID = in.GetGiveawayId()
		purpose = in.GetPurpose()
	}
	return nil, s.svcCtx.Book.LaunchPrepaidGiveaway(id, peer, giveawayID, purpose)
}

func (s *Service) PaymentsGetStarsGiveawayOptions(ctx context.Context, _ *mtproto.TLPaymentsGetStarsGiveawayOptions) (*mtproto.Vector_StarsGiveawayOption, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.StarsGiveawayOptions(id)
}

func (s *Service) PaymentsCheckGiftCode(ctx context.Context, in *mtproto.TLPaymentsCheckGiftCode) (*mtproto.Payments_CheckedGiftCode, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	slug := ""
	if in != nil {
		slug = in.GetSlug()
	}
	return nil, s.svcCtx.Book.CheckGiftCode(id, slug)
}

func (s *Service) PaymentsApplyGiftCode(ctx context.Context, in *mtproto.TLPaymentsApplyGiftCode) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	slug := ""
	if in != nil {
		slug = in.GetSlug()
	}
	return nil, s.svcCtx.Book.ApplyGiftCode(id, slug)
}
