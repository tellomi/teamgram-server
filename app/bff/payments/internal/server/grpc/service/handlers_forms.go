package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	twofahelper "github.com/teamgram/teamgram-server/app/bff/twofa"
)

// AccountGetTmpPassword lives on RPCPayments. The two-factor package builds it.
func (s *Service) AccountGetTmpPassword(ctx context.Context, in *mtproto.TLAccountGetTmpPassword) (*mtproto.Account_TmpPassword, error) {
	return twofahelper.TmpPassword(ctx, in)
}

func (s *Service) PaymentsGetPaymentForm(ctx context.Context, in *mtproto.TLPaymentsGetPaymentForm) (*mtproto.Payments_PaymentForm, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.PaymentForm(id, in)
}

func (s *Service) PaymentsGetPaymentReceipt(ctx context.Context, in *mtproto.TLPaymentsGetPaymentReceipt) (*mtproto.Payments_PaymentReceipt, error) {
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
	return s.svcCtx.Book.PaymentReceipt(id, peer, msgID)
}

func (s *Service) PaymentsValidateRequestedInfo(ctx context.Context, in *mtproto.TLPaymentsValidateRequestedInfo) (*mtproto.Payments_ValidatedRequestedInfo, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.ValidateRequestedInfo(id, in)
}

func (s *Service) PaymentsSendPaymentForm(ctx context.Context, in *mtproto.TLPaymentsSendPaymentForm) (*mtproto.Payments_PaymentResult, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.SendPaymentForm(id, in)
}

func (s *Service) PaymentsGetSavedInfo(ctx context.Context, _ *mtproto.TLPaymentsGetSavedInfo) (*mtproto.Payments_SavedInfo, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.SavedInfo(id)
}

func (s *Service) PaymentsClearSavedInfo(ctx context.Context, in *mtproto.TLPaymentsClearSavedInfo) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	credentials, info := false, false
	if in != nil {
		credentials = in.GetCredentials()
		info = in.GetInfo()
	}
	return s.svcCtx.Book.ClearSavedInfo(id, credentials, info)
}

func (s *Service) PaymentsGetBankCardData(ctx context.Context, in *mtproto.TLPaymentsGetBankCardData) (*mtproto.Payments_BankCardData, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	number := ""
	if in != nil {
		number = in.GetNumber()
	}
	return s.svcCtx.Book.BankCardData(id, number)
}

func (s *Service) PaymentsGetCurrencyRates(ctx context.Context, _ *mtproto.TLPaymentsGetCurrencyRates) (*mtproto.Payments_CurrencyRates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.CurrencyRates(id)
}

func (s *Service) PaymentsGetOnrampProviders(ctx context.Context, in *mtproto.TLPaymentsGetOnrampProviders) (*mtproto.Vector_OnrampProviderInfo, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return s.svcCtx.Book.OnrampProviders(id, nil)
	}
	return s.svcCtx.Book.OnrampProviders(id, in.GetCryptoCurrency())
}

func (s *Service) PaymentsGetOnrampBaseCurrencies(ctx context.Context, in *mtproto.TLPaymentsGetOnrampBaseCurrencies) (*mtproto.Vector_String, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	provider, crypto := "", ""
	if in != nil {
		provider = in.GetProvider()
		crypto = in.GetCryptoCurrency()
	}
	return s.svcCtx.Book.OnrampBaseCurrencies(id, provider, crypto)
}

func (s *Service) PaymentsGetOnrampAvailability(ctx context.Context, in *mtproto.TLPaymentsGetOnrampAvailability) (*mtproto.OnrampAvailability, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.OnrampAvailability(id, in)
}

func (s *Service) PaymentsGetOnrampLimits(ctx context.Context, in *mtproto.TLPaymentsGetOnrampLimits) (*mtproto.OnrampLimits, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.OnrampLimits(id, in)
}

func (s *Service) PaymentsGetOnrampQuote(ctx context.Context, in *mtproto.TLPaymentsGetOnrampQuote) (*mtproto.OnrampQuote, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.OnrampQuote(id, in)
}

func (s *Service) PaymentsCreateOnrampSession(ctx context.Context, in *mtproto.TLPaymentsCreateOnrampSession) (*mtproto.OnrampSession, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.CreateOnrampSession(id, in)
}
