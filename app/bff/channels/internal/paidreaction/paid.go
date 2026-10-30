// Package paidreaction records messages.sendPaidReaction and
// messages.togglePaidReactionPrivacy.
//
// Star balances are not debited. The handler still returns the message
// reaction update clients apply, with reactionPaid as the reaction.
//
// https://core.telegram.org/method/messages.sendPaidReaction
// https://core.telegram.org/method/messages.togglePaidReactionPrivacy
package paidreaction

import (
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type key struct {
	channel int64
	msg     int32
	user    int64
}

type rec struct {
	count     int32
	anonymous bool
	show      *mtproto.Peer
	seen      map[int64]struct{}
}

var (
	mu   sync.Mutex
	book = map[key]*rec{}
)

// Reset clears recorded reactions. Tests call it.
func Reset() {
	mu.Lock()
	book = map[key]*rec{}
	mu.Unlock()
}

// Send applies a paid reaction on a channel message.
func Send(self int64, peer *mtproto.InputPeer, msgID, count int32, randomID int64, privacy *mtproto.PaidReactionPrivacy) (*mtproto.Updates, error) {
	channelID, err := channelOf(peer)
	if err != nil {
		return nil, err
	}
	if msgID <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if count <= 0 {
		return nil, mtproto.ErrReactionsCountInvalid
	}
	if err := randomIDOK(randomID); err != nil {
		return nil, err
	}
	anonymous, show, err := privacyOf(self, privacy)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	k := key{channel: channelID, msg: msgID, user: self}
	row := book[k]
	if row == nil {
		row = &rec{seen: map[int64]struct{}{}, show: mtproto.MakePeerUser(self)}
		book[k] = row
	}
	if _, ok := row.seen[randomID]; !ok {
		row.count += count
		row.seen[randomID] = struct{}{}
	}
	row.anonymous = anonymous
	if show != nil {
		row.show = show
	}
	return updates(channelID, msgID, row), nil
}

// Toggle changes the privacy of a paid reaction this user already sent.
func Toggle(self int64, peer *mtproto.InputPeer, msgID int32, privacy *mtproto.PaidReactionPrivacy) error {
	channelID, err := togglePeer(peer)
	if err != nil {
		return err
	}
	if privacy == nil {
		return mtproto.ErrReactionEmpty
	}
	anonymous, show, err := privacyOf(self, privacy)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	row := book[key{channel: channelID, msg: msgID, user: self}]
	if row == nil || row.count == 0 {
		return mtproto.ErrReactionEmpty
	}
	row.anonymous = anonymous
	if show != nil {
		row.show = show
	}
	return nil
}

func channelOf(peer *mtproto.InputPeer) (int64, error) {
	if peer == nil || peer.GetPredicateName() == "" || peer.GetPredicateName() == mtproto.Predicate_inputPeerEmpty {
		return 0, mtproto.ErrPeerIdInvalid
	}
	if peer.GetPredicateName() != mtproto.Predicate_inputPeerChannel || peer.GetChannelId() == 0 {
		return 0, mtproto.ErrChannelInvalid
	}
	return peer.GetChannelId(), nil
}

func togglePeer(peer *mtproto.InputPeer) (int64, error) {
	if peer == nil || peer.GetPredicateName() != mtproto.Predicate_inputPeerChannel || peer.GetChannelId() == 0 {
		return 0, mtproto.ErrPeerIdInvalid
	}
	return peer.GetChannelId(), nil
}

func randomIDOK(id int64) error {
	if id == 0 {
		return mtproto.ErrRandomIdEmpty
	}
	ts := int64(uint64(id) >> 32)
	now := time.Now().Unix()
	if ts <= 0 || ts > now+300 || now-ts > 24*3600 {
		return mtproto.ErrRandomIdExpired
	}
	return nil
}

func privacyOf(self int64, p *mtproto.PaidReactionPrivacy) (anonymous bool, show *mtproto.Peer, err error) {
	if p == nil || p.GetPredicateName() == "" || p.GetPredicateName() == mtproto.Predicate_paidReactionPrivacyDefault {
		return false, mtproto.MakePeerUser(self), nil
	}
	switch p.GetPredicateName() {
	case mtproto.Predicate_paidReactionPrivacyAnonymous:
		return true, nil, nil
	case mtproto.Predicate_paidReactionPrivacyPeer:
		show, err = shownPeer(self, p.GetPeer())
		return false, show, err
	default:
		return false, nil, mtproto.ErrPeerIdInvalid
	}
}

func shownPeer(self int64, in *mtproto.InputPeer) (*mtproto.Peer, error) {
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		return mtproto.MakePeerUser(self), nil
	case mtproto.Predicate_inputPeerUser:
		if in.GetUserId() == 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
		return mtproto.MakePeerUser(in.GetUserId()), nil
	case mtproto.Predicate_inputPeerChat:
		if in.GetChatId() == 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
		return mtproto.MakePeerChat(in.GetChatId()), nil
	case mtproto.Predicate_inputPeerChannel:
		if in.GetChannelId() == 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
		return mtproto.MakePeerChannel(in.GetChannelId()), nil
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
}

func updates(channelID int64, msgID int32, row *rec) *mtproto.Updates {
	rc := mtproto.MakeTLReactionCount(&mtproto.ReactionCount{
		Reaction_REACTION: mtproto.MakeTLReactionPaid(&mtproto.Reaction{}).To_Reaction(),
		Count:             row.count,
		ChosenOrder:       &wrapperspb.Int32Value{Value: 0},
	}).To_ReactionCount()
	reactor := &mtproto.MessageReactor{Top: true, My: true, Count: row.count, Anonymous: row.anonymous}
	if !row.anonymous && row.show != nil {
		reactor.PeerId = row.show
	}
	reactions := mtproto.MakeTLMessageReactions(&mtproto.MessageReactions{
		CanSeeList:  true,
		Results:     []*mtproto.ReactionCount{rc},
		TopReactors: []*mtproto.MessageReactor{mtproto.MakeTLMessageReactor(reactor).To_MessageReactor()},
	}).To_MessageReactions()
	upd := mtproto.MakeTLUpdateMessageReactions(&mtproto.Update{
		Peer_PEER:                  mtproto.MakePeerChannel(channelID),
		MsgId_INT32:                msgID,
		Reactions_MESSAGEREACTIONS: reactions,
	}).To_Update()
	return mtproto.MakeUpdatesByUpdates(upd)
}
