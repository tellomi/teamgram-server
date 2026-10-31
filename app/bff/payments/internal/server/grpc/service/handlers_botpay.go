package service

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/botadmin"
)

// MessagesSetBotShippingResults implements messages.setBotShippingResults.
// Checkout queries are not created here, so an unknown query_id is
// QUERY_ID_INVALID. https://core.telegram.org/method/messages.setBotShippingResults
func (s *Service) MessagesSetBotShippingResults(ctx context.Context, in *mtproto.TLMessagesSetBotShippingResults) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if !botadmin.CallerIsBot(ctx, id) {
		return nil, mtproto.ErrUserBotRequired
	}
	if in == nil || in.GetQueryId() == 0 {
		return nil, mtproto.ErrQueryIdInvalid
	}
	if msg := in.GetError(); msg != nil && utf8.RuneCountInString(msg.GetValue()) > 255 {
		return nil, mtproto.ErrMessageTooLong
	}
	for _, opt := range in.GetShippingOptions() {
		if opt == nil || strings.TrimSpace(opt.GetId()) == "" || strings.TrimSpace(opt.GetTitle()) == "" || len(opt.GetPrices()) == 0 {
			return nil, mtproto.ErrCurrencyTotalAmountInvalid
		}
		for _, price := range opt.GetPrices() {
			if price == nil || price.GetAmount() < 0 || strings.TrimSpace(price.GetLabel()) == "" {
				return nil, mtproto.ErrCurrencyTotalAmountInvalid
			}
		}
	}
	return nil, mtproto.ErrQueryIdInvalid
}

// MessagesSetBotPrecheckoutResults implements messages.setBotPrecheckoutResults.
// https://core.telegram.org/method/messages.setBotPrecheckoutResults
func (s *Service) MessagesSetBotPrecheckoutResults(ctx context.Context, in *mtproto.TLMessagesSetBotPrecheckoutResults) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if !botadmin.CallerIsBot(ctx, id) {
		return nil, mtproto.ErrUserBotRequired
	}
	if in == nil || in.GetQueryId() == 0 {
		return nil, mtproto.ErrQueryIdInvalid
	}
	if msg := in.GetError(); msg != nil && utf8.RuneCountInString(msg.GetValue()) > 255 {
		return nil, mtproto.ErrMessageTooLong
	}
	if !in.GetSuccess() && (in.GetError() == nil || strings.TrimSpace(in.GetError().GetValue()) == "") {
		return nil, mtproto.ErrQueryIdInvalid
	}
	return nil, mtproto.ErrQueryIdInvalid
}

// PaymentsExportInvoice implements payments.exportInvoice.
// A well-formed invoice still has no payment provider on this server.
// https://core.telegram.org/method/payments.exportInvoice
func (s *Service) PaymentsExportInvoice(ctx context.Context, in *mtproto.TLPaymentsExportInvoice) (*mtproto.Payments_ExportedInvoice, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if !botadmin.CallerIsBot(ctx, id) {
		return nil, mtproto.ErrUserBotRequired
	}
	media := (*mtproto.InputMedia)(nil)
	if in != nil {
		media = in.GetInvoiceMedia()
	}
	if media == nil || media.GetPredicateName() != mtproto.Predicate_inputMediaInvoice {
		return nil, mtproto.ErrMediaInvalid
	}
	if len(media.GetPayload_BYTES()) == 0 {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	inv := media.GetInvoice()
	if inv == nil || strings.TrimSpace(inv.GetCurrency()) == "" || len(inv.GetPrices()) == 0 {
		return nil, mtproto.ErrCurrencyTotalAmountInvalid
	}
	var total int64
	for _, price := range inv.GetPrices() {
		if price == nil || strings.TrimSpace(price.GetLabel()) == "" {
			return nil, mtproto.ErrCurrencyTotalAmountInvalid
		}
		total += price.GetAmount()
	}
	if total <= 0 {
		return nil, mtproto.ErrCurrencyTotalAmountInvalid
	}
	provider := ""
	if media.GetProvider_FLAGSTRING() != nil {
		provider = strings.TrimSpace(media.GetProvider_FLAGSTRING().GetValue())
	}
	if provider == "" {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if media.GetProviderData() != nil && !json.Valid([]byte(media.GetProviderData().GetData())) {
		return nil, mtproto.ErrDataJsonInvalid
	}
	return nil, mtproto.ErrPaymentProviderInvalid
}
