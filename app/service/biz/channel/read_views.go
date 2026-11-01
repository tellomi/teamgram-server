package channel

import (
	"context"
	"sort"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Reader is one member who has read a group message, with the unix time
// their inbox cursor last moved.
type Reader struct {
	UserID int64
	Date   int32
}

// MessageViews is messages.getMessagesViews for one channel.
//
// https://core.telegram.org/method/messages.getMessagesViews
//
// Broadcast posts carry a view counter. The counter starts at 1 (the author)
// and increment adds the caller once. A supergroup message has no counter
// unless it is a forward of a channel post, in which case the original
// post's counter is returned. Missing ids occupy a slot with the views
// flag unset, so one stale id does not fail the whole batch.
func (s *Service) MessageViews(ctx context.Context, self, channelID int64, ids []int32, increment bool) ([]*mtproto.MessageViews, error) {
	return s.messageViews(ctx, self, channelID, ids, increment, 0)
}

func (s *Service) messageViews(ctx context.Context, self, channelID int64, ids []int32, increment bool, depth int) ([]*mtproto.MessageViews, error) {
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID}
	ch, _, err := s.requireView(ctx, self, in)
	if err != nil {
		return nil, err
	}
	out := make([]*mtproto.MessageViews, 0, len(ids))
	for _, id := range ids {
		stored, err := s.store.GetMessage(ctx, ch.ID, id)
		if err != nil {
			return nil, err
		}
		if stored == nil || stored.Deleted {
			out = append(out, emptyMessageViews())
			continue
		}
		body, err := unmarshalMessage(stored.Data)
		if err != nil {
			out = append(out, emptyMessageViews())
			continue
		}
		if ch.Broadcast || body.GetViews() != nil {
			n, err := s.bumpViews(ctx, ch, self, stored, body, increment)
			if err != nil {
				return nil, err
			}
			out = append(out, countedViews(n))
			continue
		}
		if depth == 0 {
			if src, post, ok := forwardedChannelPost(body); ok && src != ch.ID {
				nested, err := s.messageViews(ctx, self, src, []int32{post}, increment, depth+1)
				if err != nil || len(nested) == 0 {
					out = append(out, emptyMessageViews())
					continue
				}
				out = append(out, nested[0])
				continue
			}
		}
		out = append(out, emptyMessageViews())
	}
	return out, nil
}

func (s *Service) bumpViews(ctx context.Context, ch *Channel, self int64, stored *StoredMessage, body *mtproto.Message, increment bool) (int32, error) {
	f := s.features(ctx, ch.ID)
	rec, found := f.Views[stored.ID]
	if !found {
		if body.GetViews() != nil {
			rec.Count = body.GetViews().GetValue()
		} else if ch.Broadcast {
			rec.Count = 1
		}
		if stored.SenderID != 0 {
			rec.Seen = []int64{stored.SenderID}
		}
	}
	bumped := false
	if increment && self != 0 && !contains64(rec.Seen, self) {
		rec.Count++
		rec.Seen = append(rec.Seen, self)
		bumped = true
	}
	if !found || bumped {
		f.Views[stored.ID] = rec
		if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
			return 0, err
		}
	}
	if bumped {
		body.Views = wrapperspb.Int32(rec.Count)
		if err := s.SaveMessageBody(ctx, ch.ID, stored.ID, body); err != nil {
			return 0, err
		}
	}
	return rec.Count, nil
}

// ReadParticipants is messages.getMessageReadParticipants for a supergroup.
//
// https://core.telegram.org/method/messages.getMessageReadParticipants
//
// Broadcast channels have view counters instead of read receipts, so the
// list is empty there. The author and the caller are left out: the client
// draws "read by" for everyone else whose inbox has passed the message.
func (s *Service) ReadParticipants(ctx context.Context, self, channelID int64, msgID int32) ([]Reader, error) {
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID}
	ch, _, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if ch.Broadcast {
		return []Reader{}, nil
	}
	stored, err := s.store.GetMessage(ctx, ch.ID, msgID)
	if err != nil {
		return nil, err
	}
	if stored == nil || stored.Deleted {
		return nil, mtproto.ErrMessageIdInvalid
	}
	parts, err := s.store.ListParticipants(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	var out []Reader
	for _, p := range parts {
		if p == nil || !p.InChannel() || p.UserID == 0 || p.UserID == self || p.UserID == stored.SenderID {
			continue
		}
		st, err := s.store.GetRead(ctx, ch.ID, p.UserID)
		if err != nil {
			return nil, err
		}
		if st == nil || st.ReadInboxMaxID < msgID {
			continue
		}
		date := f.InboxReadAt[p.UserID]
		if date == 0 {
			date = stored.Date
		}
		out = append(out, Reader{UserID: p.UserID, Date: date})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	if out == nil {
		out = []Reader{}
	}
	return out, nil
}

// UnreadMentions is messages.getUnreadMentions for a channel or supergroup.
//
// https://core.telegram.org/method/messages.getUnreadMentions
//
// A message is unread when its id is above the caller's inbox cursor, and
// it is a mention when it names the caller, replies to one of their
// messages, or was stored with the mentioned flag. The caller's own
// messages are not mentions of themselves.
func (s *Service) UnreadMentions(ctx context.Context, self, channelID int64, offsetID, addOffset, limit, maxID, minID int32) (*mtproto.Messages_Messages, error) {
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID}
	ch, _, err := s.requireView(ctx, self, in)
	if err != nil {
		return nil, err
	}
	st, err := s.store.GetRead(ctx, ch.ID, self)
	if err != nil {
		return nil, err
	}
	if st == nil {
		st = &ReadState{}
	}
	f := s.features(ctx, ch.ID)
	floor := f.UserMinID[self]
	if f.AvailableMinID > floor {
		floor = f.AvailableMinID
	}
	if minID > floor {
		floor = minID
	}
	all, err := s.store.ListMessages(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	senderOf := map[int32]int64{}
	for _, stored := range all {
		if stored != nil {
			senderOf[stored.ID] = stored.SenderID
		}
	}
	var hit []*mtproto.Message
	for _, stored := range all {
		if stored == nil || stored.Deleted || stored.SenderID == self {
			continue
		}
		if stored.ID <= st.ReadInboxMaxID || stored.ID <= floor {
			continue
		}
		if maxID > 0 && stored.ID >= maxID {
			continue
		}
		body, err := unmarshalMessage(stored.Data)
		if err != nil {
			continue
		}
		if !mentionsUser(body, self, senderOf) {
			continue
		}
		hit = append(hit, viewMessage(body, stored.SenderID, self))
	}
	total := int32(len(hit))
	hit = pageDesc(hit, offsetID, addOffset, limit)
	return mtproto.MakeTLMessagesChannelMessages(&mtproto.Messages_Messages{
		Pts:      ch.Pts,
		Count:    total,
		Inexact:  false,
		Messages: nonempty(hit),
		Chats:    []*mtproto.Chat{s.ChatFor(ctx, self, ch)},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages(), nil
}

// MessageEditData is messages.getMessageEditData for a channel message.
//
// https://core.telegram.org/method/messages.getMessageEditData
//
// The caption flag is set when the message media has a caption the editor
// can change (photo, video, file, gif, music). Text, stickers, voice, and
// round video do not.
func (s *Service) MessageEditData(ctx context.Context, self, channelID int64, msgID int32) (bool, error) {
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID}
	ch, actor, err := s.requireMember(ctx, self, in)
	if err != nil {
		return false, err
	}
	stored, err := s.store.GetMessage(ctx, ch.ID, msgID)
	if err != nil {
		return false, err
	}
	if stored == nil || stored.Deleted {
		return false, mtproto.ErrMessageIdInvalid
	}
	if err = s.canModify(ch, actor, stored, true); err != nil {
		return false, err
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil {
		return false, err
	}
	return mtproto.CaptionEditable(body), nil
}

// VisibleMemberIDs lists current members for a caller who is in the channel.
// messages.getOnlines counts how many of them are online.
func (s *Service) VisibleMemberIDs(ctx context.Context, self, channelID int64) ([]int64, error) {
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID}
	if _, _, err := s.requireMember(ctx, self, in); err != nil {
		return nil, err
	}
	return s.MemberIDs(ctx, channelID)
}

func mentionsUser(msg *mtproto.Message, self int64, senderOf map[int32]int64) bool {
	if msg == nil {
		return false
	}
	if msg.GetMentioned() {
		return true
	}
	for _, e := range msg.GetEntities() {
		if e == nil {
			continue
		}
		switch e.GetPredicateName() {
		case mtproto.Predicate_messageEntityMentionName:
			if e.GetUserId_INT64() == self {
				return true
			}
		case mtproto.Predicate_inputMessageEntityMentionName:
			if e.GetUserId_INPUTUSER().GetUserId() == self {
				return true
			}
		}
	}
	replyID := msg.GetReplyTo().GetReplyToMsgId()
	if replyID == 0 {
		replyID = msg.GetReplyTo().GetReplyToMsgId_INT32()
	}
	return replyID != 0 && senderOf[replyID] == self
}

func forwardedChannelPost(msg *mtproto.Message) (int64, int32, bool) {
	fwd := msg.GetFwdFrom()
	if fwd == nil {
		return 0, 0, false
	}
	if from := fwd.GetFromId(); from.GetPredicateName() == mtproto.Predicate_peerChannel && fwd.GetChannelPost().GetValue() != 0 {
		return from.GetChannelId(), fwd.GetChannelPost().GetValue(), true
	}
	if saved := fwd.GetSavedFromPeer(); saved.GetPredicateName() == mtproto.Predicate_peerChannel && fwd.GetSavedFromMsgId().GetValue() != 0 {
		return saved.GetChannelId(), fwd.GetSavedFromMsgId().GetValue(), true
	}
	return 0, 0, false
}

func contains64(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func emptyMessageViews() *mtproto.MessageViews {
	return mtproto.MakeTLMessageViews(&mtproto.MessageViews{}).To_MessageViews()
}

func countedViews(n int32) *mtproto.MessageViews {
	return mtproto.MakeTLMessageViews(&mtproto.MessageViews{
		Views: wrapperspb.Int32(n),
	}).To_MessageViews()
}
