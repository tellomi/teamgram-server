package book

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Checkout info is remembered in this process and dropped on restart.
// No invoice, receipt, card credential, or on-ramp session is stored.
const (
	CodeThemeParamsInvalid   = "THEME_PARAMS_INVALID"
	CodePremiumPurpose       = "PREMIUM_PURPOSE_INVALID"
	CodeCurrencyCodeInvalid  = "CURRENCY_CODE_INVALID"
	CodeMonthInvalid         = "MONTH_INVALID"
	CodeGiftStarsInvalid     = "GIFT_STARS_INVALID"
	CodeStarGiftMessage      = "STARGIFT_MESSAGE_INVALID"
	CodePaymentInfoInvalid   = "PAYMENT_INFO_INVALID"
	CodeAddressInvalid       = "SHIPPING_ADDRESS_INVALID"
	CodeShippingOption       = "SHIPPING_OPTION_INVALID"
	CodeRequestedInfoInvalid = "REQUESTED_INFO_INVALID"
	CodeCredentialsInvalid   = "PAYMENT_CREDENTIALS_INVALID"
	CodeTmpPasswordInvalid   = "TMP_PASSWORD_INVALID"
	CodePaymentFailed        = "PAYMENT_FAILED"
	CodeMessageIDInvalid     = "MESSAGE_ID_INVALID"
	CodeOnrampProvider       = "ONRAMP_PROVIDER_INVALID"
	CodeOnrampCurrency       = "ONRAMP_CURRENCY_INVALID"
	CodeOnrampAmount         = "ONRAMP_AMOUNT_INVALID"
	CodeOnrampMethod         = "ONRAMP_METHOD_INVALID"
	CodeOnrampUnavailable    = "ONRAMP_UNAVAILABLE"
	CodeURLInvalid           = "URL_INVALID"
	CodeThemeInvalid         = "THEME_INVALID"
	CodeMemoInvalid          = "MEMO_INVALID"

	maxInfoRunes   = 128
	maxGiftMessage = 256
	maxMonths      = 36
)

// payerState is one user's saved checkout info and the last info id issued.
type payerState struct {
	saved  *mtproto.PaymentRequestedInfo
	lastID string
	seq    int64
}

func (b *Book) payer(self int64) *payerState {
	row := b.payers[self]
	if row == nil {
		row = &payerState{}
		b.payers[self] = row
	}
	return row
}

// PaymentForm checks the invoice shape. No invoice is stored, so a
// well-formed plain invoice is INVOICE_INVALID and a well-formed gift
// invoice is STARGIFT_NOT_FOUND.
func (b *Book) PaymentForm(self int64, in *mtproto.TLPaymentsGetPaymentForm) (*mtproto.Payments_PaymentForm, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	var inv *mtproto.InputInvoice
	var theme *mtproto.DataJSON
	if in != nil {
		inv = in.GetInvoice()
		theme = in.GetThemeParams()
	}
	if err := themeParams(theme); err != nil {
		return nil, err
	}
	kind, err := invoiceShape(self, inv)
	if err != nil {
		return nil, err
	}
	if kind == invoiceGift {
		return nil, fail(CodeStarGiftMissing)
	}
	return nil, fail(CodeInvoiceInvalid)
}

// PaymentReceipt checks the peer and message id. No receipt is stored.
func (b *Book) PaymentReceipt(self int64, peer *mtproto.InputPeer, msgID int32) (*mtproto.Payments_PaymentReceipt, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if _, _, err := parsePeer(self, peer); err != nil {
		return nil, err
	}
	// A non-zero id is still not a receipt. No payment message is stored.
	return nil, fail(CodeMessageIDInvalid)
}

// ValidateRequestedInfo checks the checkout fields and returns an id.
// save stores the info for payments.getSavedInfo. Shipping options are
// not offered.
func (b *Book) ValidateRequestedInfo(self int64, in *mtproto.TLPaymentsValidateRequestedInfo) (*mtproto.Payments_ValidatedRequestedInfo, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	var inv *mtproto.InputInvoice
	var info *mtproto.PaymentRequestedInfo
	save := false
	if in != nil {
		inv = in.GetInvoice()
		info = in.GetInfo()
		save = in.GetSave()
	}
	if _, err := invoiceShape(self, inv); err != nil {
		return nil, err
	}
	cleaned, err := cleanRequestedInfo(info)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	row := b.payer(self)
	row.seq++
	row.lastID = "ri" + itoa(self) + "-" + itoa(row.seq)
	if save {
		row.saved = cleaned
	}
	return mtproto.MakeTLPaymentsValidatedRequestedInfo(&mtproto.Payments_ValidatedRequestedInfo{
		Id: wrapperspb.String(row.lastID),
	}).To_Payments_ValidatedRequestedInfo(), nil
}

// SavedInfo returns the info stored by validateRequestedInfo with save set.
// Credentials are never stored.
func (b *Book) SavedInfo(self int64) (*mtproto.Payments_SavedInfo, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := &mtproto.Payments_SavedInfo{}
	if row := b.payers[self]; row != nil {
		out.SavedInfo = row.saved
	}
	return mtproto.MakeTLPaymentsSavedInfo(out).To_Payments_SavedInfo(), nil
}

// ClearSavedInfo drops saved checkout info when info is set. credentials is
// accepted and there is nothing to drop. Neither flag leaves the info in place.
func (b *Book) ClearSavedInfo(self int64, credentials, info bool) (*mtproto.Bool, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if info {
		if row := b.payers[self]; row != nil {
			row.saved = nil
		}
	}
	_ = credentials
	return mtproto.BoolTrue, nil
}

// SendPaymentForm checks the form, invoice, and credentials. No charge runs.
func (b *Book) SendPaymentForm(self int64, in *mtproto.TLPaymentsSendPaymentForm) (*mtproto.Payments_PaymentResult, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if in == nil || in.GetFormId() <= 0 {
		return nil, fail(CodeFormInvalid)
	}
	if _, err := invoiceShape(self, in.GetInvoice()); err != nil {
		return nil, err
	}
	if err := credentialsShape(in.GetCredentials()); err != nil {
		return nil, err
	}
	if tip := in.GetTipAmount(); tip != nil && tip.GetValue() < 0 {
		return nil, fail(CodeAmountInvalid)
	}
	if ship := in.GetShippingOptionId(); ship != nil {
		return nil, fail(CodeShippingOption)
	}
	if infoID := in.GetRequestedInfoId(); infoID != nil {
		b.mu.Lock()
		row := b.payers[self]
		last := ""
		if row != nil {
			last = row.lastID
		}
		b.mu.Unlock()
		if strings.TrimSpace(infoID.GetValue()) == "" || infoID.GetValue() != last {
			return nil, fail(CodeRequestedInfoInvalid)
		}
	}
	return nil, fail(CodePaymentFailed)
}

// BankCardData classifies a Luhn-valid number. open_urls is empty.
func (b *Book) BankCardData(self int64, number string) (*mtproto.Payments_BankCardData, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	digits := cardDigits(number)
	if !luhn(digits) {
		return nil, mtproto.ErrBankCardNumberInvalid
	}
	return mtproto.MakeTLPaymentsBankCardData(&mtproto.Payments_BankCardData{
		Title:    cardTitle(digits),
		OpenUrls: []*mtproto.BankCardOpenUrl{},
	}).To_Payments_BankCardData(), nil
}

// CurrencyRates returns an empty rate list. No rate feed is configured.
func (b *Book) CurrencyRates(self int64) (*mtproto.Payments_CurrencyRates, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	return mtproto.MakeTLPaymentsCurrencyRates(&mtproto.Payments_CurrencyRates{
		Rates: []*mtproto.CurrencyRate{},
	}).To_Payments_CurrencyRates(), nil
}

// OnrampProviders returns an empty provider list.
func (b *Book) OnrampProviders(self int64, crypto *wrapperspb.StringValue) (*mtproto.Vector_OnrampProviderInfo, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if crypto != nil {
		if err := onrampToken(crypto.GetValue(), CodeOnrampCurrency); err != nil {
			return nil, err
		}
	}
	return &mtproto.Vector_OnrampProviderInfo{Datas: []*mtproto.OnrampProviderInfo{}}, nil
}

// OnrampBaseCurrencies returns an empty currency list for a well-formed pair.
func (b *Book) OnrampBaseCurrencies(self int64, provider, crypto string) (*mtproto.Vector_String, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if err := onrampID(provider, CodeOnrampProvider); err != nil {
		return nil, err
	}
	if err := onrampToken(crypto, CodeOnrampCurrency); err != nil {
		return nil, err
	}
	return &mtproto.Vector_String{Datas: []string{}}, nil
}

// OnrampAvailability reports that buying is not allowed. No provider is connected.
func (b *Book) OnrampAvailability(self int64, in *mtproto.TLPaymentsGetOnrampAvailability) (*mtproto.OnrampAvailability, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	provider, crypto := "", ""
	var base *wrapperspb.StringValue
	if in != nil {
		provider = in.GetProvider()
		crypto = in.GetCryptoCurrency()
		base = in.GetBaseCurrency()
	}
	if err := onrampID(provider, CodeOnrampProvider); err != nil {
		return nil, err
	}
	if err := onrampToken(crypto, CodeOnrampCurrency); err != nil {
		return nil, err
	}
	if base != nil {
		if err := onrampToken(base.GetValue(), CodeOnrampCurrency); err != nil {
			return nil, err
		}
	}
	return mtproto.MakeTLOnrampAvailability(&mtproto.OnrampAvailability{
		CountryCode: "ZZ",
		Methods:     []*mtproto.OnrampMethodAvailability{},
	}).To_OnrampAvailability(), nil
}

// OnrampLimits returns a zero range. No provider publishes a limit.
func (b *Book) OnrampLimits(self int64, in *mtproto.TLPaymentsGetOnrampLimits) (*mtproto.OnrampLimits, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	provider, crypto, base := "", "", ""
	var method *wrapperspb.StringValue
	if in != nil {
		provider = in.GetProvider()
		crypto = in.GetCryptoCurrency()
		base = in.GetBaseCurrency()
		method = in.GetPaymentMethod()
	}
	if err := onrampID(provider, CodeOnrampProvider); err != nil {
		return nil, err
	}
	if err := onrampToken(crypto, CodeOnrampCurrency); err != nil {
		return nil, err
	}
	if err := onrampToken(base, CodeOnrampCurrency); err != nil {
		return nil, err
	}
	pay := ""
	if method != nil {
		pay = method.GetValue()
		if err := onrampID(pay, CodeOnrampMethod); err != nil {
			return nil, err
		}
	}
	return mtproto.MakeTLOnrampLimits(&mtproto.OnrampLimits{
		BaseCurrency:    base,
		BaseMinAmount:   "0",
		BaseMaxAmount:   "0",
		CryptoMinAmount: "0",
		CryptoMaxAmount: "0",
		PaymentMethod:   pay,
	}).To_OnrampLimits(), nil
}

// OnrampQuote checks the pair and exactly one amount, then refuses a quote.
func (b *Book) OnrampQuote(self int64, in *mtproto.TLPaymentsGetOnrampQuote) (*mtproto.OnrampQuote, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if err := quoteShape(in); err != nil {
		return nil, err
	}
	return nil, fail(CodeOnrampUnavailable)
}

// CreateOnrampSession checks the address and currencies, then refuses a session.
func (b *Book) CreateOnrampSession(self int64, in *mtproto.TLPaymentsCreateOnrampSession) (*mtproto.OnrampSession, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if err := sessionShape(in); err != nil {
		return nil, err
	}
	return nil, fail(CodeOnrampUnavailable)
}

type invoiceKind int

const (
	invoicePlain invoiceKind = iota
	invoiceGift
)

func invoiceShape(self int64, inv *mtproto.InputInvoice) (invoiceKind, error) {
	if inv == nil {
		return invoicePlain, fail(CodeInvoiceInvalid)
	}
	switch inv.GetPredicateName() {
	case mtproto.Predicate_inputInvoiceMessage:
		if _, _, err := parsePeer(self, inv.GetPeer()); err != nil {
			return invoicePlain, err
		}
		if inv.GetMsgId() <= 0 {
			return invoicePlain, fail(CodeMsgInvalid)
		}
		return invoicePlain, nil
	case mtproto.Predicate_inputInvoiceSlug:
		if err := giftSlug(inv.GetSlug()); err != nil {
			return invoicePlain, err
		}
		return invoicePlain, nil
	case mtproto.Predicate_inputInvoiceChatInviteSubscription:
		if err := giftSlug(inv.GetHash()); err != nil {
			return invoicePlain, err
		}
		return invoicePlain, nil
	case mtproto.Predicate_inputInvoicePremiumGiftCode, mtproto.Predicate_inputInvoiceStars, mtproto.Predicate_inputInvoicePremiumAuthCode:
		if err := purposeShape(self, inv.GetPurpose(), CodePremiumPurpose); err != nil {
			return invoicePlain, err
		}
		if inv.GetPredicateName() == mtproto.Predicate_inputInvoicePremiumGiftCode && inv.GetOption_PREMIUMGIFTCODEOPTION() == nil {
			return invoicePlain, fail(CodeInvoiceInvalid)
		}
		return invoicePlain, nil
	case mtproto.Predicate_inputInvoicePremiumGiftStars:
		if _, err := parseUser(self, inv.GetUserId()); err != nil {
			return invoicePlain, err
		}
		if inv.GetMonths() < 1 || inv.GetMonths() > maxMonths {
			return invoicePlain, fail(CodeMonthInvalid)
		}
		if err := giftMessage(inv.GetMessage()); err != nil {
			return invoicePlain, err
		}
		return invoicePlain, nil
	case mtproto.Predicate_inputInvoiceBusinessBotTransferStars:
		if _, err := parseUser(self, inv.GetBot()); err != nil {
			return invoicePlain, err
		}
		if inv.GetStars() <= 0 {
			return invoicePlain, fail(CodeGiftStarsInvalid)
		}
		return invoicePlain, nil
	case mtproto.Predicate_inputInvoiceStarGift, mtproto.Predicate_inputInvoiceStarGiftAuctionBid:
		if _, _, err := parsePeer(self, inv.GetPeer()); err != nil && inv.GetPredicateName() == mtproto.Predicate_inputInvoiceStarGift {
			return invoiceGift, err
		}
		if inv.GetPredicateName() == mtproto.Predicate_inputInvoiceStarGiftAuctionBid && inv.GetPeer() != nil {
			if _, _, err := parsePeer(self, inv.GetPeer()); err != nil {
				return invoiceGift, err
			}
		}
		if inv.GetGiftId() <= 0 {
			return invoiceGift, fail(CodeStarGiftInvalid)
		}
		if inv.GetPredicateName() == mtproto.Predicate_inputInvoiceStarGiftAuctionBid && inv.GetBidAmount() <= 0 {
			return invoiceGift, fail(CodeStarGiftInvalid)
		}
		if err := giftMessage(inv.GetMessage()); err != nil {
			return invoiceGift, err
		}
		return invoiceGift, nil
	case mtproto.Predicate_inputInvoiceStarGiftUpgrade, mtproto.Predicate_inputInvoiceStarGiftDropOriginalDetails:
		if err := savedRef(self, inv.GetStargift()); err != nil {
			return invoiceGift, err
		}
		return invoiceGift, nil
	case mtproto.Predicate_inputInvoiceStarGiftTransfer:
		if err := savedRef(self, inv.GetStargift()); err != nil {
			return invoiceGift, err
		}
		if _, _, err := parsePeer(self, inv.GetToId_INPUTPEER()); err != nil {
			return invoiceGift, err
		}
		return invoiceGift, nil
	case mtproto.Predicate_inputInvoiceStarGiftResale:
		if err := starSlug(inv.GetSlug()); err != nil {
			return invoiceGift, err
		}
		if _, _, err := parsePeer(self, inv.GetToId_INPUTPEER()); err != nil {
			return invoiceGift, err
		}
		if err := giftMessage(inv.GetMessage()); err != nil {
			return invoiceGift, err
		}
		return invoiceGift, nil
	case mtproto.Predicate_inputInvoiceStarGiftPrepaidUpgrade:
		if _, _, err := parsePeer(self, inv.GetPeer()); err != nil {
			return invoiceGift, err
		}
		if err := giftSlug(inv.GetHash()); err != nil {
			return invoiceGift, err
		}
		return invoiceGift, nil
	default:
		return invoicePlain, fail(CodeInvoiceInvalid)
	}
}

func purposeShape(self int64, p *mtproto.InputStorePaymentPurpose, bad string) error {
	if p == nil {
		return fail(bad)
	}
	switch p.GetPredicateName() {
	case mtproto.Predicate_inputStorePaymentPremiumSubscription:
		return nil
	case mtproto.Predicate_inputStorePaymentGiftPremium:
		if _, err := parseUser(self, p.GetUserId()); err != nil {
			return fail(bad)
		}
		return money(p.GetCurrency(), p.GetAmount(), bad)
	case mtproto.Predicate_inputStorePaymentPremiumGiftCode:
		if err := money(p.GetCurrency(), p.GetAmount(), bad); err != nil {
			return err
		}
		return purposeUsers(self, p.GetUsers_VECTORINPUTUSER(), bad)
	case mtproto.Predicate_inputStorePaymentPremiumGiveaway:
		if _, _, err := parsePeer(self, p.GetBoostPeer()); err != nil {
			return fail(bad)
		}
		if p.GetRandomId() == 0 || p.GetUntilDate() <= 0 {
			return fail(bad)
		}
		if err := purposePeers(self, p.GetAdditionalPeers(), bad); err != nil {
			return err
		}
		return money(p.GetCurrency(), p.GetAmount(), bad)
	case mtproto.Predicate_inputStorePaymentStarsTopup, mtproto.Predicate_inputStorePaymentStars:
		if p.GetStars() <= 0 {
			return fail(bad)
		}
		if p.GetSpendPurposePeer() != nil {
			if _, _, err := parsePeer(self, p.GetSpendPurposePeer()); err != nil {
				return fail(bad)
			}
		}
		return money(p.GetCurrency(), p.GetAmount(), bad)
	case mtproto.Predicate_inputStorePaymentStarsGift:
		if _, err := parseUser(self, p.GetUserId()); err != nil {
			return fail(bad)
		}
		if p.GetStars() <= 0 {
			return fail(bad)
		}
		return money(p.GetCurrency(), p.GetAmount(), bad)
	case mtproto.Predicate_inputStorePaymentStarsGiveaway:
		if p.GetStars() <= 0 || p.GetUsers_INT32() <= 0 || p.GetRandomId() == 0 || p.GetUntilDate() <= 0 {
			return fail(bad)
		}
		if _, _, err := parsePeer(self, p.GetBoostPeer()); err != nil {
			return fail(bad)
		}
		if err := purposePeers(self, p.GetAdditionalPeers(), bad); err != nil {
			return err
		}
		return money(p.GetCurrency(), p.GetAmount(), bad)
	case mtproto.Predicate_inputStorePaymentAuthCode:
		if strings.TrimSpace(p.GetPhoneNumber()) == "" || strings.TrimSpace(p.GetPhoneCodeHash()) == "" || p.GetPremiumDays() <= 0 {
			return fail(bad)
		}
		return money(p.GetCurrency(), p.GetAmount(), bad)
	default:
		return fail(bad)
	}
}

func money(currency string, amount int64, bad string) error {
	if err := currencyCode(currency); err != nil {
		return fail(bad)
	}
	if amount <= 0 {
		return fail(bad)
	}
	return nil
}

func purposeUsers(self int64, users []*mtproto.InputUser, bad string) error {
	for _, user := range users {
		if _, err := parseUser(self, user); err != nil {
			return fail(bad)
		}
	}
	return nil
}

func purposePeers(self int64, peers []*mtproto.InputPeer, bad string) error {
	for _, peer := range peers {
		if _, _, err := parsePeer(self, peer); err != nil {
			return fail(bad)
		}
	}
	return nil
}

func currencyCode(raw string) error {
	if len(raw) < 1 || len(raw) > 8 {
		return fail(CodeCurrencyCodeInvalid)
	}
	for _, r := range raw {
		if !unicode.IsLetter(r) {
			return fail(CodeCurrencyCodeInvalid)
		}
	}
	return nil
}

func giftMessage(msg *mtproto.TextWithEntities) error {
	if msg == nil {
		return nil
	}
	text := msg.GetText()
	if text == "" || utf8.RuneCountInString(text) > maxGiftMessage {
		return fail(CodeStarGiftMessage)
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return fail(CodeStarGiftMessage)
		}
	}
	return nil
}

func themeParams(theme *mtproto.DataJSON) error {
	if theme == nil {
		return nil
	}
	if theme.GetPredicateName() != mtproto.Predicate_dataJSON {
		return fail(CodeThemeParamsInvalid)
	}
	return nil
}

func cleanRequestedInfo(info *mtproto.PaymentRequestedInfo) (*mtproto.PaymentRequestedInfo, error) {
	if info == nil || info.GetPredicateName() != mtproto.Predicate_paymentRequestedInfo {
		return nil, fail(CodePaymentInfoInvalid)
	}
	out := &mtproto.PaymentRequestedInfo{}
	if name := info.GetName(); name != nil {
		value, err := infoText(name.GetValue(), "NAME_INVALID")
		if err != nil {
			return nil, err
		}
		out.Name = wrapperspb.String(value)
	}
	if phone := info.GetPhone(); phone != nil {
		value := strings.TrimSpace(phone.GetValue())
		if !phoneDigits(value) {
			return nil, mtproto.ErrPhoneNumberInvalid
		}
		out.Phone = wrapperspb.String(value)
	}
	if email := info.GetEmail(); email != nil {
		value := strings.TrimSpace(email.GetValue())
		if !emailOK(value) {
			return nil, mtproto.ErrEmailInvalid
		}
		out.Email = wrapperspb.String(value)
	}
	if addr := info.GetShippingAddress(); addr != nil {
		cleaned, err := cleanAddress(addr)
		if err != nil {
			return nil, err
		}
		out.ShippingAddress = cleaned
	}
	if out.Name == nil && out.Phone == nil && out.Email == nil && out.ShippingAddress == nil {
		return nil, fail(CodePaymentInfoInvalid)
	}
	return mtproto.MakeTLPaymentRequestedInfo(out).To_PaymentRequestedInfo(), nil
}

func infoText(raw, code string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || utf8.RuneCountInString(raw) > maxInfoRunes {
		return "", fail(code)
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return "", fail(code)
		}
	}
	return raw, nil
}

func phoneDigits(raw string) bool {
	if len(raw) < 7 || len(raw) > 15 {
		return false
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func emailOK(raw string) bool {
	if len(raw) < 3 || len(raw) > maxInfoRunes || strings.ContainsAny(raw, " \t") {
		return false
	}
	at := strings.IndexByte(raw, '@')
	if at <= 0 || at != strings.LastIndexByte(raw, '@') || at == len(raw)-1 {
		return false
	}
	domain := raw[at+1:]
	dot := strings.IndexByte(domain, '.')
	return dot > 0 && dot < len(domain)-1
}

func cleanAddress(addr *mtproto.PostAddress) (*mtproto.PostAddress, error) {
	if addr.GetPredicateName() != mtproto.Predicate_postAddress {
		return nil, fail(CodeAddressInvalid)
	}
	street, err := infoText(addr.GetStreetLine1(), CodeAddressInvalid)
	if err != nil {
		return nil, err
	}
	line2 := strings.TrimSpace(addr.GetStreetLine2())
	if line2 != "" {
		if line2, err = infoText(line2, CodeAddressInvalid); err != nil {
			return nil, err
		}
	}
	city, err := infoText(addr.GetCity(), CodeAddressInvalid)
	if err != nil {
		return nil, err
	}
	state := strings.TrimSpace(addr.GetState())
	if state != "" {
		if state, err = infoText(state, CodeAddressInvalid); err != nil {
			return nil, err
		}
	}
	country := strings.TrimSpace(addr.GetCountryIso2())
	if len(country) != 2 || !unicode.IsLetter(rune(country[0])) || !unicode.IsLetter(rune(country[1])) {
		return nil, fail(CodeAddressInvalid)
	}
	post, err := infoText(addr.GetPostCode(), CodeAddressInvalid)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLPostAddress(&mtproto.PostAddress{
		StreetLine1: street,
		StreetLine2: line2,
		City:        city,
		State:       state,
		CountryIso2: strings.ToUpper(country),
		PostCode:    post,
	}).To_PostAddress(), nil
}

func credentialsShape(c *mtproto.InputPaymentCredentials) error {
	if c == nil {
		return fail(CodeCredentialsInvalid)
	}
	switch c.GetPredicateName() {
	case mtproto.Predicate_inputPaymentCredentials:
		return dataJSON(c.GetData())
	case mtproto.Predicate_inputPaymentCredentialsApplePay:
		return dataJSON(c.GetPaymentData())
	case mtproto.Predicate_inputPaymentCredentialsGooglePay:
		return dataJSON(c.GetPaymentToken())
	case mtproto.Predicate_inputPaymentCredentialsSaved:
		if strings.TrimSpace(c.GetId()) == "" {
			return fail(CodeCredentialsInvalid)
		}
		if len(c.GetTmpPassword()) == 0 {
			return fail(CodeTmpPasswordInvalid)
		}
		return nil
	default:
		return fail(CodeCredentialsInvalid)
	}
}

func dataJSON(d *mtproto.DataJSON) error {
	if d == nil || d.GetPredicateName() != mtproto.Predicate_dataJSON || strings.TrimSpace(d.GetData()) == "" {
		return fail(CodeCredentialsInvalid)
	}
	return nil
}

func cardDigits(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r == ' ' || r == '-' {
			continue
		}
		if r < '0' || r > '9' {
			return ""
		}
		b.WriteRune(r)
	}
	return b.String()
}

func luhn(digits string) bool {
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	sum := 0
	alt := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

func cardTitle(digits string) string {
	switch {
	case strings.HasPrefix(digits, "4"):
		return "Visa"
	case strings.HasPrefix(digits, "34") || strings.HasPrefix(digits, "37"):
		return "American Express"
	case strings.HasPrefix(digits, "62"):
		return "UnionPay"
	}
	if len(digits) >= 4 {
		n := int(digits[0]-'0')*1000 + int(digits[1]-'0')*100 + int(digits[2]-'0')*10 + int(digits[3]-'0')
		if n >= 2221 && n <= 2720 {
			return "Mastercard"
		}
	}
	if len(digits) >= 2 {
		n := int(digits[0]-'0')*10 + int(digits[1]-'0')
		if n >= 51 && n <= 55 {
			return "Mastercard"
		}
	}
	return "Card"
}

func onrampToken(raw, code string) error {
	if len(raw) < 2 || len(raw) > 16 {
		return fail(code)
	}
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return fail(code)
	}
	return nil
}

func onrampID(raw, code string) error {
	if len(raw) < 1 || len(raw) > 64 {
		return fail(code)
	}
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			continue
		}
		return fail(code)
	}
	return nil
}

func onrampAmount(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 32 || raw == "." {
		return fail(CodeOnrampAmount)
	}
	dot := false
	for i, r := range raw {
		if r == '.' {
			if dot || i == 0 || i == len(raw)-1 {
				return fail(CodeOnrampAmount)
			}
			dot = true
			continue
		}
		if r < '0' || r > '9' {
			return fail(CodeOnrampAmount)
		}
	}
	return nil
}

func quoteShape(in *mtproto.TLPaymentsGetOnrampQuote) error {
	if in == nil {
		return fail(CodeOnrampProvider)
	}
	if err := onrampID(in.GetProvider(), CodeOnrampProvider); err != nil {
		return err
	}
	if err := onrampToken(in.GetCryptoCurrency(), CodeOnrampCurrency); err != nil {
		return err
	}
	if err := onrampToken(in.GetBaseCurrency(), CodeOnrampCurrency); err != nil {
		return err
	}
	base, crypto := in.GetBaseAmount(), in.GetCryptoAmount()
	switch {
	case base != nil && crypto != nil, base == nil && crypto == nil:
		return fail(CodeOnrampAmount)
	case base != nil:
		if err := onrampAmount(base.GetValue()); err != nil {
			return err
		}
	default:
		if err := onrampAmount(crypto.GetValue()); err != nil {
			return err
		}
	}
	if method := in.GetPaymentMethod(); method != nil {
		if err := onrampID(method.GetValue(), CodeOnrampMethod); err != nil {
			return err
		}
	}
	return nil
}

func sessionShape(in *mtproto.TLPaymentsCreateOnrampSession) error {
	if in == nil {
		return fail(CodeOnrampProvider)
	}
	if err := onrampID(in.GetProvider(), CodeOnrampProvider); err != nil {
		return err
	}
	if err := onrampToken(in.GetCryptoCurrency(), CodeOnrampCurrency); err != nil {
		return err
	}
	if err := onrampAddress(in.GetAddress()); err != nil {
		return err
	}
	if method := in.GetPaymentMethod(); method != nil {
		if err := onrampID(method.GetValue(), CodeOnrampMethod); err != nil {
			return err
		}
	}
	if base := in.GetBaseCurrency(); base != nil {
		if err := onrampToken(base.GetValue(), CodeOnrampCurrency); err != nil {
			return err
		}
	}
	if amount := in.GetBaseAmount(); amount != nil {
		if err := onrampAmount(amount.GetValue()); err != nil {
			return err
		}
	}
	if amount := in.GetCryptoAmount(); amount != nil {
		if err := onrampAmount(amount.GetValue()); err != nil {
			return err
		}
	}
	if memo := in.GetMemo(); memo != nil {
		if _, err := infoText(memo.GetValue(), CodeMemoInvalid); err != nil {
			return err
		}
	}
	if theme := in.GetTheme(); theme != nil {
		if _, err := infoText(theme.GetValue(), CodeThemeInvalid); err != nil {
			return err
		}
	}
	if err := returnURL(in.GetSuccessReturnUrl()); err != nil {
		return err
	}
	return returnURL(in.GetFailReturnUrl())
}

func onrampAddress(raw string) error {
	raw = strings.TrimSpace(raw)
	if len(raw) < 8 || len(raw) > 128 {
		return fail(CodeOnrampCurrency)
	}
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ':' || r == '_' || r == '-' {
			continue
		}
		return fail(CodeOnrampCurrency)
	}
	return nil
}

func returnURL(raw *wrapperspb.StringValue) error {
	if raw == nil {
		return nil
	}
	value := strings.TrimSpace(raw.GetValue())
	if len(value) < 8 || len(value) > 512 {
		return fail(CodeURLInvalid)
	}
	if !strings.HasPrefix(value, "https://") && !strings.HasPrefix(value, "http://") {
		return fail(CodeURLInvalid)
	}
	return nil
}
