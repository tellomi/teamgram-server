package book

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func userGift(msgID int32) *mtproto.InputSavedStarGift {
	return mtproto.MakeTLInputSavedStarGiftUser(&mtproto.InputSavedStarGift{MsgId: msgID}).To_InputSavedStarGift()
}

func TestStarGiftCatalogAndChecks(t *testing.T) {
	b := New()
	if _, err := b.StarGifts(0, 0); codeOf(err) != CodeAuthKeyUnregistered {
		t.Fatal(err)
	}
	got, err := b.StarGifts(4, 0)
	if err != nil || got.GetPredicateName() != mtproto.Predicate_payments_starGifts || got.GetHash() != 0 || len(got.GetGifts()) != 0 {
		t.Fatalf("%s hash=%d gifts=%d err=%v", got.GetPredicateName(), got.GetHash(), len(got.GetGifts()), err)
	}
	again, err := b.StarGifts(4, 99)
	if err != nil || again.GetPredicateName() != mtproto.Predicate_payments_starGifts {
		t.Fatal(again, err)
	}
	if _, err := b.CheckCanSendGift(4, 0); codeOf(err) != CodeStarGiftInvalid {
		t.Fatal(err)
	}
	failRes, err := b.CheckCanSendGift(4, 7)
	if err != nil || failRes.GetPredicateName() != mtproto.Predicate_payments_checkCanSendGiftResultFail {
		t.Fatal(failRes, err)
	}
	if !strings.Contains(failRes.GetReason().GetText(), "not available") {
		t.Fatal(failRes.GetReason())
	}
}

func TestSavedStarGiftsAndNotifications(t *testing.T) {
	b := New()
	peer := selfPeer()
	if _, err := b.SavedStarGifts(4, &mtproto.TLPaymentsGetSavedStarGifts{Peer: peer, Offset: "1", Limit: 10}); codeOf(err) != CodeOffsetInvalid {
		t.Fatal(err)
	}
	if _, err := b.SavedStarGifts(4, &mtproto.TLPaymentsGetSavedStarGifts{Peer: peer, Limit: 10, CollectionId: CollectionID(1)}); codeOf(err) != CodeCollectionInvalid {
		t.Fatal(err)
	}
	page, err := b.SavedStarGifts(4, &mtproto.TLPaymentsGetSavedStarGifts{Peer: peer, Limit: 10})
	if err != nil || page.GetCount() != 0 || page.GetChatNotificationsEnabled() != nil {
		t.Fatal(page, err)
	}
	if _, err := b.ToggleChatStarGiftNotifications(4, peer, false); err != nil {
		t.Fatal(err)
	}
	page, err = b.SavedStarGifts(4, &mtproto.TLPaymentsGetSavedStarGifts{Peer: peer})
	if err != nil || page.GetChatNotificationsEnabled().GetPredicateName() != mtproto.Predicate_boolFalse {
		t.Fatal(page.GetChatNotificationsEnabled(), err)
	}
	if _, err := b.SavedStarGift(4, nil); codeOf(err) != CodeStarGiftInvalid {
		t.Fatal(err)
	}
	if _, err := b.SavedStarGift(4, []*mtproto.InputSavedStarGift{userGift(0)}); codeOf(err) != CodeMsgInvalid {
		t.Fatal(err)
	}
	empty, err := b.SavedStarGift(4, []*mtproto.InputSavedStarGift{userGift(3)})
	if err != nil || len(empty.GetGifts()) != 0 {
		t.Fatal(empty, err)
	}
	chat := mtproto.MakeTLInputSavedStarGiftChat(&mtproto.InputSavedStarGift{Peer: peer, SavedId: 0}).To_InputSavedStarGift()
	if _, err := b.SavedStarGift(4, []*mtproto.InputSavedStarGift{chat}); codeOf(err) != CodeSavedIDEmpty {
		t.Fatal(err)
	}
}

func TestCollectionsRoundTrip(t *testing.T) {
	b := New()
	peer := selfPeer()
	if _, err := b.CreateStarGiftCollection(4, peer, "  ", nil); codeOf(err) != CodeTitleInvalid {
		t.Fatal(err)
	}
	if _, err := b.CreateStarGiftCollection(4, peer, "Pins", []*mtproto.InputSavedStarGift{userGift(2)}); codeOf(err) != CodeStarGiftMissing {
		t.Fatal(err)
	}
	made, err := b.CreateStarGiftCollection(4, peer, " Pins ", nil)
	if err != nil || made.GetCollectionId() == 0 || made.GetTitle() != "Pins" || made.GetGiftsCount() != 0 {
		t.Fatal(made, err)
	}
	second, err := b.CreateStarGiftCollection(4, peer, "Other", nil)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := b.GetStarGiftCollections(4, peer, 0)
	if err != nil || len(listed.GetCollections()) != 2 {
		t.Fatal(listed, err)
	}
	sum := colsHash([]giftCollection{{id: made.GetCollectionId(), title: "Pins"}, {id: second.GetCollectionId(), title: "Other"}})
	same, err := b.GetStarGiftCollections(4, peer, sum)
	if err != nil || same.GetPredicateName() != mtproto.Predicate_payments_starGiftCollectionsNotModified {
		t.Fatal(same, err)
	}
	renamed, err := b.UpdateStarGiftCollection(4, &mtproto.TLPaymentsUpdateStarGiftCollection{
		Peer:         peer,
		CollectionId: made.GetCollectionId(),
		Title:        wrapperspb.String("Kept"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.GetTitle() != "Kept" {
		t.Fatal(renamed.GetTitle())
	}
	if err := b.ReorderStarGiftCollections(4, peer, []int32{second.GetCollectionId(), made.GetCollectionId()}); err != nil {
		t.Fatal(err)
	}
	if err := b.ReorderStarGiftCollections(4, peer, []int32{made.GetCollectionId()}); codeOf(err) != CodeCollectionInvalid {
		t.Fatal(err)
	}
	if err := b.DeleteStarGiftCollection(4, peer, made.GetCollectionId()); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteStarGiftCollection(4, peer, made.GetCollectionId()); codeOf(err) != CodeCollectionInvalid {
		t.Fatal(err)
	}
	page, err := b.SavedStarGifts(4, &mtproto.TLPaymentsGetSavedStarGifts{Peer: peer, CollectionId: CollectionID(second.GetCollectionId())})
	if err != nil || page.GetCount() != 0 {
		t.Fatal(page, err)
	}
}

func TestGiftMutationsStay400(t *testing.T) {
	b := New()
	gift := userGift(8)
	peer := selfPeer()
	if codeOf(b.SaveStarGift(4, gift)) != CodeStarGiftMissing {
		t.Fatal("save")
	}
	if codeOf(b.ConvertStarGift(4, gift)) != CodeStarGiftMissing {
		t.Fatal("convert")
	}
	if codeOf(b.UpgradeStarGift(4, gift)) != CodeStarGiftMissing {
		t.Fatal("upgrade")
	}
	if codeOf(b.TransferStarGift(4, gift, peer, nil)) != CodeStarGiftMissing {
		t.Fatal("transfer")
	}
	amount := mtproto.MakeTLStarsAmount(&mtproto.StarsAmount{Amount: 5}).To_StarsAmount()
	if codeOf(b.UpdateStarGiftPrice(4, gift, amount, 0)) != CodeStarGiftMissing {
		t.Fatal("price")
	}
	if codeOf(b.UpdateStarGiftPrice(4, gift, nil, 0)) != CodeAmountInvalid {
		t.Fatal("amount")
	}
	if codeOf(b.WithdrawalURL(4, gift, nil, nil)) != "PASSWORD_HASH_INVALID" {
		t.Fatal("password")
	}
	if codeOf(b.WithdrawalURL(4, gift, []byte{1}, nil)) != CodeStarGiftMissing {
		t.Fatal("withdraw")
	}
	if codeOf(b.ToggleStarGiftsPinned(4, peer, nil)) != "" {
		t.Fatal("pin empty")
	}
	if codeOf(b.ToggleStarGiftsPinned(4, peer, []*mtproto.InputSavedStarGift{gift})) != CodeStarGiftMissing {
		t.Fatal("pin")
	}
	if codeOf(b.CraftStarGift(4, nil)) != CodeStarGiftInvalid {
		t.Fatal("craft empty")
	}
	if codeOf(b.CraftStarGift(4, []*mtproto.InputSavedStarGift{gift})) != CodeStarGiftMissing {
		t.Fatal("craft")
	}
	if codeOf(b.ResolveStarGiftOffer(4, 0)) != CodeMsgInvalid {
		t.Fatal("offer id")
	}
	if codeOf(b.ResolveStarGiftOffer(4, 3)) != CodeStarGiftMissing {
		t.Fatal("offer")
	}
	if codeOf(b.SendStarGiftOffer(4, &mtproto.TLPaymentsSendStarGiftOffer{
		Peer: peer, Slug: "Bad slug", Price: amount, Duration: 60, RandomId: 1,
	})) != CodeStarGiftSlugInvalid {
		t.Fatal("slug")
	}
	if codeOf(b.SendStarGiftOffer(4, &mtproto.TLPaymentsSendStarGiftOffer{
		Peer: peer, Slug: "Plush-1", Price: amount, Duration: 60, RandomId: 0,
	})) != CodeRandomIDEmpty {
		t.Fatal("random")
	}
	if codeOf(b.SendStarGiftOffer(4, &mtproto.TLPaymentsSendStarGiftOffer{
		Peer: peer, Slug: "Plush-1", Price: amount, Duration: 60, RandomId: 9,
	})) != CodeStarGiftMissing {
		t.Fatal("send")
	}
	if codeOf(b.UniqueStarGift(4, "")) != CodeStarGiftSlugInvalid {
		t.Fatal("unique slug")
	}
	if codeOf(b.UniqueStarGiftValue(4, "Plush-1")) != CodeStarGiftMissing {
		t.Fatal("value")
	}
	auction := mtproto.MakeTLInputStarGiftAuction(&mtproto.InputStarGiftAuction{GiftId: 4}).To_InputStarGiftAuction()
	if codeOf(b.AuctionState(4, auction)) != CodeStarGiftMissing {
		t.Fatal("auction")
	}
	preview, err := b.UpgradePreview(4, 4)
	if err != nil || len(preview.GetSampleAttributes()) != 0 || len(preview.GetPrices()) != 0 {
		t.Fatal(preview, err)
	}
	resale, err := b.ResaleStarGifts(4, &mtproto.TLPaymentsGetResaleStarGifts{GiftId: 4, Limit: 5})
	if err != nil || resale.GetCount() != 0 {
		t.Fatal(resale, err)
	}
	auctions, err := b.ActiveAuctions(4, 1)
	if err != nil || auctions.GetPredicateName() != mtproto.Predicate_payments_starGiftActiveAuctions || len(auctions.GetAuctions()) != 0 {
		t.Fatal(auctions, err)
	}
}

func TestHostGiftMutations(t *testing.T) {
	b := New()
	gift := userGift(8)
	peer := selfPeer()
	if codeOf(b.DeleteStarGift(4, gift)) != CodeStarGiftMissing {
		t.Fatal("delete")
	}
	if codeOf(b.WithdrawStarGift(4, gift, nil)) != "PASSWORD_HASH_INVALID" {
		t.Fatal("withdraw password")
	}
	if codeOf(b.WithdrawStarGift(4, gift, []byte{9})) != CodeStarGiftMissing {
		t.Fatal("withdraw")
	}
	if codeOf(b.DepositStarGift(4, peer, "Bad slug")) != CodeStarGiftSlugInvalid {
		t.Fatal("deposit slug")
	}
	if codeOf(b.DepositStarGift(4, peer, "Plush-1")) != CodeStarGiftMissing {
		t.Fatal("deposit")
	}
	if codeOf(b.DisplayStarGift(4, nil, false, "Plush-1")) != CodeStarGiftMissing {
		t.Fatal("display")
	}
}

func TestCollectibleInfo(t *testing.T) {
	b := New()
	if codeOf(b.CollectibleInfo(4, nil)) != CodeCollectibleInvalid {
		t.Fatal("nil")
	}
	bad := mtproto.MakeTLInputCollectibleUsername(&mtproto.InputCollectible{Username: "no"}).To_InputCollectible()
	if codeOf(b.CollectibleInfo(4, bad)) != CodeCollectibleInvalid {
		t.Fatal("short")
	}
	name := mtproto.MakeTLInputCollectibleUsername(&mtproto.InputCollectible{Username: "telegram"}).To_InputCollectible()
	if codeOf(b.CollectibleInfo(4, name)) != CodeCollectibleMissing {
		t.Fatal("name")
	}
	phone := mtproto.MakeTLInputCollectiblePhone(&mtproto.InputCollectible{Phone: "+15551212"}).To_InputCollectible()
	if codeOf(b.CollectibleInfo(4, phone)) != CodeCollectibleMissing {
		t.Fatal("phone")
	}
}
