// Package book is the in-process Stars, gift, and Premium boost book.
//
// Balances stay at zero. Nothing here charges a card, talks to a store, or
// mints Stars. Boost slots and gift-collection titles are remembered for the
// life of the process. There is no gift inventory.
package book

import (
	"sync"
	"time"

	"github.com/teamgram/teamgram-server/pkg/premium"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

const (
	slotCount = 4
	boostLife = 30 * 24 * 3600
	cooldown  = 24 * 3600

	maxIDLen  = 128
	maxLimit  = 100
	pageLimit = 20

	CodePeerInvalid         = "PEER_ID_INVALID"
	CodeUserInvalid         = "USER_ID_INVALID"
	CodeLimitInvalid        = "LIMIT_INVALID"
	CodeOffsetInvalid       = "OFFSET_INVALID"
	CodeSlugInvalid         = "SLUG_INVALID"
	CodeGiftCodeMissing     = "GIFT_CODE_NOT_FOUND"
	CodeMsgInvalid          = "MSG_ID_INVALID"
	CodeGiveawayMissing     = "GIVEAWAY_NOT_FOUND"
	CodeGiveawayIDInvalid   = "GIVEAWAY_ID_INVALID"
	CodePurposeInvalid      = "PURPOSE_INVALID"
	CodePrepaidMissing      = "PREPAID_GIVEAWAY_NOT_FOUND"
	CodeFormInvalid         = "FORM_ID_INVALID"
	CodeInvoiceInvalid      = "INVOICE_INVALID"
	CodeStarsUnavailable    = "STARS_PAYMENT_UNAVAILABLE"
	CodeChargeInvalid       = "CHARGE_ID_INVALID"
	CodeChargeMissing       = "CHARGE_NOT_FOUND"
	CodeSubscriptionInvalid = "SUBSCRIPTION_ID_INVALID"
	CodeSubscriptionMissing = "SUBSCRIPTION_NOT_FOUND"
	CodeTransactionInvalid  = "TRANSACTION_ID_INVALID"
	CodeTransactionsTooMuch = "TRANSACTIONS_TOO_MUCH"
	CodeAmountInvalid       = "AMOUNT_INVALID"
	CodeBalanceTooLow       = "BALANCE_TOO_LOW"
	CodeAdsUnavailable      = "ADS_REVENUE_UNAVAILABLE"
	CodeSlotInvalid         = "SLOT_INVALID"
	CodeBoostsEmpty         = "BOOSTS_EMPTY"
	CodeBoostCooldown       = "BOOST_COOLDOWN"
	CodePremiumRequired     = "PREMIUM_ACCOUNT_REQUIRED"
	CodeAuthKeyUnregistered = "AUTH_KEY_UNREGISTERED"
	CodeStarGiftInvalid     = "STARGIFT_INVALID"
	CodeStarGiftMissing     = "STARGIFT_NOT_FOUND"
	CodeStarGiftSlugInvalid = "STARGIFT_SLUG_INVALID"
	CodeSavedIDEmpty        = "SAVED_ID_EMPTY"
	CodeCollectionInvalid   = "COLLECTION_ID_INVALID"
	CodeTitleInvalid        = "TITLE_INVALID"
	CodeCollectionsTooMuch  = "COLLECTIONS_TOO_MUCH"
	CodeCollectibleInvalid  = "COLLECTIBLE_INVALID"
	CodeCollectibleMissing  = "COLLECTIBLE_NOT_FOUND"
	CodeRandomIDEmpty       = "RANDOM_ID_EMPTY"
)

const (
	maxTitleRunes  = 64
	maxGiftRefs    = 100
	maxCollections = 100
)

// Book stores boost slots, gift-collection titles, and saved checkout info.
// Stars reads do not write it. Nothing here charges a card.
type Book struct {
	mu      sync.Mutex
	users   map[int64]*account
	gifts   *giftState
	payers  map[int64]*payerState
	now     func() int32
	premium func() bool
}

type account struct {
	slots [slotCount]slotState
}

type slotState struct {
	peer          string
	kind          string
	id            int64
	date          int32
	expires       int32
	cooldownUntil int32
}

// New returns an empty book. Tests can replace the clock and premium check.
func New() *Book {
	return &Book{
		users:   map[int64]*account{},
		gifts:   newGiftState(),
		payers:  map[int64]*payerState{},
		now:     func() int32 { return int32(time.Now().Unix()) },
		premium: func() bool { return premium.Load().Everyone },
	}
}

var shared = New()

// Shared is the process book used by the bff handlers.
func Shared() *Book { return shared }

func fail(code string) error {
	if code == "" {
		return nil
	}
	if code == CodeAuthKeyUnregistered {
		return mtproto.ErrAuthKeyUnregistered
	}
	if code == CodePremiumRequired {
		return mtproto.ErrPremiumAccountRequired
	}
	if code == CodePeerInvalid {
		return mtproto.ErrPeerIdInvalid
	}
	if code == CodeUserInvalid {
		return mtproto.ErrUserIdInvalid
	}
	if code == CodeMsgInvalid {
		return mtproto.ErrMsgIdInvalid
	}
	return status.Error(mtproto.ErrBadRequest, code)
}

func (b *Book) clock() int32 {
	if b.now == nil {
		return int32(time.Now().Unix())
	}
	return b.now()
}

func (b *Book) isPremium() bool {
	if b.premium == nil {
		return premium.Load().Everyone
	}
	return b.premium()
}

func zeroAmount(ton bool) *mtproto.StarsAmount {
	if ton {
		return mtproto.MakeTLStarsTonAmount(&mtproto.StarsAmount{}).To_StarsAmount()
	}
	return mtproto.MakeTLStarsAmount(&mtproto.StarsAmount{}).To_StarsAmount()
}
