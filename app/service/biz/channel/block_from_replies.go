package channel

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/msgexpiry"
)

// ReplyAuthorForBlock is the user contacts.blockFromReplies should block when
// msg_id is not a message in the @replies chat.
//
// Official clients pass the id of a notification in that chat. This server
// does not write those notifications, and the callers in
// ISSUE-S-BLOCK-FROM-REPLIES-001 pass the supergroup reply id or the
// discussion-group comment id instead. Those ids live on a channel, and the
// same number can exist in more than one channel the caller can see.
//
// A hit is a reply, or any user message in a linked discussion group. A reply
// to the caller's own message and a discussion comment outrank a reply to
// someone else. Two different authors at that rank is MSG_ID_INVALID: the id
// alone does not say which person to block. TELLOMI_CHANNEL_COMMENTS is not
// consulted. Discussion comments are ordinary messages in the linked group
// when that flag is off.
func (s *Service) ReplyAuthorForBlock(ctx context.Context, self int64, msgID int32) (int64, error) {
	if s == nil || s.store == nil || self == 0 || msgID <= 0 {
		return 0, mtproto.ErrMsgIdInvalid
	}
	ids, err := s.replyBlockChannels(ctx, self)
	if err != nil {
		return 0, err
	}
	var hits []replyBlockHit
	for _, id := range ids {
		ch, err := s.store.GetChannel(ctx, id)
		if err != nil {
			return 0, err
		}
		ok, err := s.viewerCanSeeReplyChannel(ctx, self, ch)
		if err != nil {
			return 0, err
		}
		if !ok {
			continue
		}
		stored, err := s.store.GetMessage(ctx, id, msgID)
		if err != nil {
			return 0, err
		}
		hit, found, err := s.replyBlockCandidate(ctx, self, ch, stored)
		if err != nil {
			return 0, err
		}
		if found {
			hits = append(hits, hit)
		}
	}
	return pickReplyBlockSender(hits)
}

type replyBlockHit struct {
	sender int64
	score  int
}

func (s *Service) replyBlockChannels(ctx context.Context, self int64) ([]int64, error) {
	joined, err := s.store.ListByUser(ctx, self)
	if err != nil {
		return nil, err
	}
	seen := map[int64]struct{}{}
	var ids []int64
	add := func(id int64) {
		if id == 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, ch := range joined {
		if ch != nil {
			add(ch.ID)
		}
	}
	base := append([]int64(nil), ids...)
	for _, id := range base {
		add(s.features(ctx, id).LinkedChatID)
	}
	return ids, nil
}

func (s *Service) viewerCanSeeReplyChannel(ctx context.Context, self int64, ch *Channel) (bool, error) {
	if ch == nil || ch.Deleted {
		return false, nil
	}
	p, err := s.store.GetParticipant(ctx, ch.ID, self)
	if err != nil {
		return false, err
	}
	if p != nil && p.State == StateKicked {
		return false, nil
	}
	if p != nil && p.InChannel() {
		return true, nil
	}
	// A comment in the linked discussion group is visible to someone who can
	// see the broadcast, including a member who has not joined the group.
	if !ch.Megagroup {
		return false, nil
	}
	link := s.features(ctx, ch.ID).LinkedChatID
	if link == 0 {
		return false, nil
	}
	if err := s.canViewChannel(ctx, link, self); err != nil {
		return false, nil
	}
	return true, nil
}

func (s *Service) replyBlockCandidate(ctx context.Context, self int64, ch *Channel, stored *StoredMessage) (replyBlockHit, bool, error) {
	if ch == nil || stored == nil || stored.Deleted {
		return replyBlockHit{}, false, nil
	}
	if msgexpiry.Enabled() && msgexpiry.Expired(stored.Date, ch.TtlPeriod, int32(time.Now().Unix())) {
		return replyBlockHit{}, false, nil
	}
	f := s.features(ctx, ch.ID)
	minID := f.AvailableMinID
	if f.UserMinID[self] > minID {
		minID = f.UserMinID[self]
	}
	if stored.ID <= minID {
		return replyBlockHit{}, false, nil
	}
	viewer, err := s.store.GetParticipant(ctx, ch.ID, self)
	if err != nil {
		return replyBlockHit{}, false, err
	}
	if prehistoryBlocks(f, viewer, stored) {
		return replyBlockHit{}, false, nil
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil || isServiceMessage(body) {
		return replyBlockHit{}, false, nil
	}
	discussion := ch.Megagroup && f.LinkedChatID != 0
	if !discussion && f.CommentMsgRoot[stored.ID] != 0 {
		discussion = true
	}
	replied := replyMsgID(body) != 0 || replyTopID(body) != 0
	if !discussion && !replied {
		return replyBlockHit{}, false, nil
	}
	author := replyBlockAuthor(ch, stored, body)
	if author == 0 || author == self {
		return replyBlockHit{}, false, nil
	}
	toSelf, err := replyAddresses(ctx, s, ch, body, self)
	if err != nil {
		return replyBlockHit{}, false, err
	}
	score := 1
	if discussion || toSelf {
		score = 8
	}
	return replyBlockHit{sender: author, score: score}, true, nil
}

func replyAddresses(ctx context.Context, s *Service, ch *Channel, body *mtproto.Message, self int64) (bool, error) {
	target := replyMsgID(body)
	if target == 0 {
		target = replyTopID(body)
	}
	if target == 0 {
		return false, nil
	}
	parent, err := s.store.GetMessage(ctx, ch.ID, target)
	if err != nil {
		return false, err
	}
	if parent == nil || parent.Deleted {
		return false, nil
	}
	parentBody, err := unmarshalMessage(parent.Data)
	if err != nil {
		return false, nil
	}
	return replyBlockAuthor(ch, parent, parentBody) == self, nil
}

func replyBlockAuthor(ch *Channel, stored *StoredMessage, body *mtproto.Message) int64 {
	if body != nil {
		if id := body.GetFromId().GetUserId(); id != 0 {
			return id
		}
		// An anonymous admin post must not be turned back into the user id.
		if body.GetFromId().GetChannelId() != 0 {
			return 0
		}
	}
	if ch != nil && ch.Megagroup && stored != nil {
		return stored.SenderID
	}
	return 0
}

func pickReplyBlockSender(hits []replyBlockHit) (int64, error) {
	best := 0
	for _, hit := range hits {
		if hit.score > best {
			best = hit.score
		}
	}
	var sender int64
	for _, hit := range hits {
		if hit.score != best || hit.sender == 0 {
			continue
		}
		if sender == 0 {
			sender = hit.sender
			continue
		}
		if sender != hit.sender {
			return 0, mtproto.ErrMsgIdInvalid
		}
	}
	if sender == 0 {
		return 0, mtproto.ErrMsgIdInvalid
	}
	return sender, nil
}
