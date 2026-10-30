package book

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// giftState keeps collection titles and chat gift-notification flags.
// Gift rows are not stored.
type giftState struct {
	notify map[string]bool
	known  map[string]bool
	cols   map[string][]giftCollection
	nextID map[string]int32
}

type giftCollection struct {
	id    int32
	title string
}

func newGiftState() *giftState {
	return &giftState{
		notify: map[string]bool{},
		known:  map[string]bool{},
		cols:   map[string][]giftCollection{},
		nextID: map[string]int32{},
	}
}

func (b *Book) StarGifts(self int64, _ int32) (*mtproto.Payments_StarGifts, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	// The hash of an empty catalog is 0. Clients send 0 when they have no
	// cache, so this method returns the empty list instead of notModified.
	return emptyStarGifts(), nil
}

func (b *Book) CheckCanSendGift(self, giftID int64) (*mtproto.Payments_CheckCanSendGiftResult, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if giftID <= 0 {
		return nil, fail(CodeStarGiftInvalid)
	}
	reason := mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
		Text:     "This gift is not available.",
		Entities: []*mtproto.MessageEntity{},
	}).To_TextWithEntities()
	return mtproto.MakeTLPaymentsCheckCanSendGiftResultFail(&mtproto.Payments_CheckCanSendGiftResult{
		Reason: reason,
	}).To_Payments_CheckCanSendGiftResult(), nil
}

func (b *Book) SavedStarGifts(self int64, in *mtproto.TLPaymentsGetSavedStarGifts) (*mtproto.Payments_SavedStarGifts, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if in == nil {
		return nil, fail(CodePeerInvalid)
	}
	ref, _, err := parsePeer(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if _, err := pageLimitOf(in.GetLimit()); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.GetOffset()) != "" {
		return nil, fail(CodeOffsetInvalid)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if in.GetCollectionId() != nil {
		if _, ok := b.findCollection(ref.key(), in.GetCollectionId().GetValue()); !ok {
			return nil, fail(CodeCollectionInvalid)
		}
	}
	return b.emptySaved(ref.key()), nil
}

func (b *Book) SavedStarGift(self int64, gifts []*mtproto.InputSavedStarGift) (*mtproto.Payments_SavedStarGifts, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if len(gifts) == 0 {
		return nil, fail(CodeStarGiftInvalid)
	}
	if err := savedRefs(self, gifts); err != nil {
		return nil, err
	}
	return emptySavedPage(nil), nil
}

func (b *Book) CraftStarGifts(self, giftID int64, offset string, limit int32) (*mtproto.Payments_SavedStarGifts, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if giftID <= 0 {
		return nil, fail(CodeStarGiftInvalid)
	}
	if _, err := pageLimitOf(limit); err != nil {
		return nil, err
	}
	if strings.TrimSpace(offset) != "" {
		return nil, fail(CodeOffsetInvalid)
	}
	return emptySavedPage(nil), nil
}

func (b *Book) ResaleStarGifts(self int64, in *mtproto.TLPaymentsGetResaleStarGifts) (*mtproto.Payments_ResaleStarGifts, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if in == nil || in.GetGiftId() <= 0 {
		return nil, fail(CodeStarGiftInvalid)
	}
	if _, err := pageLimitOf(in.GetLimit()); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.GetOffset()) != "" {
		return nil, fail(CodeOffsetInvalid)
	}
	if err := attributeIDs(in.GetAttributes()); err != nil {
		return nil, err
	}
	return mtproto.MakeTLPaymentsResaleStarGifts(&mtproto.Payments_ResaleStarGifts{
		Gifts: []*mtproto.StarGift{},
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Payments_ResaleStarGifts(), nil
}

func (b *Book) UpgradePreview(self, giftID int64) (*mtproto.Payments_StarGiftUpgradePreview, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if giftID <= 0 {
		return nil, fail(CodeStarGiftInvalid)
	}
	return mtproto.MakeTLPaymentsStarGiftUpgradePreview(&mtproto.Payments_StarGiftUpgradePreview{
		SampleAttributes: []*mtproto.StarGiftAttribute{},
		Prices:           []*mtproto.StarGiftUpgradePrice{},
		NextPrices:       []*mtproto.StarGiftUpgradePrice{},
	}).To_Payments_StarGiftUpgradePreview(), nil
}

func (b *Book) UpgradeAttributes(self, giftID int64) (*mtproto.Payments_StarGiftUpgradeAttributes, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if giftID <= 0 {
		return nil, fail(CodeStarGiftInvalid)
	}
	return mtproto.MakeTLPaymentsStarGiftUpgradeAttributes(&mtproto.Payments_StarGiftUpgradeAttributes{
		Attributes: []*mtproto.StarGiftAttribute{},
	}).To_Payments_StarGiftUpgradeAttributes(), nil
}

func (b *Book) ActiveAuctions(self, _ int64) (*mtproto.Payments_StarGiftActiveAuctions, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	// No auctions are listed. The hash of that empty list is 0, which is
	// also what a client sends when it has no cache.
	return mtproto.MakeTLPaymentsStarGiftActiveAuctions(&mtproto.Payments_StarGiftActiveAuctions{
		Auctions: []*mtproto.StarGiftActiveAuctionState{},
		Users:    []*mtproto.User{},
		Chats:    []*mtproto.Chat{},
	}).To_Payments_StarGiftActiveAuctions(), nil
}

func (b *Book) AuctionAcquired(self, giftID int64) (*mtproto.Payments_StarGiftAuctionAcquiredGifts, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if giftID <= 0 {
		return nil, fail(CodeStarGiftInvalid)
	}
	return mtproto.MakeTLPaymentsStarGiftAuctionAcquiredGifts(&mtproto.Payments_StarGiftAuctionAcquiredGifts{
		Gifts: []*mtproto.StarGiftAuctionAcquiredGift{},
		Users: []*mtproto.User{},
		Chats: []*mtproto.Chat{},
	}).To_Payments_StarGiftAuctionAcquiredGifts(), nil
}

func (b *Book) AuctionState(self int64, auction *mtproto.InputStarGiftAuction) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if err := auctionRef(auction); err != nil {
		return err
	}
	return fail(CodeStarGiftMissing)
}

func (b *Book) UniqueStarGift(self int64, slug string) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if err := starSlug(slug); err != nil {
		return err
	}
	return fail(CodeStarGiftMissing)
}

func (b *Book) UniqueStarGiftValue(self int64, slug string) error {
	return b.UniqueStarGift(self, slug)
}

func (b *Book) UserStarGifts(self int64, user *mtproto.InputUser, offset string, limit int32) (*mtproto.Payments_UserStarGifts, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if _, err := parseUser(self, user); err != nil {
		return nil, err
	}
	if _, err := pageLimitOf(limit); err != nil {
		return nil, err
	}
	if strings.TrimSpace(offset) != "" {
		return nil, fail(CodeOffsetInvalid)
	}
	return mtproto.MakeTLPaymentsUserStarGifts(&mtproto.Payments_UserStarGifts{
		Gifts: []*mtproto.UserStarGift{},
		Users: []*mtproto.User{},
	}).To_Payments_UserStarGifts(), nil
}

func (b *Book) UserStarGift(self int64, msgIDs []int32) (*mtproto.Payments_UserStarGifts, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if len(msgIDs) == 0 || len(msgIDs) > maxGiftRefs {
		return nil, fail(CodeStarGiftInvalid)
	}
	for _, id := range msgIDs {
		if id <= 0 {
			return nil, fail(CodeMsgInvalid)
		}
	}
	return mtproto.MakeTLPaymentsUserStarGifts(&mtproto.Payments_UserStarGifts{
		Gifts: []*mtproto.UserStarGift{},
		Users: []*mtproto.User{},
	}).To_Payments_UserStarGifts(), nil
}

func (b *Book) SaveStarGift(self int64, gift *mtproto.InputSavedStarGift) error {
	if err := b.requireMissingGift(self, gift); err != nil {
		return err
	}
	return fail(CodeStarGiftMissing)
}

func (b *Book) ConvertStarGift(self int64, gift *mtproto.InputSavedStarGift) error {
	return b.SaveStarGift(self, gift)
}

func (b *Book) UpgradeStarGift(self int64, gift *mtproto.InputSavedStarGift) error {
	return b.SaveStarGift(self, gift)
}

func (b *Book) TransferStarGift(self int64, gift *mtproto.InputSavedStarGift, to *mtproto.InputPeer, toUser *mtproto.InputUser) error {
	if err := b.requireMissingGift(self, gift); err != nil {
		return err
	}
	if to != nil {
		if _, _, err := parsePeer(self, to); err != nil {
			return err
		}
	} else if _, err := parseUser(self, toUser); err != nil {
		return err
	}
	return fail(CodeStarGiftMissing)
}

func (b *Book) UpdateStarGiftPrice(self int64, gift *mtproto.InputSavedStarGift, amount *mtproto.StarsAmount, legacyStars int64) error {
	if err := b.requireMissingGift(self, gift); err != nil {
		return err
	}
	if err := priceOf(amount, legacyStars, true); err != nil {
		return err
	}
	return fail(CodeStarGiftMissing)
}

func (b *Book) WithdrawalURL(self int64, gift *mtproto.InputSavedStarGift, tmp []byte, password *mtproto.InputCheckPasswordSRP) error {
	if err := b.requireMissingGift(self, gift); err != nil {
		return err
	}
	if len(tmp) == 0 && !passwordPresent(password) {
		return mtproto.ErrPasswordHashInvalid
	}
	return fail(CodeStarGiftMissing)
}

func (b *Book) ToggleChatStarGiftNotifications(self int64, peer *mtproto.InputPeer, enabled bool) (*mtproto.Bool, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	ref, _, err := parsePeer(self, peer)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.gifts.notify[ref.key()] = enabled
	b.gifts.known[ref.key()] = true
	b.mu.Unlock()
	return mtproto.ToBool(true), nil
}

func (b *Book) ToggleStarGiftsPinned(self int64, peer *mtproto.InputPeer, gifts []*mtproto.InputSavedStarGift) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if _, _, err := parsePeer(self, peer); err != nil {
		return err
	}
	if len(gifts) == 0 {
		return nil
	}
	if err := savedRefs(self, gifts); err != nil {
		return err
	}
	return fail(CodeStarGiftMissing)
}

func (b *Book) CraftStarGift(self int64, gifts []*mtproto.InputSavedStarGift) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if len(gifts) == 0 {
		return fail(CodeStarGiftInvalid)
	}
	if err := savedRefs(self, gifts); err != nil {
		return err
	}
	return fail(CodeStarGiftMissing)
}

func (b *Book) ResolveStarGiftOffer(self int64, msgID int32) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if msgID <= 0 {
		return fail(CodeMsgInvalid)
	}
	return fail(CodeStarGiftMissing)
}

func (b *Book) SendStarGiftOffer(self int64, in *mtproto.TLPaymentsSendStarGiftOffer) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if in == nil {
		return fail(CodePeerInvalid)
	}
	if _, _, err := parsePeer(self, in.GetPeer()); err != nil {
		return err
	}
	if err := starSlug(in.GetSlug()); err != nil {
		return err
	}
	if err := priceOf(in.GetPrice(), 0, false); err != nil {
		return err
	}
	if in.GetDuration() <= 0 {
		return fail(CodeStarGiftInvalid)
	}
	if in.GetRandomId() == 0 {
		return fail(CodeRandomIDEmpty)
	}
	if in.GetAllowPaidStars() != nil && in.GetAllowPaidStars().GetValue() < 0 {
		return fail(CodeAmountInvalid)
	}
	return fail(CodeStarGiftMissing)
}

func (b *Book) GetStarGiftCollections(self int64, peer *mtproto.InputPeer, hash int64) (*mtproto.Payments_StarGiftCollections, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	ref, _, err := parsePeer(self, peer)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	rows := b.gifts.cols[ref.key()]
	sum := colsHash(rows)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLPaymentsStarGiftCollectionsNotModified(&mtproto.Payments_StarGiftCollections{}).To_Payments_StarGiftCollections(), nil
	}
	return collectionsOf(rows), nil
}

func (b *Book) CreateStarGiftCollection(self int64, peer *mtproto.InputPeer, title string, gifts []*mtproto.InputSavedStarGift) (*mtproto.StarGiftCollection, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	ref, _, err := parsePeer(self, peer)
	if err != nil {
		return nil, err
	}
	title, err = cleanTitle(title)
	if err != nil {
		return nil, err
	}
	if len(gifts) > 0 {
		if err := savedRefs(self, gifts); err != nil {
			return nil, err
		}
		return nil, fail(CodeStarGiftMissing)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := ref.key()
	if len(b.gifts.cols[key]) >= maxCollections {
		return nil, fail(CodeCollectionsTooMuch)
	}
	b.gifts.nextID[key]++
	row := giftCollection{id: b.gifts.nextID[key], title: title}
	b.gifts.cols[key] = append(b.gifts.cols[key], row)
	return presentCollection(row), nil
}

func (b *Book) UpdateStarGiftCollection(self int64, in *mtproto.TLPaymentsUpdateStarGiftCollection) (*mtproto.StarGiftCollection, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if in == nil {
		return nil, fail(CodePeerInvalid)
	}
	ref, _, err := parsePeer(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if len(in.GetDeleteStargift()) > 0 || len(in.GetAddStargift()) > 0 || len(in.GetOrder()) > 0 {
		if err := savedRefs(self, in.GetDeleteStargift()); err != nil {
			return nil, err
		}
		if err := savedRefs(self, in.GetAddStargift()); err != nil {
			return nil, err
		}
		if err := savedRefs(self, in.GetOrder()); err != nil {
			return nil, err
		}
		return nil, fail(CodeStarGiftMissing)
	}
	var title string
	rename := in.GetTitle() != nil
	if rename {
		title, err = cleanTitle(in.GetTitle().GetValue())
		if err != nil {
			return nil, err
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := ref.key()
	idx, ok := b.findCollection(key, in.GetCollectionId())
	if !ok {
		return nil, fail(CodeCollectionInvalid)
	}
	if rename {
		b.gifts.cols[key][idx].title = title
	}
	return presentCollection(b.gifts.cols[key][idx]), nil
}

func (b *Book) ReorderStarGiftCollections(self int64, peer *mtproto.InputPeer, order []int32) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	ref, _, err := parsePeer(self, peer)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := ref.key()
	rows := b.gifts.cols[key]
	if len(order) != len(rows) {
		return fail(CodeCollectionInvalid)
	}
	byID := map[int32]giftCollection{}
	for _, row := range rows {
		byID[row.id] = row
	}
	next := make([]giftCollection, 0, len(order))
	seen := map[int32]bool{}
	for _, id := range order {
		row, ok := byID[id]
		if !ok || seen[id] {
			return fail(CodeCollectionInvalid)
		}
		seen[id] = true
		next = append(next, row)
	}
	b.gifts.cols[key] = next
	return nil
}

func (b *Book) DeleteStarGiftCollection(self int64, peer *mtproto.InputPeer, id int32) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	ref, _, err := parsePeer(self, peer)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := ref.key()
	idx, ok := b.findCollection(key, id)
	if !ok {
		return fail(CodeCollectionInvalid)
	}
	rows := b.gifts.cols[key]
	b.gifts.cols[key] = append(rows[:idx], rows[idx+1:]...)
	return nil
}

func (b *Book) DeleteStarGift(self int64, gift *mtproto.InputSavedStarGift) error {
	return b.SaveStarGift(self, gift)
}

func (b *Book) WithdrawStarGift(self int64, gift *mtproto.InputSavedStarGift, tmp []byte) error {
	return b.WithdrawalURL(self, gift, tmp, nil)
}

func (b *Book) DepositStarGift(self int64, peer *mtproto.InputPeer, slug string) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if _, _, err := parsePeer(self, peer); err != nil {
		return err
	}
	if err := starSlug(slug); err != nil {
		return err
	}
	return fail(CodeStarGiftMissing)
}

func (b *Book) DisplayStarGift(self int64, peer *mtproto.InputPeer, hasPeer bool, slug string) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if hasPeer {
		if _, _, err := parsePeer(self, peer); err != nil {
			return err
		}
	}
	if err := starSlug(slug); err != nil {
		return err
	}
	return fail(CodeStarGiftMissing)
}

func (b *Book) CollectibleInfo(self int64, in *mtproto.InputCollectible) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if in == nil {
		return fail(CodeCollectibleInvalid)
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputCollectibleUsername:
		if !collectibleUsername(in.GetUsername()) {
			return fail(CodeCollectibleInvalid)
		}
	case mtproto.Predicate_inputCollectiblePhone:
		if !collectiblePhone(in.GetPhone()) {
			return fail(CodeCollectibleInvalid)
		}
	default:
		return fail(CodeCollectibleInvalid)
	}
	return fail(CodeCollectibleMissing)
}

func (b *Book) requireMissingGift(self int64, gift *mtproto.InputSavedStarGift) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	return savedRef(self, gift)
}

func (b *Book) findCollection(key string, id int32) (int, bool) {
	if id == 0 {
		return 0, false
	}
	for i, row := range b.gifts.cols[key] {
		if row.id == id {
			return i, true
		}
	}
	return 0, false
}

func (b *Book) emptySaved(key string) *mtproto.Payments_SavedStarGifts {
	var notify *mtproto.Bool
	if b.gifts.known[key] {
		notify = mtproto.ToBool(b.gifts.notify[key])
	}
	return emptySavedPage(notify)
}

func emptySavedPage(notify *mtproto.Bool) *mtproto.Payments_SavedStarGifts {
	return mtproto.MakeTLPaymentsSavedStarGifts(&mtproto.Payments_SavedStarGifts{
		ChatNotificationsEnabled: notify,
		Gifts:                    []*mtproto.SavedStarGift{},
		Chats:                    []*mtproto.Chat{},
		Users:                    []*mtproto.User{},
	}).To_Payments_SavedStarGifts()
}

func emptyStarGifts() *mtproto.Payments_StarGifts {
	return mtproto.MakeTLPaymentsStarGifts(&mtproto.Payments_StarGifts{
		Gifts: []*mtproto.StarGift{},
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Payments_StarGifts()
}

func collectionsOf(rows []giftCollection) *mtproto.Payments_StarGiftCollections {
	out := make([]*mtproto.StarGiftCollection, 0, len(rows))
	for _, row := range rows {
		out = append(out, presentCollection(row))
	}
	return mtproto.MakeTLPaymentsStarGiftCollections(&mtproto.Payments_StarGiftCollections{
		Collections: out,
	}).To_Payments_StarGiftCollections()
}

func presentCollection(row giftCollection) *mtproto.StarGiftCollection {
	return mtproto.MakeTLStarGiftCollection(&mtproto.StarGiftCollection{
		CollectionId: row.id,
		Title:        row.title,
		Hash:         titleHash(row.title),
	}).To_StarGiftCollection()
}

func savedRefs(self int64, gifts []*mtproto.InputSavedStarGift) error {
	if len(gifts) > maxGiftRefs {
		return fail(CodeStarGiftInvalid)
	}
	for _, gift := range gifts {
		if err := savedRef(self, gift); err != nil {
			return err
		}
	}
	return nil
}

func savedRef(self int64, gift *mtproto.InputSavedStarGift) error {
	if gift == nil {
		return fail(CodeStarGiftInvalid)
	}
	switch gift.GetPredicateName() {
	case mtproto.Predicate_inputSavedStarGiftUser:
		if gift.GetMsgId() <= 0 {
			return fail(CodeMsgInvalid)
		}
	case mtproto.Predicate_inputSavedStarGiftChat:
		if _, _, err := parsePeer(self, gift.GetPeer()); err != nil {
			return err
		}
		if gift.GetSavedId() == 0 {
			return fail(CodeSavedIDEmpty)
		}
	case mtproto.Predicate_inputSavedStarGiftSlug:
		if err := starSlug(gift.GetSlug()); err != nil {
			return err
		}
	default:
		return fail(CodeStarGiftInvalid)
	}
	return nil
}

func auctionRef(in *mtproto.InputStarGiftAuction) error {
	if in == nil {
		return fail(CodeStarGiftInvalid)
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputStarGiftAuction:
		if in.GetGiftId() <= 0 {
			return fail(CodeStarGiftInvalid)
		}
	case mtproto.Predicate_inputStarGiftAuctionSlug:
		if err := starSlug(in.GetSlug()); err != nil {
			return err
		}
	default:
		return fail(CodeStarGiftInvalid)
	}
	return nil
}

func attributeIDs(rows []*mtproto.StarGiftAttributeId) error {
	if len(rows) > maxGiftRefs {
		return fail(CodeStarGiftInvalid)
	}
	for _, row := range rows {
		if row == nil {
			return fail(CodeStarGiftInvalid)
		}
		switch row.GetPredicateName() {
		case mtproto.Predicate_starGiftAttributeIdModel, mtproto.Predicate_starGiftAttributeIdPattern:
			if row.GetDocumentId() == 0 {
				return fail(CodeStarGiftInvalid)
			}
		case mtproto.Predicate_starGiftAttributeIdBackdrop:
		default:
			return fail(CodeStarGiftInvalid)
		}
	}
	return nil
}

func priceOf(amount *mtproto.StarsAmount, legacy int64, allowLegacy bool) error {
	if amount == nil {
		if allowLegacy && legacy > 0 {
			return nil
		}
		return fail(CodeAmountInvalid)
	}
	switch amount.GetPredicateName() {
	case mtproto.Predicate_starsAmount:
		if amount.GetAmount() <= 0 || amount.GetNanos() < 0 || amount.GetNanos() >= 1_000_000_000 {
			return fail(CodeAmountInvalid)
		}
	case mtproto.Predicate_starsTonAmount:
		if amount.GetAmount() <= 0 {
			return fail(CodeAmountInvalid)
		}
	default:
		return fail(CodeAmountInvalid)
	}
	return nil
}

func starSlug(slug string) error {
	if err := giftSlug(slug); err != nil {
		return fail(CodeStarGiftSlugInvalid)
	}
	return nil
}

func cleanTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > maxTitleRunes {
		return "", fail(CodeTitleInvalid)
	}
	for _, r := range title {
		if unicode.IsControl(r) {
			return "", fail(CodeTitleInvalid)
		}
	}
	return title, nil
}

func collectibleUsername(name string) bool {
	name = strings.TrimSpace(name)
	if len(name) < 4 || len(name) > 32 {
		return false
	}
	for i, r := range name {
		if i == 0 && !unicode.IsLetter(r) {
			return false
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}
		return false
	}
	return true
}

func collectiblePhone(phone string) bool {
	phone = strings.TrimSpace(phone)
	if strings.HasPrefix(phone, "+") {
		phone = phone[1:]
	}
	if len(phone) < 7 || len(phone) > 15 {
		return false
	}
	for _, r := range phone {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// colsHash is the list cache key. An empty list hashes to 0, which clients
// send when they have no cache, so an empty list is never notModified.
func colsHash(rows []giftCollection) int64 {
	var h int64
	for _, row := range rows {
		h = mixHash(h, int64(row.id))
		h = mixHash(h, titleHash(row.title))
	}
	return h
}

func titleHash(title string) int64 {
	var h int64
	for _, r := range title {
		h = mixHash(h, int64(r))
	}
	if h == 0 {
		return 1
	}
	return h
}

func mixHash(hash, id int64) int64 {
	hash ^= id >> 21
	hash ^= id << 35
	hash ^= id >> 4
	return hash + id
}

// CollectionID is exported for tests that build a collection_id flag.
func CollectionID(id int32) *wrapperspb.Int32Value {
	return wrapperspb.Int32(id)
}
