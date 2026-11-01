package channel

import (
	"context"
	"sort"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const maxReactionPage = 100

// ReactionList is messages.getMessageReactionsList for a megagroup.
//
// https://core.telegram.org/method/messages.getMessageReactionsList
//
// A broadcast channel returns BROADCAST_FORBIDDEN so reactors stay anonymous.
// Rows are newest first. offset is the previous next_offset. A missing or
// deleted message is MSG_ID_INVALID.
func (s *Service) ReactionList(ctx context.Context, self, channelID, accessHash int64, msgID int32, reaction *mtproto.Reaction, offset string, limit int32) (*mtproto.Messages_MessageReactionsList, error) {
	in := &mtproto.InputChannel{
		PredicateName: mtproto.Predicate_inputChannel,
		ChannelId:     channelID,
		AccessHash:    accessHash,
	}
	ch, _, err := s.requireView(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !ch.Megagroup {
		return nil, mtproto.ErrBroadcastForbidden
	}
	if msgID <= 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	stored, err := s.store.GetMessage(ctx, ch.ID, msgID)
	if err != nil {
		return nil, err
	}
	if stored == nil || stored.Deleted {
		return nil, mtproto.ErrMsgIdInvalid
	}
	f := s.features(ctx, ch.ID)
	_, readAt := reactionView(ch, f, self)
	var matched []ReactRec
	for _, r := range f.Reactions {
		if r.MsgID != msgID || !channelReactionMatches(reaction, r.Emoticon) {
			continue
		}
		matched = append(matched, r)
	}
	matched = newestChannelReactions(matched)
	start, end, next, err := channelReactionPage(len(matched), offset, limit)
	if err != nil {
		return nil, err
	}
	var rows []*mtproto.MessagePeerReaction
	var ids []int64
	for _, r := range matched[start:end] {
		rows = append(rows, channelPeerReaction(r, self, readAt))
		ids = append(ids, r.UserID)
	}
	if rows == nil {
		rows = []*mtproto.MessagePeerReaction{}
	}
	out := mtproto.MakeTLMessagesMessageReactionsList(&mtproto.Messages_MessageReactionsList{
		Count:     int32(len(matched)),
		Reactions: rows,
		Chats:     []*mtproto.Chat{},
		Users:     channelStubUsers(self, ids),
	}).To_Messages_MessageReactionsList()
	if next != "" {
		out.NextOffset = &wrapperspb.StringValue{Value: next}
	}
	return out, nil
}

func channelReactionMatches(filter *mtproto.Reaction, emo string) bool {
	if filter == nil {
		return true
	}
	switch filter.PredicateName {
	case mtproto.Predicate_reactionEmoji, "":
		want := filter.Emoticon
		if want == "" {
			want = filter.GetEmoticon()
		}
		return want != "" && emo == want
	default:
		// Custom emoji and paid reactions are not stored on channel messages.
		return false
	}
}

func channelReactionPage(n int, offset string, limit int32) (start, end int, next string, err error) {
	if len(offset) > 64 {
		return 0, 0, "", mtproto.ErrNextOffsetInvalid
	}
	if offset != "" {
		v, conv := strconv.Atoi(offset)
		if conv != nil || v < 0 {
			return 0, 0, "", mtproto.ErrOffsetInvalid
		}
		start = v
	}
	if limit < 1 || limit > maxReactionPage {
		limit = maxReactionPage
	}
	if start > n {
		start = n
	}
	end = start + int(limit)
	if end > n {
		end = n
	}
	if end < n {
		next = strconv.Itoa(end)
	}
	return start, end, next, nil
}

func newestChannelReactions(items []ReactRec) []ReactRec {
	type row struct {
		rec ReactRec
		idx int
	}
	rows := make([]row, len(items))
	for i, rec := range items {
		rows[i] = row{rec: rec, idx: i}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].rec.Date != rows[j].rec.Date {
			return rows[i].rec.Date > rows[j].rec.Date
		}
		return rows[i].idx > rows[j].idx
	})
	out := make([]ReactRec, len(rows))
	for i, row := range rows {
		out[i] = row.rec
	}
	return out
}

func recentChannelReactions(items []ReactRec, self int64, readAt, cap int32) []*mtproto.MessagePeerReaction {
	if cap <= 0 || len(items) == 0 {
		return nil
	}
	ordered := newestChannelReactions(items)
	if int32(len(ordered)) > cap {
		ordered = ordered[:cap]
	}
	var rows []*mtproto.MessagePeerReaction
	for _, r := range ordered {
		rows = append(rows, channelPeerReaction(r, self, readAt))
	}
	return rows
}

func channelPeerReaction(r ReactRec, self int64, readAt int32) *mtproto.MessagePeerReaction {
	return mtproto.MakeTLMessagePeerReaction(&mtproto.MessagePeerReaction{
		Big:    r.Big,
		Unread: r.UserID != self && r.Date > readAt,
		My:     r.UserID == self,
		PeerId: mtproto.MakePeerUser(r.UserID),
		Date:   r.Date,
		Reaction_REACTION: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{
			Emoticon: r.Emoticon,
		}).To_Reaction(),
		Reaction_STRING: r.Emoticon,
	}).To_MessagePeerReaction()
}

func channelStubUsers(self int64, ids []int64) []*mtproto.User {
	var out []*mtproto.User
	seen := map[int64]struct{}{}
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, mtproto.MakeTLUser(&mtproto.User{Id: id, Self: id == self}).To_User())
	}
	if out == nil {
		out = []*mtproto.User{}
	}
	return out
}
