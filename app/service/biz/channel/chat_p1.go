package channel

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/emojiblock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func allowReact(f *Features, emos []string) error {
	if f == nil || len(emos) == 0 {
		return nil
	}
	switch f.ReactionMode {
	case "none":
		return mtproto.ErrReactionInvalid
	case "some":
		ok := map[string]bool{}
		for _, e := range f.AvailableReactions {
			ok[e] = true
		}
		for _, e := range emos {
			if e == "" || !ok[e] || emojiblock.Blocked(e) {
				return mtproto.ErrReactionInvalid
			}
		}
	default:
		for _, e := range emos {
			if e == "" || emojiblock.Blocked(e) {
				return mtproto.ErrReactionInvalid
			}
		}
	}
	if f.ReactionLimit > 0 && int32(len(uniqueStrings(emos))) > f.ReactionLimit {
		return mtproto.ErrReactionsTooMany
	}
	return nil
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func chatReactionsOf(f *Features) *mtproto.ChatReactions {
	if f == nil || f.ReactionMode == "" {
		return nil
	}
	switch f.ReactionMode {
	case "none":
		return mtproto.MakeTLChatReactionsNone(&mtproto.ChatReactions{}).To_ChatReactions()
	case "some":
		var reactions []*mtproto.Reaction
		for _, emo := range f.AvailableReactions {
			reactions = append(reactions, mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: emo}).To_Reaction())
		}
		for _, id := range f.CustomReactionDocs {
			reactions = append(reactions, mtproto.MakeTLReactionCustomEmoji(&mtproto.Reaction{DocumentId: id}).To_Reaction())
		}
		if reactions == nil {
			reactions = []*mtproto.Reaction{}
		}
		return mtproto.MakeTLChatReactionsSome(&mtproto.ChatReactions{Reactions: reactions}).To_ChatReactions()
	default:
		return mtproto.MakeTLChatReactionsAll(&mtproto.ChatReactions{AllowCustom: f.AllowCustom}).To_ChatReactions()
	}
}

// ParseChatReactions reads messages.setChatAvailableReactions.
func ParseChatReactions(r *mtproto.ChatReactions, legacy []string) (mode string, allow bool, emos []string, docs []int64, err error) {
	if r == nil && len(legacy) == 0 {
		return "", false, nil, nil, mtproto.ErrReactionInvalid
	}
	if r == nil {
		return "some", false, legacy, nil, nil
	}
	switch r.PredicateName {
	case mtproto.Predicate_chatReactionsNone:
		return "none", false, nil, nil, nil
	case mtproto.Predicate_chatReactionsAll:
		return "all", r.AllowCustom, nil, nil, nil
	case mtproto.Predicate_chatReactionsSome, "":
		for _, item := range r.Reactions {
			if item == nil {
				continue
			}
			if item.PredicateName == mtproto.Predicate_reactionCustomEmoji || item.DocumentId != 0 && item.Emoticon == "" {
				if item.DocumentId == 0 {
					return "", false, nil, nil, mtproto.ErrReactionInvalid
				}
				docs = append(docs, item.DocumentId)
				continue
			}
			emo := item.Emoticon
			if emo == "" || emojiblock.Blocked(emo) {
				return "", false, nil, nil, mtproto.ErrReactionInvalid
			}
			emos = append(emos, emo)
		}
		if len(emos) == 0 && len(docs) == 0 && len(legacy) > 0 {
			emos = legacy
		}
		if len(emos) == 0 && len(docs) == 0 {
			return "", false, nil, nil, mtproto.ErrReactionInvalid
		}
		return "some", len(docs) > 0, emos, docs, nil
	default:
		return "", false, nil, nil, mtproto.ErrReactionInvalid
	}
}

// SetAvailableReactions stores the reaction policy and returns an update that
// tells clients to reload the chat.
func (s *Service) SetAvailableReactions(ctx context.Context, self int64, channelID int64, mode string, allow bool, emos []string, docs []int64, limit int32, paid *bool) (*mtproto.Updates, error) {
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID}
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !s.canChangeInfo(ch, p) {
		return nil, mtproto.ErrChatAdminRequired
	}
	if limit < 0 || limit > 11 {
		return nil, mtproto.ErrReactionsTooMany
	}
	f := s.features(ctx, ch.ID)
	f.ReactionMode = mode
	f.AllowCustom = allow
	f.AvailableReactions = emos
	f.CustomReactionDocs = docs
	if limit > 0 {
		f.ReactionLimit = limit
	}
	if paid != nil {
		f.PaidReactions = *paid
	}
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	return s.updatesWith(ctx, self, ch, channelUpdate(ch.ID)), nil
}

// SetHistoryTTL sets the auto-delete timer for a channel or supergroup.
// https://core.telegram.org/method/messages.setHistoryTTL
func (s *Service) SetHistoryTTL(ctx context.Context, self int64, channelID int64, period int32) (*mtproto.Updates, error) {
	if period < 0 || period > 366*86400 {
		return nil, mtproto.ErrTtlPeriodInvalid
	}
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID}
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !s.canChangeInfo(ch, p) {
		return nil, mtproto.ErrChatAdminRequired
	}
	ch.TtlPeriod = period
	ch.Version++
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	var ttl *wrapperspb.Int32Value
	if period > 0 {
		ttl = &wrapperspb.Int32Value{Value: period}
	}
	up := mtproto.MakeTLUpdatePeerHistoryTTL(&mtproto.Update{
		Peer_PEER: mtproto.MakePeerChannel(ch.ID),
		TtlPeriod: ttl,
	}).To_Update()
	return s.updatesWith(ctx, self, ch, up, channelUpdate(ch.ID)), nil
}

// Card is the channel pts and chat object for a viewer who can see it.
func (s *Service) Card(ctx context.Context, viewer, channelID int64) (int32, *mtproto.Chat, error) {
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID}
	ch, _, err := s.requireView(ctx, viewer, in)
	if err != nil {
		return 0, nil, err
	}
	return ch.Pts, s.ChatFor(ctx, viewer, ch), nil
}

// LoadMessage returns the stored body of one channel message.
func (s *Service) LoadMessage(ctx context.Context, channelID int64, id int32) (*mtproto.Message, error) {
	stored, err := s.store.GetMessage(ctx, channelID, id)
	if err != nil {
		return nil, err
	}
	if stored == nil || stored.Deleted {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return unmarshalMessage(stored.Data)
}

// SaveMessageBody rewrites a message body without marking it edited.
func (s *Service) SaveMessageBody(ctx context.Context, channelID int64, id int32, msg *mtproto.Message) error {
	if msg == nil {
		return mtproto.ErrMessageIdInvalid
	}
	msg.Out = false
	raw, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	return s.store.ReplaceMessageBody(ctx, channelID, id, raw)
}

// UnreadReactions returns channel messages that have reactions newer than
// the viewer's messages.readReactions cursor.
func (s *Service) UnreadReactions(ctx context.Context, self, channelID int64, topMsgID, offsetID, addOffset, limit, maxID, minID int32) (*mtproto.Messages_Messages, error) {
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID}
	ch, _, err := s.requireView(ctx, self, in)
	if err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	cursor := f.ReactionRead[self]
	ids := map[int32]bool{}
	for _, r := range f.Reactions {
		if r.UserID == self || r.Date <= cursor {
			continue
		}
		if topMsgID != 0 && !sameTopic(f, r.MsgID, topMsgID) {
			continue
		}
		if maxID > 0 && r.MsgID >= maxID {
			continue
		}
		if minID > 0 && r.MsgID <= minID {
			continue
		}
		ids[r.MsgID] = true
	}
	var msgs []*mtproto.Message
	for id := range ids {
		body, err := s.LoadMessage(ctx, ch.ID, id)
		if err != nil {
			continue
		}
		body.Reactions = reactionSummary(f.Reactions, id, self)
		msgs = append(msgs, body)
	}
	msgs = pageDesc(msgs, offsetID, addOffset, limit)
	return mtproto.MakeTLMessagesChannelMessages(&mtproto.Messages_Messages{
		Pts:      ch.Pts,
		Count:    int32(len(msgs)),
		Messages: nonempty(msgs),
		Chats:    []*mtproto.Chat{s.ChatFor(ctx, self, ch)},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages(), nil
}

// ReadReactions marks channel reactions as read and returns affectedHistory.
func (s *Service) ReadReactions(ctx context.Context, self, channelID int64, topMsgID int32) (*mtproto.Messages_AffectedHistory, error) {
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID}
	ch, _, err := s.requireView(ctx, self, in)
	if err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	if f.ReactionRead == nil {
		f.ReactionRead = map[int64]int32{}
	}
	now := int32(time.Now().Unix())
	if topMsgID == 0 {
		f.ReactionRead[self] = now
	} else {
		// A topic read still advances the dialog cursor. Topic filtering on
		// the next getUnreadReactions uses the same cursor, which matches
		// "mark the reactions I can see as read".
		f.ReactionRead[self] = now
	}
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      ch.Pts,
		PtsCount: 0,
		Offset:   0,
	}).To_Messages_AffectedHistory(), nil
}

// DeleteParticipantReaction removes one member's reactions on a message.
// The member can remove their own; an admin can remove anyone's.
func (s *Service) DeleteParticipantReaction(ctx context.Context, self, channelID int64, msgID int32, participant int64) (*mtproto.Updates, error) {
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID}
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if self != participant && !p.IsAdmin() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if _, err := s.LoadMessage(ctx, ch.ID, msgID); err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	kept := f.Reactions[:0]
	for _, r := range f.Reactions {
		if r.MsgID == msgID && r.UserID == participant {
			continue
		}
		kept = append(kept, r)
	}
	f.Reactions = kept
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	up := mtproto.MakeTLUpdateMessageReactions(&mtproto.Update{
		Peer_PEER:                  mtproto.MakePeerChannel(ch.ID),
		MsgId_INT32:                msgID,
		Reactions_MESSAGEREACTIONS: reactionSummary(f.Reactions, msgID, self),
	}).To_Update()
	return s.updatesWith(ctx, self, ch, up), nil
}

// DeleteParticipantReactions removes every reaction by one member.
func (s *Service) DeleteParticipantReactions(ctx context.Context, self, channelID, participant int64) (*mtproto.Bool, error) {
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID}
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if self != participant && !p.IsAdmin() {
		return nil, mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	kept := f.Reactions[:0]
	for _, r := range f.Reactions {
		if r.UserID == participant {
			continue
		}
		kept = append(kept, r)
	}
	f.Reactions = kept
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func sameTopic(f *Features, msgID, top int32) bool {
	if msgID == top {
		return true
	}
	if f == nil || f.MsgTopic == nil {
		return false
	}
	return f.MsgTopic[msgID] == top
}

func pageDesc(msgs []*mtproto.Message, offsetID, addOffset, limit int32) []*mtproto.Message {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	for i := 0; i < len(msgs); i++ {
		for j := i + 1; j < len(msgs); j++ {
			if msgs[j].Id > msgs[i].Id {
				msgs[i], msgs[j] = msgs[j], msgs[i]
			}
		}
	}
	start := 0
	if offsetID > 0 {
		for start < len(msgs) && msgs[start].Id >= offsetID {
			start++
		}
	}
	start += int(addOffset)
	if start < 0 {
		start = 0
	}
	if start > len(msgs) {
		start = len(msgs)
	}
	end := start + int(limit)
	if end > len(msgs) {
		end = len(msgs)
	}
	return msgs[start:end]
}

func nonempty(msgs []*mtproto.Message) []*mtproto.Message {
	if msgs == nil {
		return []*mtproto.Message{}
	}
	return msgs
}
