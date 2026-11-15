package channel

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
)

const maxParticipantRank = 16

// EditParticipantRank is messages.editChatParticipantRank for a supergroup
// or channel. A member may set their own rank unless edit_rank is banned.
// Changing someone else's rank needs manage_ranks (the creator always has it).
// https://core.telegram.org/method/messages.editChatParticipantRank
func (s *Service) EditParticipantRank(ctx context.Context, actor, channelID, userID int64, rank string) (*mtproto.Updates, error) {
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID}
	ch, actorP, err := s.requireMember(ctx, actor, in)
	if err != nil {
		return nil, err
	}
	target, err := s.store.GetParticipant(ctx, ch.ID, userID)
	if err != nil {
		return nil, err
	}
	if target == nil || !target.InChannel() {
		return nil, mtproto.ErrUserNotParticipant
	}
	rank = strings.TrimSpace(rank)
	if len(rank) > maxParticipantRank {
		return nil, mtproto.ErrAdminRankInvalid
	}
	if err = rankEditAllowed(ch, actorP, target); err != nil {
		return nil, err
	}
	prev := target.Rank
	if prev == rank {
		return s.pack(ctx, actor, ch.ID, channelUpdate(ch.ID)), nil
	}
	target.Rank = rank
	if err = s.store.PutParticipant(ctx, target); err != nil {
		return nil, err
	}
	s.logAdmin(ctx, ch.ID, actor, mtproto.MakeTLChannelAdminLogEventActionParticipantEditRank(&mtproto.ChannelAdminLogEventAction{
		UserId:   userID,
		PrevRank: prev,
		NewRank:  rank,
	}).To_ChannelAdminLogEventAction())
	return s.pack(ctx, actor, ch.ID, channelUpdate(ch.ID)), nil
}

func rankEditAllowed(ch *Channel, actor, target *Participant) error {
	if actor == nil || target == nil {
		return mtproto.ErrPeerIdInvalid
	}
	if actor.UserID == target.UserID {
		if actor.IsCreator() || actor.adminRights().CanManageRanks() {
			return nil
		}
		if editRankBanned(ch, actor) {
			return mtproto.ErrChatAdminRequired
		}
		return nil
	}
	if actor.IsCreator() || actor.adminRights().CanManageRanks() {
		return nil
	}
	return mtproto.ErrChatAdminRequired
}

// editRankBanned is true when the default ban or this member's ban still
// forbids editing a rank. until_date 0 is a permanent ban, same as send bans.
func editRankBanned(ch *Channel, p *Participant) bool {
	now := int32(time.Now().Unix())
	for _, br := range memberBans(ch, p) {
		if br == nil || !br.GetEditRank() {
			continue
		}
		until := br.GetUntilDate()
		if until == 0 {
			until = math.MaxInt32
		}
		if now < until {
			return true
		}
	}
	return false
}
