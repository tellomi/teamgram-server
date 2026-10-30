package book

import (
	"testing"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func slugInvoice(slug string) *mtproto.InputInvoice {
	return mtproto.MakeTLInputInvoiceSlug(&mtproto.InputInvoice{Slug: slug}).To_InputInvoice()
}

func messageInvoice(peer *mtproto.InputPeer, msg int32) *mtproto.InputInvoice {
	return mtproto.MakeTLInputInvoiceMessage(&mtproto.InputInvoice{Peer: peer, MsgId: msg}).To_InputInvoice()
}

func cardJSON() *mtproto.InputPaymentCredentials {
	return mtproto.MakeTLInputPaymentCredentials(&mtproto.InputPaymentCredentials{
		Data: mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: `{"card":"ok"}`}).To_DataJSON(),
	}).To_InputPaymentCredentials()
}

func requestedName(name string) *mtproto.PaymentRequestedInfo {
	return mtproto.MakeTLPaymentRequestedInfo(&mtproto.PaymentRequestedInfo{
		Name: wrapperspb.String(name),
	}).To_PaymentRequestedInfo()
}

func TestPaymentFormAndCharge(t *testing.T) {
	b := New()
	if _, err := b.PaymentForm(0, nil); codeOf(err) != CodeAuthKeyUnregistered {
		t.Fatal(codeOf(err))
	}
	if _, err := b.PaymentForm(7, nil); codeOf(err) != CodeInvoiceInvalid {
		t.Fatal(codeOf(err))
	}
	form := &mtproto.TLPaymentsGetPaymentForm{Invoice: slugInvoice("gift_code")}
	if _, err := b.PaymentForm(7, form); codeOf(err) != CodeInvoiceInvalid {
		t.Fatal(codeOf(err))
	}
	form.ThemeParams = &mtproto.DataJSON{PredicateName: "nope"}
	if _, err := b.PaymentForm(7, form); codeOf(err) != CodeThemeParamsInvalid {
		t.Fatal(codeOf(err))
	}
	form.ThemeParams = nil
	form.Invoice = messageInvoice(nil, 1)
	if _, err := b.PaymentForm(7, form); codeOf(err) != CodePeerInvalid {
		t.Fatal(codeOf(err))
	}
	form.Invoice = messageInvoice(selfPeer(), 0)
	if _, err := b.PaymentForm(7, form); codeOf(err) != CodeMsgInvalid {
		t.Fatal(codeOf(err))
	}
	gift := mtproto.MakeTLInputInvoiceStarGift(&mtproto.InputInvoice{
		Peer: selfPeer(), GiftId: 4,
	}).To_InputInvoice()
	if _, err := b.PaymentForm(7, &mtproto.TLPaymentsGetPaymentForm{Invoice: gift}); codeOf(err) != CodeStarGiftMissing {
		t.Fatal(codeOf(err))
	}

	send := &mtproto.TLPaymentsSendPaymentForm{
		FormId: 1, Invoice: slugInvoice("gift_code"), Credentials: cardJSON(),
	}
	if _, err := b.SendPaymentForm(7, send); codeOf(err) != CodePaymentFailed {
		t.Fatal(codeOf(err))
	}
	send.FormId = 0
	if _, err := b.SendPaymentForm(7, send); codeOf(err) != CodeFormInvalid {
		t.Fatal(codeOf(err))
	}
	send.FormId = 1
	send.Credentials = mtproto.MakeTLInputPaymentCredentialsSaved(&mtproto.InputPaymentCredentials{Id: "card"}).To_InputPaymentCredentials()
	if _, err := b.SendPaymentForm(7, send); codeOf(err) != CodeTmpPasswordInvalid {
		t.Fatal(codeOf(err))
	}
	send.ShippingOptionId = wrapperspb.String("ship")
	send.Credentials = cardJSON()
	if _, err := b.SendPaymentForm(7, send); codeOf(err) != CodeShippingOption {
		t.Fatal(codeOf(err))
	}
}

func TestSavedInfo(t *testing.T) {
	b := New()
	info := &mtproto.TLPaymentsValidateRequestedInfo{
		Invoice: slugInvoice("gift_code"),
		Info:    requestedName("Ada Lovelace"),
		Save:    true,
	}
	got, err := b.ValidateRequestedInfo(7, info)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetId().GetValue() == "" {
		t.Fatal("missing id")
	}
	roundTrip(t, got)
	saved, err := b.SavedInfo(7)
	if err != nil {
		t.Fatal(err)
	}
	if saved.GetSavedInfo().GetName().GetValue() != "Ada Lovelace" || saved.GetHasSavedCredentials() {
		t.Fatalf("%+v", saved)
	}
	roundTrip(t, saved)

	send := &mtproto.TLPaymentsSendPaymentForm{
		FormId: 3, Invoice: slugInvoice("gift_code"), Credentials: cardJSON(),
		RequestedInfoId: wrapperspb.String("nope"),
	}
	if _, err := b.SendPaymentForm(7, send); codeOf(err) != CodeRequestedInfoInvalid {
		t.Fatal(codeOf(err))
	}
	send.RequestedInfoId = wrapperspb.String(got.GetId().GetValue())
	if _, err := b.SendPaymentForm(7, send); codeOf(err) != CodePaymentFailed {
		t.Fatal(codeOf(err))
	}

	if _, err := b.ClearSavedInfo(7, false, true); err != nil {
		t.Fatal(err)
	}
	saved, err = b.SavedInfo(7)
	if err != nil || saved.GetSavedInfo() != nil {
		t.Fatal(err, saved)
	}
	bad := &mtproto.TLPaymentsValidateRequestedInfo{
		Invoice: slugInvoice("gift_code"),
		Info:    requestedName(""),
	}
	if _, err := b.ValidateRequestedInfo(7, bad); codeOf(err) != "NAME_INVALID" {
		t.Fatal(codeOf(err))
	}
}

func TestBankCardAndRates(t *testing.T) {
	b := New()
	if _, err := b.BankCardData(7, "4242424242424241"); codeOf(err) != "BANK_CARD_NUMBER_INVALID" {
		t.Fatal(codeOf(err))
	}
	got, err := b.BankCardData(7, "4242 4242 4242 4242")
	if err != nil {
		t.Fatal(err)
	}
	if got.GetTitle() != "Visa" || len(got.GetOpenUrls()) != 0 {
		t.Fatalf("%+v", got)
	}
	roundTrip(t, got)
	rates, err := b.CurrencyRates(7)
	if err != nil || len(rates.GetRates()) != 0 {
		t.Fatal(err, rates)
	}
	roundTrip(t, rates)
	if _, err := b.PaymentReceipt(7, selfPeer(), 9); codeOf(err) != CodeMessageIDInvalid {
		t.Fatal(codeOf(err))
	}
}

func TestOnramp(t *testing.T) {
	b := New()
	providers, err := b.OnrampProviders(7, wrapperspb.String("TON"))
	if err != nil || len(providers.GetDatas()) != 0 {
		t.Fatal(err, providers)
	}
	if _, err := b.OnrampProviders(7, wrapperspb.String("")); codeOf(err) != CodeOnrampCurrency {
		t.Fatal(codeOf(err))
	}
	bases, err := b.OnrampBaseCurrencies(7, "none", "TON")
	if err != nil || len(bases.GetDatas()) != 0 {
		t.Fatal(err, bases)
	}
	avail, err := b.OnrampAvailability(7, &mtproto.TLPaymentsGetOnrampAvailability{
		Provider: "none", CryptoCurrency: "TON",
	})
	if err != nil || avail.GetAllowed() || avail.GetBuyAllowed() || avail.GetCountryCode() != "ZZ" {
		t.Fatal(err, avail)
	}
	roundTrip(t, avail)
	limits, err := b.OnrampLimits(7, &mtproto.TLPaymentsGetOnrampLimits{
		Provider: "none", CryptoCurrency: "TON", BaseCurrency: "USD",
	})
	if err != nil || limits.GetBaseMaxAmount() != "0" || limits.GetCryptoMaxAmount() != "0" {
		t.Fatal(err, limits)
	}
	roundTrip(t, limits)
	quote := &mtproto.TLPaymentsGetOnrampQuote{
		Provider: "none", CryptoCurrency: "TON", BaseCurrency: "USD",
		BaseAmount: wrapperspb.String("10"),
	}
	if _, err := b.OnrampQuote(7, quote); codeOf(err) != CodeOnrampUnavailable {
		t.Fatal(codeOf(err))
	}
	quote.CryptoAmount = wrapperspb.String("1")
	if _, err := b.OnrampQuote(7, quote); codeOf(err) != CodeOnrampAmount {
		t.Fatal(codeOf(err))
	}
	session := &mtproto.TLPaymentsCreateOnrampSession{
		Provider: "none", CryptoCurrency: "TON", Address: "EQAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	if _, err := b.CreateOnrampSession(7, session); codeOf(err) != CodeOnrampUnavailable {
		t.Fatal(codeOf(err))
	}
	session.Address = "short"
	if _, err := b.CreateOnrampSession(7, session); codeOf(err) != CodeOnrampCurrency {
		t.Fatal(codeOf(err))
	}
}
