package chatbox

import (
	"context"
	"sort"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// recentReactionCap is how many reactors clients draw on the message bubble.
// https://core.telegram.org/constructor/messageReactions
const recentReactionCap = 3

// maxReactionPage is the largest page messages.getMessageReactionsList returns.
const maxReactionPage = 100

// ReactionList is messages.getMessageReactionsList for a private chat or
// basic group.
//
// https://core.telegram.org/method/messages.getMessageReactionsList
// https://core.telegram.org/api/reactions
//
// Rows are newest first. offset is the next_offset from the previous page
// (a decimal count of rows already returned). limit is capped at 100; a
// non-positive limit uses that cap. A reaction filter keeps one emoji or
// custom emoji. Broadcast channels are not served here.
func (b *Book) ReactionList(ctx context.Context, self int64, peer *mtproto.PeerUtil, msgID int32, reaction *mtproto.Reaction, offset string, limit int32) (*mtproto.Messages_MessageReactionsList, error) {
	if peer == nil || !peer.IsUserOrChatOrChannel() || peer.IsChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if msgID <= 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadReact(ctx, peer)
	if err != nil {
		return nil, err
	}
	var matched []reactionItem
	for _, item := range doc.Items {
		if item.MsgID != msgID || !reactionMatches(reaction, item.Emoticon, item.DocumentID) {
			continue
		}
		matched = append(matched, item)
	}
	matched = newestReactions(matched)
	start, end, next, err := reactionPage(len(matched), offset, limit)
	if err != nil {
		return nil, err
	}
	readAt := doc.Read[self]
	var rows []*mtproto.MessagePeerReaction
	var ids []int64
	for _, item := range matched[start:end] {
		rows = append(rows, peerReaction(item.UserID, item.Date, item.Emoticon, item.DocumentID, item.Big, item.UserID != self && item.Date > readAt, item.UserID == self))
		ids = append(ids, item.UserID)
	}
	return reactionListResult(self, int32(len(matched)), rows, ids, next), nil
}

// ReactionsFor is messages.getMessagesReactions for a private chat or basic group.
//
// https://core.telegram.org/method/messages.getMessagesReactions
//
// One updateMessageReactions per positive id. The caller has already dropped
// ids that are not stored messages. An empty id list is MSG_ID_INVALID.
func (b *Book) ReactionsFor(ctx context.Context, self int64, peer *mtproto.PeerUtil, ids []int32) (*mtproto.Updates, error) {
	if peer == nil || !peer.IsUserOrChatOrChannel() || peer.IsChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(ids) == 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadReact(ctx, peer)
	if err != nil {
		return nil, err
	}
	var ups []*mtproto.Update
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		ups = append(ups, reactionUpdate(peer, id, doc, self))
	}
	if len(ups) == 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	return mtproto.MakeUpdatesByUpdates(ups...), nil
}

func reactionMatches(filter *mtproto.Reaction, emo string, doc int64) bool {
	if filter == nil {
		return true
	}
	switch filter.PredicateName {
	case mtproto.Predicate_reactionCustomEmoji:
		return doc != 0 && filter.DocumentId == doc
	case mtproto.Predicate_reactionEmoji, "":
		want := filter.Emoticon
		if want == "" {
			want = filter.GetEmoticon()
		}
		return want != "" && emo == want && doc == 0
	default:
		// reactionEmpty and reactionPaid match nothing stored here.
		return false
	}
}

func reactionPage(n int, offset string, limit int32) (start, end int, next string, err error) {
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

func newestReactions(items []reactionItem) []reactionItem {
	type row struct {
		item reactionItem
		idx  int
	}
	rows := make([]row, len(items))
	for i, item := range items {
		rows[i] = row{item: item, idx: i}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].item.Date != rows[j].item.Date {
			return rows[i].item.Date > rows[j].item.Date
		}
		return rows[i].idx > rows[j].idx
	})
	out := make([]reactionItem, len(rows))
	for i, row := range rows {
		out[i] = row.item
	}
	return out
}

func recentPeerReactions(items []reactionItem, self int64, readAt, cap int32) []*mtproto.MessagePeerReaction {
	if cap <= 0 || len(items) == 0 {
		return nil
	}
	ordered := newestReactions(items)
	if int32(len(ordered)) > cap {
		ordered = ordered[:cap]
	}
	var rows []*mtproto.MessagePeerReaction
	for _, item := range ordered {
		rows = append(rows, peerReaction(item.UserID, item.Date, item.Emoticon, item.DocumentID, item.Big, item.UserID != self && item.Date > readAt, item.UserID == self))
	}
	return rows
}

func peerReaction(userID int64, date int32, emo string, docID int64, big, unread, my bool) *mtproto.MessagePeerReaction {
	return mtproto.MakeTLMessagePeerReaction(&mtproto.MessagePeerReaction{
		Big:               big,
		Unread:            unread,
		My:                my,
		PeerId:            mtproto.MakePeerUser(userID),
		Date:              date,
		Reaction_REACTION: reactionOf(emo, docID),
		Reaction_STRING:   emo,
	}).To_MessagePeerReaction()
}

func reactionOf(emo string, doc int64) *mtproto.Reaction {
	if doc != 0 {
		return mtproto.MakeTLReactionCustomEmoji(&mtproto.Reaction{DocumentId: doc}).To_Reaction()
	}
	return mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: emo}).To_Reaction()
}

func reactionListResult(self int64, count int32, rows []*mtproto.MessagePeerReaction, ids []int64, next string) *mtproto.Messages_MessageReactionsList {
	if rows == nil {
		rows = []*mtproto.MessagePeerReaction{}
	}
	out := mtproto.MakeTLMessagesMessageReactionsList(&mtproto.Messages_MessageReactionsList{
		Count:     count,
		Reactions: rows,
		Chats:     []*mtproto.Chat{},
		Users:     stubUsers(self, ids),
	}).To_Messages_MessageReactionsList()
	if next != "" {
		out.NextOffset = &wrapperspb.StringValue{Value: next}
	}
	return out
}

func stubUsers(self int64, ids []int64) []*mtproto.User {
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
