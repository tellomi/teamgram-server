package accountrest

import (
	"sync"

	"github.com/teamgram/proto/mtproto"
)

// Paid-message revenue and exemptions.
// https://core.telegram.org/method/account.getPaidMessagesRevenue
// https://core.telegram.org/method/account.toggleNoPaidMessagesException
//
// Stars received are recorded per (owner, parent channel, payer). With no
// ledger the amount is 0. refund_charged is accepted and does not invent a balance.
// require_payment without parent_peer is 400 UNSUPPORTED.
// A parent that is not a channel is 400 PARENT_PEER_INVALID.

type paidKey struct {
	owner  int64
	parent int64
	payer  int64
}

type paidState struct {
	stars   int64
	exempt  bool
	require bool
}

type paidStore struct {
	mu   sync.Mutex
	rows map[paidKey]*paidState
}

var paid = &paidStore{rows: map[paidKey]*paidState{}}

func resetPaid() {
	paid.mu.Lock()
	paid.rows = map[paidKey]*paidState{}
	paid.mu.Unlock()
}

// GetPaidMessagesRevenue returns account.paidMessagesRevenue.
func GetPaidMessagesRevenue(self int64, parent *mtproto.InputPeer, user *mtproto.InputUser) (*mtproto.Account_PaidMessagesRevenue, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	parentID, err := parentChannel(parent)
	if err != nil {
		return nil, err
	}
	payer, err := payerID(self, user)
	if err != nil {
		return nil, err
	}
	paid.mu.Lock()
	stars := int64(0)
	if row := paid.rows[paidKey{owner: self, parent: parentID, payer: payer}]; row != nil {
		stars = row.stars
	}
	paid.mu.Unlock()
	return mtproto.MakeTLAccountPaidMessagesRevenue(&mtproto.Account_PaidMessagesRevenue{
		StarsAmount: stars,
	}).To_Account_PaidMessagesRevenue(), nil
}

// ToggleNoPaidMessagesException allows or requires payment from one user.
func ToggleNoPaidMessagesException(self int64, parent *mtproto.InputPeer, user *mtproto.InputUser, refund, require bool) (*mtproto.Bool, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if require && parent == nil {
		return nil, mtproto.ErrUnsupported
	}
	parentID, err := parentChannel(parent)
	if err != nil {
		return nil, err
	}
	payer, err := payerID(self, user)
	if err != nil {
		return nil, err
	}
	_ = refund
	paid.mu.Lock()
	key := paidKey{owner: self, parent: parentID, payer: payer}
	row := paid.rows[key]
	if row == nil {
		row = &paidState{}
		paid.rows[key] = row
	}
	row.require = require
	row.exempt = !require
	paid.mu.Unlock()
	return mtproto.BoolTrue, nil
}

// RequirementsToContact answers users.getRequirementsToContact.
// This server does not charge for messages, so every requested user is
// requirementToContactEmpty. The vector stays the same length as the input.
// https://core.telegram.org/method/users.getRequirementsToContact
func RequirementsToContact(ids []*mtproto.InputUser) *mtproto.Vector_RequirementToContact {
	out := make([]*mtproto.RequirementToContact, 0, len(ids))
	empty := mtproto.MakeTLRequirementToContactEmpty(&mtproto.RequirementToContact{}).To_RequirementToContact()
	for range ids {
		out = append(out, empty)
	}
	return &mtproto.Vector_RequirementToContact{Datas: out}
}

func payerID(self int64, user *mtproto.InputUser) (int64, error) {
	if user == nil {
		return 0, mtproto.ErrUserIdInvalid
	}
	peer := mtproto.FromInputUser(self, user)
	if peer.PeerType != mtproto.PEER_USER && peer.PeerType != mtproto.PEER_SELF {
		return 0, mtproto.ErrUserIdInvalid
	}
	if peer.PeerId == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	return peer.PeerId, nil
}

func parentChannel(parent *mtproto.InputPeer) (int64, error) {
	if parent == nil {
		return 0, nil
	}
	if parent.GetPredicateName() != mtproto.Predicate_inputPeerChannel || parent.GetChannelId() == 0 {
		return 0, mtproto.ErrParentPeerInvalid
	}
	return parent.GetChannelId(), nil
}
