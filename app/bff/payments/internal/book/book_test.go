package book

import (
	"testing"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func selfPeer() *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
}

func userPeer(id int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: id}).To_InputPeer()
}

func channelPeer(id int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: id}).To_InputPeer()
}

func userInput(id int64) *mtproto.InputUser {
	return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: id}).To_InputUser()
}

func codeOf(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}

func roundTrip(t *testing.T, obj interface {
	Encode(*mtproto.EncodeBuf, int32) error
	Decode(*mtproto.DecodeBuf) error
}) {
	t.Helper()
	buf := mtproto.NewEncodeBuf(2048)
	if err := obj.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	raw := append([]byte(nil), buf.GetBuf()...)
	d := mtproto.NewDecodeBuf(raw)
	if err := obj.Decode(d); err != nil {
		t.Fatal(err)
	}
	if d.GetError() != nil {
		t.Fatal(d.GetError())
	}
	if d.GetOffset() != len(raw) {
		t.Fatalf("decode consumed %d of %d", d.GetOffset(), len(raw))
	}
}

func TestStarsStatusRoundTrip(t *testing.T) {
	b := New()
	got, err := b.StarsStatus(7, selfPeer(), false)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetBalance_STARSAMOUNT().GetAmount() != 0 || got.GetBalance_STARSAMOUNT().GetPredicateName() != mtproto.Predicate_starsAmount {
		t.Fatalf("balance %+v", got.GetBalance_STARSAMOUNT())
	}
	if got.GetHistory() != nil || got.GetSubscriptions() != nil {
		t.Fatal("status should omit history and subscriptions")
	}
	roundTrip(t, got)

	ton, err := b.StarsStatus(7, userPeer(3), true)
	if err != nil {
		t.Fatal(err)
	}
	if ton.GetBalance_STARSAMOUNT().GetPredicateName() != mtproto.Predicate_starsTonAmount {
		t.Fatal(ton.GetBalance_STARSAMOUNT().GetPredicateName())
	}
	roundTrip(t, ton)

	if _, err := b.StarsStatus(7, nil, false); codeOf(err) != CodePeerInvalid {
		t.Fatal(err)
	}
	if _, err := b.StarsStatus(0, selfPeer(), false); codeOf(err) != CodeAuthKeyUnregistered {
		t.Fatal(err)
	}
}

func TestStarsTransactionsAndSubscriptions(t *testing.T) {
	b := New()
	tx, err := b.StarsTransactions(7, &mtproto.TLPaymentsGetStarsTransactions{Peer: selfPeer(), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if tx.GetHistory() == nil || len(tx.GetHistory()) != 0 {
		t.Fatal("empty history flag missing")
	}
	roundTrip(t, tx)
	if _, err := b.StarsTransactions(7, &mtproto.TLPaymentsGetStarsTransactions{Peer: selfPeer(), Limit: 101}); codeOf(err) != CodeLimitInvalid {
		t.Fatal(err)
	}
	if _, err := b.StarsTransactions(7, &mtproto.TLPaymentsGetStarsTransactions{Peer: selfPeer(), Offset: "next"}); codeOf(err) != CodeOffsetInvalid {
		t.Fatal(err)
	}
	if _, err := b.StarsTransactions(7, &mtproto.TLPaymentsGetStarsTransactions{
		Peer: selfPeer(), SubscriptionId: wrapperspb.String("  "),
	}); codeOf(err) != CodeSubscriptionInvalid {
		t.Fatal(err)
	}

	byID, err := b.StarsTransactionsByID(7, &mtproto.TLPaymentsGetStarsTransactionsByID{
		Peer: channelPeer(4),
		Id:   []*mtproto.InputStarsTransaction{mtproto.MakeTLInputStarsTransaction(&mtproto.InputStarsTransaction{Id: "abc"}).To_InputStarsTransaction()},
	})
	if err != nil || len(byID.GetHistory()) != 0 {
		t.Fatal(err, byID)
	}
	roundTrip(t, byID)
	if _, err := b.StarsTransactionsByID(7, &mtproto.TLPaymentsGetStarsTransactionsByID{
		Peer: selfPeer(),
		Id:   []*mtproto.InputStarsTransaction{mtproto.MakeTLInputStarsTransaction(&mtproto.InputStarsTransaction{}).To_InputStarsTransaction()},
	}); codeOf(err) != CodeTransactionInvalid {
		t.Fatal(err)
	}

	subs, err := b.StarsSubscriptions(7, &mtproto.TLPaymentsGetStarsSubscriptions{Peer: selfPeer(), MissingBalance: true})
	if err != nil {
		t.Fatal(err)
	}
	if subs.GetSubscriptions() == nil || subs.GetSubscriptionsMissingBalance().GetValue() != 0 {
		t.Fatalf("%+v", subs)
	}
	roundTrip(t, subs)
	if err := b.ChangeStarsSubscription(7, &mtproto.TLPaymentsChangeStarsSubscription{Peer: selfPeer(), SubscriptionId: "sub"}); codeOf(err) != CodeSubscriptionMissing {
		t.Fatal(err)
	}
	if err := b.FulfillStarsSubscription(7, selfPeer(), ""); codeOf(err) != CodeSubscriptionInvalid {
		t.Fatal(err)
	}
}

func TestStarsMoneyRailsRefuse(t *testing.T) {
	b := New()
	opts, err := b.StarsTopupOptions(7)
	if err != nil || len(opts.GetDatas()) != 0 {
		t.Fatal(err, opts)
	}
	roundTrip(t, opts)
	gifts, err := b.StarsGiftOptions(7, userInput(8))
	if err != nil || len(gifts.GetDatas()) != 0 {
		t.Fatal(err)
	}
	roundTrip(t, gifts)
	if _, err := b.StarsGiftOptions(7, mtproto.MakeTLInputUserEmpty(nil).To_InputUser()); codeOf(err) != CodeUserInvalid {
		t.Fatal(err)
	}

	invoice := mtproto.MakeTLInputInvoiceStars(&mtproto.InputInvoice{}).To_InputInvoice()
	if err := b.SendStarsForm(7, 0, invoice); codeOf(err) != CodeFormInvalid {
		t.Fatal(err)
	}
	if err := b.SendStarsForm(7, 5, nil); codeOf(err) != CodeInvoiceInvalid {
		t.Fatal(err)
	}
	if err := b.SendStarsForm(7, 5, invoice); codeOf(err) != CodeStarsUnavailable {
		t.Fatal(err)
	}
	if err := b.RefundStarsCharge(7, userInput(8), ""); codeOf(err) != CodeChargeInvalid {
		t.Fatal(err)
	}
	if err := b.RefundStarsCharge(7, userInput(8), "charge"); codeOf(err) != CodeChargeMissing {
		t.Fatal(err)
	}

	stats, err := b.StarsRevenueStats(7, channelPeer(4), false)
	if err != nil {
		t.Fatal(err)
	}
	if stats.GetRevenueGraph().GetPredicateName() != mtproto.Predicate_statsGraphError {
		t.Fatal(stats.GetRevenueGraph().GetPredicateName())
	}
	if stats.GetStatus().GetCurrentBalance_STARSAMOUNT().GetAmount() != 0 {
		t.Fatal(stats.GetStatus())
	}
	roundTrip(t, stats)

	pw := mtproto.MakeTLInputCheckPasswordSRP(&mtproto.InputCheckPasswordSRP{A: []byte{1}, M1: []byte{2}}).To_InputCheckPasswordSRP()
	if err := b.StarsRevenueWithdrawalURL(7, &mtproto.TLPaymentsGetStarsRevenueWithdrawalUrl{Peer: selfPeer()}); codeOf(err) != "PASSWORD_HASH_INVALID" {
		t.Fatal(err)
	}
	if err := b.StarsRevenueWithdrawalURL(7, &mtproto.TLPaymentsGetStarsRevenueWithdrawalUrl{
		Peer: selfPeer(), Amount: wrapperspb.Int64(0), Password: pw,
	}); codeOf(err) != CodeAmountInvalid {
		t.Fatal(err)
	}
	if err := b.StarsRevenueWithdrawalURL(7, &mtproto.TLPaymentsGetStarsRevenueWithdrawalUrl{Peer: selfPeer(), Password: pw}); codeOf(err) != CodeBalanceTooLow {
		t.Fatal(err)
	}
	if err := b.StarsRevenueAdsAccountURL(7, selfPeer()); codeOf(err) != CodeAdsUnavailable {
		t.Fatal(err)
	}
}

func TestGiveawaysAndGiftCodes(t *testing.T) {
	b := New()
	codes, err := b.PremiumGiftCodeOptions(7, channelPeer(4))
	if err != nil || len(codes.GetDatas()) != 0 {
		t.Fatal(err)
	}
	roundTrip(t, codes)
	if _, err := b.PremiumGiftCodeOptions(7, &mtproto.InputPeer{}); codeOf(err) != CodePeerInvalid {
		t.Fatal(err)
	}
	give, err := b.StarsGiveawayOptions(7)
	if err != nil || len(give.GetDatas()) != 0 {
		t.Fatal(err)
	}
	roundTrip(t, give)
	if err := b.GiveawayInfo(7, channelPeer(4), 0); codeOf(err) != CodeMsgInvalid {
		t.Fatal(err)
	}
	if err := b.GiveawayInfo(7, channelPeer(4), 15); codeOf(err) != CodeGiveawayMissing {
		t.Fatal(err)
	}
	purpose := mtproto.MakeTLInputStorePaymentPremiumSubscription(&mtproto.InputStorePaymentPurpose{}).To_InputStorePaymentPurpose()
	if err := b.LaunchPrepaidGiveaway(7, channelPeer(4), 0, purpose); codeOf(err) != CodeGiveawayIDInvalid {
		t.Fatal(err)
	}
	if err := b.LaunchPrepaidGiveaway(7, channelPeer(4), 9, nil); codeOf(err) != CodePurposeInvalid {
		t.Fatal(err)
	}
	if err := b.LaunchPrepaidGiveaway(7, channelPeer(4), 9, purpose); codeOf(err) != CodePrepaidMissing {
		t.Fatal(err)
	}
	if err := b.CheckGiftCode(7, "bad slug"); codeOf(err) != CodeSlugInvalid {
		t.Fatal(err)
	}
	if err := b.ApplyGiftCode(7, "Gift_Code-1"); codeOf(err) != CodeGiftCodeMissing {
		t.Fatal(err)
	}
}

func TestBoostSlots(t *testing.T) {
	b := New()
	var now int32 = 1_700_000_000
	b.now = func() int32 { return now }
	ch := channelPeer(9)
	other := channelPeer(10)

	mine, err := b.MyBoosts(7)
	if err != nil || len(mine.GetMyBoosts()) != slotCount {
		t.Fatalf("%v %+v", err, mine)
	}
	roundTrip(t, mine)

	applied, err := b.ApplyBoost(7, ch, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if applied.GetMyBoosts()[0].GetPeer().GetChannelId() != 9 {
		t.Fatalf("slot0 %+v", applied.GetMyBoosts()[0])
	}
	roundTrip(t, applied)

	st, err := b.BoostsStatus(7, ch)
	if err != nil {
		t.Fatal(err)
	}
	if st.GetBoosts() != 1 || !st.GetMyBoost() || len(st.GetMyBoostSlots()) != 1 || st.GetMyBoostSlots()[0] != 0 {
		t.Fatalf("%+v", st)
	}
	if st.GetLevel() != 1 || st.GetNextLevelBoosts().GetValue() != 2 {
		t.Fatalf("level %+v", st)
	}
	roundTrip(t, st)

	list, err := b.BoostsList(7, &mtproto.TLPremiumGetBoostsList{Peer: ch, Limit: 10})
	if err != nil || list.GetCount() != 1 || list.GetBoosts()[0].GetUserId().GetValue() != 7 {
		t.Fatalf("%v %+v", err, list)
	}
	roundTrip(t, list)
	if _, err := b.BoostsList(7, &mtproto.TLPremiumGetBoostsList{Peer: ch, Offset: "nope"}); codeOf(err) != CodeOffsetInvalid {
		t.Fatal(err)
	}
	gifts, err := b.BoostsList(7, &mtproto.TLPremiumGetBoostsList{Peer: ch, Gifts: true})
	if err != nil || gifts.GetCount() != 0 {
		t.Fatal(err, gifts)
	}

	if _, err := b.ApplyBoost(7, other, []int32{0}, true); codeOf(err) != CodeBoostCooldown {
		t.Fatal(err)
	}
	now += cooldown + 1
	moved, err := b.ApplyBoost(7, other, []int32{0}, true)
	if err != nil {
		t.Fatal(err)
	}
	if moved.GetMyBoosts()[0].GetPeer().GetChannelId() != 10 {
		t.Fatalf("moved %+v", moved.GetMyBoosts()[0])
	}
	left, err := b.BoostsStatus(7, ch)
	if err != nil || left.GetBoosts() != 0 || left.GetMyBoost() {
		t.Fatalf("%v %+v", err, left)
	}

	if _, err := b.ApplyBoost(7, ch, []int32{9}, true); codeOf(err) != CodeSlotInvalid {
		t.Fatal(err)
	}
	for i := 0; i < slotCount; i++ {
		if _, err := b.ApplyBoost(7, ch, []int32{int32(i)}, true); err != nil && codeOf(err) != CodeBoostCooldown {
			t.Fatal(err)
		}
	}
	// Fill every slot onto ch, waiting out cooldown when a slot is elsewhere.
	now += cooldown + 1
	for i := 0; i < slotCount; i++ {
		if _, err := b.ApplyBoost(7, ch, []int32{int32(i)}, true); err != nil {
			t.Fatal(i, err)
		}
	}
	if _, err := b.ApplyBoost(8, other, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ApplyBoost(7, other, nil, false); codeOf(err) != CodeBoostsEmpty {
		t.Fatal(err)
	}

	b.premium = func() bool { return false }
	empty, err := b.MyBoosts(7)
	if err != nil || len(empty.GetMyBoosts()) != 0 {
		t.Fatal(err, empty)
	}
	if _, err := b.ApplyBoost(7, ch, nil, false); codeOf(err) != CodePremiumRequired {
		t.Fatal(err)
	}
	hidden, err := b.BoostsStatus(7, ch)
	if err != nil || hidden.GetBoosts() != slotCount || hidden.GetMyBoost() {
		t.Fatalf("non-premium status %+v %v", hidden, err)
	}
}
