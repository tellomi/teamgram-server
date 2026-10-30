package core

import (
	"strings"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/grpc/status"
)

// Assign does not talk to Apple or Google. A non-empty receipt with a
// well-formed purpose is RECEIPT_INVALID. An empty receipt is RECEIPT_EMPTY.
// A purpose that is missing or the wrong shape is INPUT_PURPOSE_INVALID.

func assignAppStore(receipt []byte, purpose *mtproto.InputStorePaymentPurpose) error {
	if len(receipt) == 0 {
		return status.Error(mtproto.ErrBadRequest, "RECEIPT_EMPTY")
	}
	if err := storePurpose(purpose); err != nil {
		return err
	}
	return status.Error(mtproto.ErrBadRequest, "RECEIPT_INVALID")
}

func assignPlayMarket(receipt *mtproto.DataJSON, purpose *mtproto.InputStorePaymentPurpose) error {
	if receipt == nil || receipt.GetPredicateName() != mtproto.Predicate_dataJSON || strings.TrimSpace(receipt.GetData()) == "" {
		return status.Error(mtproto.ErrBadRequest, "RECEIPT_EMPTY")
	}
	if err := storePurpose(purpose); err != nil {
		return err
	}
	return status.Error(mtproto.ErrBadRequest, "RECEIPT_INVALID")
}

func storePurpose(p *mtproto.InputStorePaymentPurpose) error {
	if p == nil {
		return status.Error(mtproto.ErrBadRequest, "INPUT_PURPOSE_INVALID")
	}
	switch p.GetPredicateName() {
	case mtproto.Predicate_inputStorePaymentPremiumSubscription:
		return nil
	case mtproto.Predicate_inputStorePaymentGiftPremium, mtproto.Predicate_inputStorePaymentStarsGift:
		if p.GetUserId() == nil || p.GetUserId().GetPredicateName() == "" {
			return status.Error(mtproto.ErrBadRequest, "INPUT_PURPOSE_INVALID")
		}
		if p.GetPredicateName() == mtproto.Predicate_inputStorePaymentStarsGift && p.GetStars() <= 0 {
			return status.Error(mtproto.ErrBadRequest, "INPUT_PURPOSE_INVALID")
		}
		return storeMoney(p)
	case mtproto.Predicate_inputStorePaymentPremiumGiftCode:
		return storeMoney(p)
	case mtproto.Predicate_inputStorePaymentPremiumGiveaway, mtproto.Predicate_inputStorePaymentStarsGiveaway:
		if p.GetBoostPeer() == nil || p.GetRandomId() == 0 || p.GetUntilDate() <= 0 {
			return status.Error(mtproto.ErrBadRequest, "INPUT_PURPOSE_INVALID")
		}
		if p.GetPredicateName() == mtproto.Predicate_inputStorePaymentStarsGiveaway && (p.GetStars() <= 0 || p.GetUsers_INT32() <= 0) {
			return status.Error(mtproto.ErrBadRequest, "INPUT_PURPOSE_INVALID")
		}
		return storeMoney(p)
	case mtproto.Predicate_inputStorePaymentStarsTopup, mtproto.Predicate_inputStorePaymentStars:
		if p.GetStars() <= 0 {
			return status.Error(mtproto.ErrBadRequest, "INPUT_PURPOSE_INVALID")
		}
		return storeMoney(p)
	case mtproto.Predicate_inputStorePaymentAuthCode:
		if strings.TrimSpace(p.GetPhoneNumber()) == "" || strings.TrimSpace(p.GetPhoneCodeHash()) == "" || p.GetPremiumDays() <= 0 {
			return status.Error(mtproto.ErrBadRequest, "INPUT_PURPOSE_INVALID")
		}
		return storeMoney(p)
	default:
		return status.Error(mtproto.ErrBadRequest, "INPUT_PURPOSE_INVALID")
	}
}

func storeMoney(p *mtproto.InputStorePaymentPurpose) error {
	raw := p.GetCurrency()
	if len(raw) < 1 || len(raw) > 8 {
		return status.Error(mtproto.ErrBadRequest, "INPUT_PURPOSE_INVALID")
	}
	for _, r := range raw {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return status.Error(mtproto.ErrBadRequest, "INPUT_PURPOSE_INVALID")
		}
	}
	if p.GetAmount() <= 0 {
		return status.Error(mtproto.ErrBadRequest, "INPUT_PURPOSE_INVALID")
	}
	return nil
}
