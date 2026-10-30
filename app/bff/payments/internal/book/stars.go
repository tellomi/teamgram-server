package book

import (
	"strings"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (b *Book) StarsStatus(self int64, peer *mtproto.InputPeer, ton bool) (*mtproto.Payments_StarsStatus, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if _, _, err := parsePeer(self, peer); err != nil {
		return nil, err
	}
	return starsStatus(ton, false, false, false), nil
}

func (b *Book) StarsTransactions(self int64, in *mtproto.TLPaymentsGetStarsTransactions) (*mtproto.Payments_StarsStatus, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if in == nil {
		return nil, fail(CodePeerInvalid)
	}
	if _, _, err := parsePeer(self, in.GetPeer()); err != nil {
		return nil, err
	}
	if _, err := pageLimitOf(in.GetLimit()); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.GetOffset()) != "" {
		return nil, fail(CodeOffsetInvalid)
	}
	if in.GetSubscriptionId() != nil {
		if err := requireID(in.GetSubscriptionId().GetValue(), CodeSubscriptionInvalid); err != nil {
			return nil, err
		}
	}
	return starsStatus(in.GetTon(), true, false, false), nil
}

func (b *Book) StarsTransactionsByID(self int64, in *mtproto.TLPaymentsGetStarsTransactionsByID) (*mtproto.Payments_StarsStatus, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if in == nil {
		return nil, fail(CodePeerInvalid)
	}
	if _, _, err := parsePeer(self, in.GetPeer()); err != nil {
		return nil, err
	}
	ids := in.GetId()
	if len(ids) > maxLimit {
		return nil, fail(CodeTransactionsTooMuch)
	}
	for _, id := range ids {
		if id == nil {
			return nil, fail(CodeTransactionInvalid)
		}
		if err := requireID(id.GetId(), CodeTransactionInvalid); err != nil {
			return nil, err
		}
	}
	return starsStatus(in.GetTon(), true, false, false), nil
}

func (b *Book) StarsSubscriptions(self int64, in *mtproto.TLPaymentsGetStarsSubscriptions) (*mtproto.Payments_StarsStatus, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if in == nil {
		return nil, fail(CodePeerInvalid)
	}
	if _, _, err := parsePeer(self, in.GetPeer()); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.GetOffset()) != "" {
		return nil, fail(CodeOffsetInvalid)
	}
	return starsStatus(false, false, true, in.GetMissingBalance()), nil
}

func (b *Book) ChangeStarsSubscription(self int64, in *mtproto.TLPaymentsChangeStarsSubscription) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if in == nil {
		return fail(CodePeerInvalid)
	}
	if _, _, err := parsePeer(self, in.GetPeer()); err != nil {
		return err
	}
	if err := requireID(in.GetSubscriptionId(), CodeSubscriptionInvalid); err != nil {
		return err
	}
	return fail(CodeSubscriptionMissing)
}

func (b *Book) FulfillStarsSubscription(self int64, peer *mtproto.InputPeer, subscriptionID string) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if _, _, err := parsePeer(self, peer); err != nil {
		return err
	}
	if err := requireID(subscriptionID, CodeSubscriptionInvalid); err != nil {
		return err
	}
	return fail(CodeSubscriptionMissing)
}

func (b *Book) BotCancelStarsSubscription(self int64, user *mtproto.InputUser, chargeID string) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if _, err := parseUser(self, user); err != nil {
		return err
	}
	if err := requireID(chargeID, CodeChargeInvalid); err != nil {
		return err
	}
	return fail(CodeChargeMissing)
}

func (b *Book) SendStarsForm(self int64, formID int64, invoice *mtproto.InputInvoice) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if formID <= 0 {
		return fail(CodeFormInvalid)
	}
	if invoice == nil || invoice.GetPredicateName() == "" {
		return fail(CodeInvoiceInvalid)
	}
	return fail(CodeStarsUnavailable)
}

func (b *Book) RefundStarsCharge(self int64, user *mtproto.InputUser, chargeID string) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if _, err := parseUser(self, user); err != nil {
		return err
	}
	if err := requireID(chargeID, CodeChargeInvalid); err != nil {
		return err
	}
	return fail(CodeChargeMissing)
}

func (b *Book) StarsRevenueStats(self int64, peer *mtproto.InputPeer, ton bool) (*mtproto.Payments_StarsRevenueStats, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if _, _, err := parsePeer(self, peer); err != nil {
		return nil, err
	}
	graph := mtproto.MakeTLStatsGraphError(&mtproto.StatsGraph{
		Error: "GRAPH_UNAVAILABLE",
	}).To_StatsGraph()
	rev := mtproto.MakeTLStarsRevenueStatus(&mtproto.StarsRevenueStatus{
		CurrentBalance_STARSAMOUNT:   zeroAmount(ton),
		AvailableBalance_STARSAMOUNT: zeroAmount(ton),
		OverallRevenue_STARSAMOUNT:   zeroAmount(ton),
	}).To_StarsRevenueStatus()
	return mtproto.MakeTLPaymentsStarsRevenueStats(&mtproto.Payments_StarsRevenueStats{
		RevenueGraph: graph,
		Status:       rev,
	}).To_Payments_StarsRevenueStats(), nil
}

func (b *Book) StarsRevenueWithdrawalURL(self int64, in *mtproto.TLPaymentsGetStarsRevenueWithdrawalUrl) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if in == nil {
		return fail(CodePeerInvalid)
	}
	if _, _, err := parsePeer(self, in.GetPeer()); err != nil {
		return err
	}
	if in.GetAmount() != nil && in.GetAmount().GetValue() <= 0 {
		return fail(CodeAmountInvalid)
	}
	if !passwordPresent(in.GetPassword()) {
		return mtproto.ErrPasswordHashInvalid
	}
	return fail(CodeBalanceTooLow)
}

func (b *Book) StarsRevenueAdsAccountURL(self int64, peer *mtproto.InputPeer) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if _, _, err := parsePeer(self, peer); err != nil {
		return err
	}
	return fail(CodeAdsUnavailable)
}

func (b *Book) StarsTopupOptions(self int64) (*mtproto.Vector_StarsTopupOption, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	return &mtproto.Vector_StarsTopupOption{Datas: []*mtproto.StarsTopupOption{}}, nil
}

func (b *Book) StarsGiftOptions(self int64, user *mtproto.InputUser) (*mtproto.Vector_StarsGiftOption, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if user != nil {
		if _, err := parseUser(self, user); err != nil {
			return nil, err
		}
	}
	return &mtproto.Vector_StarsGiftOption{Datas: []*mtproto.StarsGiftOption{}}, nil
}

func starsStatus(ton, history, subs, missing bool) *mtproto.Payments_StarsStatus {
	st := &mtproto.Payments_StarsStatus{
		Balance_STARSAMOUNT: zeroAmount(ton),
		Chats:               []*mtproto.Chat{},
		Users:               []*mtproto.User{},
	}
	if history {
		st.History = []*mtproto.StarsTransaction{}
	}
	if subs {
		st.Subscriptions = []*mtproto.StarsSubscription{}
	}
	if missing {
		st.SubscriptionsMissingBalance = wrapperspb.Int64(0)
	}
	return mtproto.MakeTLPaymentsStarsStatus(st).To_Payments_StarsStatus()
}

func passwordPresent(p *mtproto.InputCheckPasswordSRP) bool {
	if p == nil || p.GetPredicateName() != mtproto.Predicate_inputCheckPasswordSRP {
		return false
	}
	return len(p.GetA()) > 0 && len(p.GetM1()) > 0
}
