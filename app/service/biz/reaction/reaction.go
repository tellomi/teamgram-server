// Package reaction stores emoji reactions on private, group, and channel
// messages and builds the messageReactions object iOS uses for the chip.
package reaction

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Key is the message identity shared by every participant. For a private
// chat or basic group it is the dialog pair plus dialog_message_id. For a
// channel it is MakeDialogId(channel) plus the channel message id, which is
// the same id every member sees.
type Key struct {
	DialogID1       int64
	DialogID2       int64
	DialogMessageID int64
}

// Box maps one user's message id back to Key. Private and basic-group
// message ids differ per user; the box row is how getHistory and
// getDifference find the chip for the id that user sees.
type Box struct {
	UserID   int64
	PeerType int32
	PeerID   int64
	MsgID    int32
}

// Row is one user's emoji on a message.
type Row struct {
	UserID   int64
	Emoticon string
	Big      bool
	Date     int32
}

// Store persists reactions. Memory is used in tests. MySQL is message_reactions.
type Store interface {
	Replace(ctx context.Context, key Key, actor int64, emoticons []string, big bool, boxes []Box) ([]Row, error)
	List(ctx context.Context, key Key) ([]Row, error)
	ListMany(ctx context.Context, keys []Key) (map[Key][]Row, error)
	KeyForBox(ctx context.Context, userID int64, peerType int32, peerID int64, msgID int32) (Key, bool, error)
	SavePts(ctx context.Context, userID int64, pts, ptsCount, updateType int32, updateJSON string, date int64) error
}

// ChannelKey is the shared key for a channel message.
func ChannelKey(channelID int64, msgID int32) Key {
	did := mtproto.MakeDialogId(0, mtproto.PEER_CHANNEL, channelID)
	return Key{DialogID1: did.A, DialogID2: did.B, DialogMessageID: int64(msgID)}
}

// Summary is messageReactions with results, recent_reactions, and chosen_order.
// An empty row list is a cleared chip: both vectors are present and empty.
func Summary(rows []Row, viewer int64) *mtproto.MessageReactions {
	counts := map[string]int32{}
	var order []string
	chosenAt := map[string]int32{}
	var chosenSeq int32
	for _, r := range rows {
		if r.Emoticon == "" {
			continue
		}
		if _, ok := counts[r.Emoticon]; !ok {
			order = append(order, r.Emoticon)
		}
		counts[r.Emoticon]++
		if r.UserID == viewer {
			if _, seen := chosenAt[r.Emoticon]; !seen {
				chosenAt[r.Emoticon] = chosenSeq
				chosenSeq++
			}
		}
	}
	results := make([]*mtproto.ReactionCount, 0, len(order))
	for _, emo := range order {
		rc := mtproto.MakeTLReactionCount(&mtproto.ReactionCount{
			Reaction_REACTION: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: emo}).To_Reaction(),
			Reaction_STRING:   emo,
			Count:             counts[emo],
		}).To_ReactionCount()
		if seq, ok := chosenAt[emo]; ok {
			rc.Chosen = true
			rc.ChosenOrder = &wrapperspb.Int32Value{Value: seq}
		}
		results = append(results, rc)
	}
	recent := make([]*mtproto.MessagePeerReaction, 0, len(rows))
	// Newest last in storage; the client shows the tail, so newest first.
	for i := len(rows) - 1; i >= 0 && len(recent) < 3; i-- {
		r := rows[i]
		if r.Emoticon == "" || r.UserID == 0 {
			continue
		}
		recent = append(recent, mtproto.MakeTLMessagePeerReaction(&mtproto.MessagePeerReaction{
			Big:               r.Big,
			My:                r.UserID == viewer,
			PeerId:            mtproto.MakePeerUser(r.UserID),
			Date:              r.Date,
			Reaction_REACTION: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: r.Emoticon}).To_Reaction(),
			Reaction_STRING:   r.Emoticon,
			Reaction:          r.Emoticon,
		}).To_MessagePeerReaction())
	}
	return mtproto.MakeTLMessageReactions(&mtproto.MessageReactions{
		CanSeeList:      true,
		Results:         results,
		RecentReactions: recent,
	}).To_MessageReactions()
}

// Update is updateMessageReactions for the id this viewer uses.
func Update(peer *mtproto.Peer, msgID int32, reactions *mtproto.MessageReactions) *mtproto.Update {
	return mtproto.MakeTLUpdateMessageReactions(&mtproto.Update{
		Peer_PEER:                  peer,
		MsgId_INT32:                msgID,
		Reactions_MESSAGEREACTIONS: reactions,
	}).To_Update()
}

// Updates wraps one reaction update with the users who reacted.
func Updates(viewer int64, peer *mtproto.Peer, msgID int32, rows []Row, users []*mtproto.User, chats []*mtproto.Chat) *mtproto.Updates {
	if users == nil {
		users = Users(rows)
	}
	return mtproto.MakeUpdatesByUpdatesUsersChats(users, chats, Update(peer, msgID, Summary(rows, viewer)))
}

// Users is a minimal user vector so recent_reactions peers resolve.
func Users(rows []Row) []*mtproto.User {
	seen := map[int64]bool{}
	out := make([]*mtproto.User, 0, len(rows))
	for _, r := range rows {
		if r.UserID == 0 || seen[r.UserID] {
			continue
		}
		seen[r.UserID] = true
		out = append(out, mtproto.MakeTLUser(&mtproto.User{Id: r.UserID, Min: true}).To_User())
	}
	return out
}

// Peer builds the peer a viewer puts on updateMessageReactions.
func Peer(peerType int32, peerID int64) *mtproto.Peer {
	switch peerType {
	case mtproto.PEER_CHAT:
		return mtproto.MakePeerChat(peerID)
	case mtproto.PEER_CHANNEL:
		return mtproto.MakePeerChannel(peerID)
	default:
		return mtproto.MakePeerUser(peerID)
	}
}

// AttachBoxes sets message.reactions from the dialog key stored on each box.
func AttachBoxes(ctx context.Context, st Store, viewer int64, boxes []*mtproto.MessageBox, msgs []*mtproto.Message) {
	if st == nil || len(msgs) == 0 {
		return
	}
	byID := map[int32]Key{}
	var keys []Key
	seen := map[Key]bool{}
	for _, b := range boxes {
		if b == nil || b.DialogMessageId == 0 || b.MessageId == 0 {
			continue
		}
		k := Key{DialogID1: b.DialogId1, DialogID2: b.DialogId2, DialogMessageID: b.DialogMessageId}
		byID[b.MessageId] = k
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	apply(ctx, st, viewer, keys, msgs, func(m *mtproto.Message) (Key, bool) {
		k, ok := byID[m.GetId()]
		return k, ok
	})
}

// AttachMessages sets message.reactions using the per-user box index.
// updates.getDifference new_messages only carry the viewer's message id.
func AttachMessages(ctx context.Context, st Store, viewer int64, msgs []*mtproto.Message) {
	if st == nil || len(msgs) == 0 {
		return
	}
	var keys []Key
	byMsg := map[int32]Key{}
	for _, m := range msgs {
		if m == nil || m.GetPeerId() == nil || m.GetId() == 0 {
			continue
		}
		pt, pid := peerOf(m.GetPeerId())
		k, ok, err := st.KeyForBox(ctx, viewer, pt, pid, m.GetId())
		if err != nil || !ok {
			continue
		}
		byMsg[m.GetId()] = k
		keys = append(keys, k)
	}
	apply(ctx, st, viewer, keys, msgs, func(m *mtproto.Message) (Key, bool) {
		k, ok := byMsg[m.GetId()]
		return k, ok
	})
}

func apply(ctx context.Context, st Store, viewer int64, keys []Key, msgs []*mtproto.Message, keyOf func(*mtproto.Message) (Key, bool)) {
	grouped, err := st.ListMany(ctx, keys)
	if err != nil || len(grouped) == 0 {
		return
	}
	for _, m := range msgs {
		if m == nil {
			continue
		}
		k, ok := keyOf(m)
		if !ok {
			continue
		}
		rows := grouped[k]
		if len(rows) == 0 {
			continue
		}
		m.Reactions = Summary(rows, viewer)
	}
}

func peerOf(p *mtproto.Peer) (int32, int64) {
	switch p.GetPredicateName() {
	case mtproto.Predicate_peerChat:
		return mtproto.PEER_CHAT, p.GetChatId()
	case mtproto.Predicate_peerChannel:
		return mtproto.PEER_CHANNEL, p.GetChannelId()
	default:
		return mtproto.PEER_USER, p.GetUserId()
	}
}

// ListPage filters rows for messages.getMessageReactionsList.
func ListPage(rows []Row, emoticon string, offset string, limit int32) (page []Row, total int32) {
	var filtered []Row
	for _, r := range rows {
		if emoticon != "" && r.Emoticon != emoticon {
			continue
		}
		filtered = append(filtered, r)
	}
	total = int32(len(filtered))
	start := 0
	if offset != "" {
		for i, r := range filtered {
			if offset == itoa(r.UserID)+":"+r.Emoticon {
				start = i + 1
				break
			}
		}
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + int(limit)
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
