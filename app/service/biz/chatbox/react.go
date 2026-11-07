package chatbox

import (
	"context"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/emojiblock"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Reaction is one emoji (or custom-emoji document) on a message.
type Reaction struct {
	MsgID      int32
	UserID     int64
	Emoticon   string
	DocumentID int64
	Date       int32
	Big        bool
}

type reactionItem struct {
	MsgID      int32  `json:"msg_id"`
	UserID     int64  `json:"user_id"`
	Emoticon   string `json:"emoticon,omitempty"`
	DocumentID int64  `json:"document_id,omitempty"`
	Date       int32  `json:"date"`
	Big        bool   `json:"big,omitempty"`
	// Dialog is the shared dialog_message_id of a private-chat message.
	// MsgID is the reactor's own id for it. See msgIDMap.
	Dialog int64 `json:"dialog_msg_id,omitempty"`
}

type reactDoc struct {
	Items       []reactionItem  `json:"items,omitempty"`
	Read        map[int64]int32 `json:"read,omitempty"`
	Mode        string          `json:"mode,omitempty"` // "", all, none, some
	AllowCustom bool            `json:"allow_custom,omitempty"`
	Some        []string        `json:"some,omitempty"`
	CustomDocs  []int64         `json:"custom_docs,omitempty"`
	Limit       int32           `json:"limit,omitempty"`
	Paid        bool            `json:"paid,omitempty"`
}

func reactKey(peerType int32, peerID int64) string {
	return "react:" + peerKey(peerType, peerID)
}

// dialogReactKey is the store key for one dialog.
//
// A private chat is one conversation. User A addressing B and user B
// addressing A must read the same reactions. Basic groups and channels are
// already identified by the shared peer id.
func dialogReactKey(self int64, peer *mtproto.PeerUtil) string {
	if peer == nil {
		return reactKey(0, 0)
	}
	if peer.PeerType == mtproto.PEER_USER || peer.PeerType == mtproto.PEER_SELF {
		other := peer.PeerId
		if peer.PeerType == mtproto.PEER_SELF || other == 0 {
			other = self
		}
		a, b := self, other
		if a > b {
			a, b = b, a
		}
		return "react:dm:" + strconv.FormatInt(a, 10) + ":" + strconv.FormatInt(b, 10)
	}
	return reactKey(peer.PeerType, peer.PeerId)
}

// ParseReaction reads a Reaction constructor.
// https://core.telegram.org/constructor/reactionEmoji
func ParseReaction(r *mtproto.Reaction) (emo string, doc int64, err error) {
	if r == nil {
		return "", 0, mtproto.ErrReactionEmpty
	}
	switch r.PredicateName {
	case mtproto.Predicate_reactionEmpty:
		return "", 0, mtproto.ErrReactionEmpty
	case mtproto.Predicate_reactionCustomEmoji:
		if r.DocumentId == 0 {
			return "", 0, mtproto.ErrReactionInvalid
		}
		return "", r.DocumentId, nil
	default:
		emo = r.Emoticon
		if emo == "" {
			emo = r.GetEmoticon()
		}
		if emo == "" {
			return "", 0, mtproto.ErrReactionInvalid
		}
		emo = mtproto.NormalizeReactionEmoji(emo)
		if emojiblock.Blocked(emo) {
			return "", 0, mtproto.ErrReactionInvalid
		}
		return emo, 0, nil
	}
}

// SetDefaultReaction stores the account's double-tap reaction.
// https://core.telegram.org/method/messages.setDefaultReaction
func (b *Book) SetDefaultReaction(ctx context.Context, userID int64, r *mtproto.Reaction) error {
	emo, doc, err := ParseReaction(r)
	if err != nil {
		return err
	}
	val := emo
	if doc != 0 {
		val = "doc:" + strconv.FormatInt(doc, 10)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := "def:" + userKey(userID)
	if err := b.load(ctx, key, &val); err != nil {
		return err
	}
	// load overwrote val when a previous value existed. Set the new one.
	if doc != 0 {
		val = "doc:" + strconv.FormatInt(doc, 10)
	} else {
		val = emo
	}
	b.def[userID] = val
	return b.save(ctx, key, val)
}

// DefaultReaction returns the account's double-tap reaction, or nil when the
// account still uses the server default.
func (b *Book) DefaultReaction(ctx context.Context, userID int64) *mtproto.Reaction {
	if userID == 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := "def:" + userKey(userID)
	var val string
	if err := b.load(ctx, key, &val); err != nil {
		val = b.def[userID]
	} else if val != "" {
		b.def[userID] = val
	} else {
		val = b.def[userID]
	}
	if val == "" {
		return nil
	}
	if strings.HasPrefix(val, "doc:") {
		id, err := strconv.ParseInt(val[4:], 10, 64)
		if err != nil || id == 0 {
			return nil
		}
		return mtproto.MakeTLReactionCustomEmoji(&mtproto.Reaction{DocumentId: id}).To_Reaction()
	}
	return mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: val}).To_Reaction()
}

// React replaces this user's reactions on a private chat or basic-group message.
func (b *Book) React(ctx context.Context, self int64, peer *mtproto.PeerUtil, msgID int32, emos []string, docs []int64, big bool) (*mtproto.Updates, error) {
	if peer == nil || !peer.IsUserOrChatOrChannel() || peer.IsChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if msgID <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadReact(ctx, self, peer)
	if err != nil {
		return nil, err
	}
	if err := doc.allow(emos, docs); err != nil {
		return nil, err
	}
	now := int32(b.now().Unix())
	dlg := b.dialogIDLocked(ctx, self, peer, msgID)
	kept := doc.Items[:0]
	for _, item := range doc.Items {
		if item.UserID == self && sameMessage(item, msgID, dlg) {
			continue
		}
		kept = append(kept, item)
	}
	doc.Items = kept
	for _, emo := range emos {
		doc.Items = append(doc.Items, reactionItem{MsgID: msgID, UserID: self, Emoticon: emo, Date: now, Big: big, Dialog: dlg})
	}
	for _, id := range docs {
		doc.Items = append(doc.Items, reactionItem{MsgID: msgID, UserID: self, DocumentID: id, Date: now, Big: big, Dialog: dlg})
	}
	if err := b.storeReact(ctx, self, peer, doc); err != nil {
		return nil, err
	}
	if savedDialog(self, peer) {
		b.syncSavedTagsLocked(ctx, self, doc)
	}
	return mtproto.MakeUpdatesByUpdates(reactionUpdate(peer, msgID, b.viewLocked(ctx, self, peer, doc), self)), nil
}

// UnreadReactions returns messages in this dialog that have reactions newer
// than the viewer's read cursor.
// https://core.telegram.org/method/messages.getUnreadReactions
func (b *Book) UnreadReactions(ctx context.Context, self int64, peer *mtproto.PeerUtil, offsetID, addOffset, limit, maxID, minID int32) ([]*mtproto.Message, error) {
	if peer == nil || peer.IsChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadReact(ctx, self, peer)
	if err != nil {
		return nil, err
	}
	doc = b.viewLocked(ctx, self, peer, doc)
	cursor := doc.Read[self]
	seen := map[int32]bool{}
	var msgs []*mtproto.Message
	for _, item := range doc.Items {
		if item.UserID == self || item.Date <= cursor || seen[item.MsgID] {
			continue
		}
		if maxID > 0 && item.MsgID >= maxID {
			continue
		}
		if minID > 0 && item.MsgID <= minID {
			continue
		}
		seen[item.MsgID] = true
		msgs = append(msgs, mtproto.MakeTLMessage(&mtproto.Message{
			Id:        item.MsgID,
			PeerId:    peer.ToPeer(),
			Date:      item.Date,
			Reactions: summary(doc, item.MsgID, self, savedDialog(self, peer)),
		}).To_Message())
	}
	return paginateMessages(msgs, offsetID, addOffset, limit), nil
}

// ReadReactions marks every reaction in the dialog as read.
func (b *Book) ReadReactions(ctx context.Context, self int64, peer *mtproto.PeerUtil) error {
	if peer == nil || peer.IsChannel() {
		return mtproto.ErrPeerIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadReact(ctx, self, peer)
	if err != nil {
		return err
	}
	if doc.Read == nil {
		doc.Read = map[int64]int32{}
	}
	doc.Read[self] = int32(b.now().Unix())
	return b.storeReact(ctx, self, peer, doc)
}

// SetAvailableReactions stores which reactions a private chat or basic group accepts.
func (b *Book) SetAvailableReactions(ctx context.Context, self int64, peer *mtproto.PeerUtil, mode string, allowCustom bool, emos []string, docs []int64, limit int32, paid *bool) error {
	if peer == nil || peer.IsChannel() {
		return mtproto.ErrPeerIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadReact(ctx, self, peer)
	if err != nil {
		return err
	}
	doc.Mode = mode
	doc.AllowCustom = allowCustom
	doc.Some = emos
	doc.CustomDocs = docs
	if limit > 0 {
		doc.Limit = limit
	}
	if paid != nil {
		doc.Paid = *paid
	}
	return b.storeReact(ctx, self, peer, doc)
}

// DeleteParticipantReaction removes one user's reactions on a message.
func (b *Book) DeleteParticipantReaction(ctx context.Context, self int64, peer *mtproto.PeerUtil, msgID int32, participant int64, admin bool) (*mtproto.Updates, error) {
	if peer == nil || peer.IsChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if self != participant && !admin {
		return nil, mtproto.ErrChatAdminRequired
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadReact(ctx, self, peer)
	if err != nil {
		return nil, err
	}
	dlg := b.dialogIDLocked(ctx, self, peer, msgID)
	kept := doc.Items[:0]
	for _, item := range doc.Items {
		if item.UserID == participant && sameMessage(item, msgID, dlg) {
			continue
		}
		kept = append(kept, item)
	}
	doc.Items = kept
	if err := b.storeReact(ctx, self, peer, doc); err != nil {
		return nil, err
	}
	if savedDialog(self, peer) {
		b.syncSavedTagsLocked(ctx, self, doc)
	}
	return mtproto.MakeUpdatesByUpdates(reactionUpdate(peer, msgID, b.viewLocked(ctx, self, peer, doc), self)), nil
}

// DeleteParticipantReactions removes every reaction by participant in the dialog.
func (b *Book) DeleteParticipantReactions(ctx context.Context, self int64, peer *mtproto.PeerUtil, participant int64, admin bool) error {
	if peer == nil || peer.IsChannel() {
		return mtproto.ErrPeerIdInvalid
	}
	if self != participant && !admin {
		return mtproto.ErrChatAdminRequired
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadReact(ctx, self, peer)
	if err != nil {
		return err
	}
	kept := doc.Items[:0]
	for _, item := range doc.Items {
		if item.UserID == participant {
			continue
		}
		kept = append(kept, item)
	}
	doc.Items = kept
	if err := b.storeReact(ctx, self, peer, doc); err != nil {
		return err
	}
	if savedDialog(self, peer) {
		b.syncSavedTagsLocked(ctx, self, doc)
	}
	return nil
}

func (b *Book) loadReact(ctx context.Context, self int64, peer *mtproto.PeerUtil) (*reactDoc, error) {
	key := dialogReactKey(self, peer)
	doc := b.react[key]
	if doc == nil {
		doc = &reactDoc{}
	}
	if err := b.load(ctx, key, doc); err != nil {
		return nil, err
	}
	if doc.Read == nil {
		doc.Read = map[int64]int32{}
	}
	b.react[key] = doc
	return doc, nil
}

// ApplyReactions copies stored reactions onto messages.getHistory rows.
// The reaction RPC returns updateMessageReactions; a later history fetch
// has to carry the same MessageReactions or the bubble disappears.
func (b *Book) ApplyReactions(ctx context.Context, self int64, peer *mtproto.PeerUtil, msgs []*mtproto.Message) {
	if b == nil || peer == nil || peer.IsChannel() || len(msgs) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadReact(ctx, self, peer)
	if err != nil || doc == nil || len(doc.Items) == 0 {
		return
	}
	doc = b.viewLocked(ctx, self, peer, doc)
	tags := savedDialog(self, peer)
	for _, m := range msgs {
		if m == nil || m.GetId() == 0 {
			continue
		}
		sum := summary(doc, m.GetId(), self, tags)
		if len(sum.GetResults()) == 0 {
			continue
		}
		m.Reactions = sum
	}
}

// DifferenceReactions is the updateMessageReactions list a user still needs
// after updates.getDifference(date). updateMessageReactions has no pts, so
// this is the catch-up instead of a pts row. Rows at or before since are
// omitted so the next difference, which carries a later date, does not repeat
// them.
func (b *Book) DifferenceReactions(ctx context.Context, userID int64, since int32) []*mtproto.Update {
	if b == nil || userID == 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []*mtproto.Update
	for key, doc := range b.react {
		peer, ok := peerFromReactKey(key, userID)
		if !ok || doc == nil {
			continue
		}
		view := b.viewLocked(ctx, userID, peer, doc)
		seen := map[int32]bool{}
		for _, item := range view.Items {
			if item.UserID == userID || item.Date <= since || seen[item.MsgID] {
				continue
			}
			seen[item.MsgID] = true
			out = append(out, reactionUpdate(peer, item.MsgID, view, userID))
			if len(out) >= 50 {
				return out
			}
		}
	}
	return out
}

// MessageHasUnreadReaction reports whether userID has an unread reaction
// from someone else on msgID. msgID is that user's own message id.
func (b *Book) MessageHasUnreadReaction(ctx context.Context, userID int64, peer *mtproto.PeerUtil, msgID int32) bool {
	if b == nil || userID == 0 || peer == nil || msgID <= 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadReact(ctx, userID, peer)
	if err != nil || doc == nil {
		return false
	}
	doc = b.viewLocked(ctx, userID, peer, doc)
	readAt := doc.Read[userID]
	for _, item := range doc.Items {
		if item.MsgID == msgID && item.UserID != userID && item.Date > readAt {
			return true
		}
	}
	return false
}

func peerFromReactKey(key string, self int64) (*mtproto.PeerUtil, bool) {
	if strings.HasPrefix(key, "react:dm:") {
		parts := strings.Split(strings.TrimPrefix(key, "react:dm:"), ":")
		if len(parts) != 2 {
			return nil, false
		}
		a, errA := strconv.ParseInt(parts[0], 10, 64)
		b, errB := strconv.ParseInt(parts[1], 10, 64)
		if errA != nil || errB != nil {
			return nil, false
		}
		other := a
		if other == self {
			other = b
		}
		if other == 0 || other == self {
			return nil, false
		}
		return &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: other}, true
	}
	if !strings.HasPrefix(key, "react:") {
		return nil, false
	}
	parts := strings.Split(strings.TrimPrefix(key, "react:"), ":")
	if len(parts) != 2 {
		return nil, false
	}
	kind, errK := strconv.ParseInt(parts[0], 10, 32)
	id, errI := strconv.ParseInt(parts[1], 10, 64)
	if errK != nil || errI != nil || id == 0 {
		return nil, false
	}
	return &mtproto.PeerUtil{PeerType: int32(kind), PeerId: id}, true
}

func (b *Book) storeReact(ctx context.Context, self int64, peer *mtproto.PeerUtil, doc *reactDoc) error {
	key := dialogReactKey(self, peer)
	b.react[key] = doc
	return b.save(ctx, key, doc)
}

func (d *reactDoc) allow(emos []string, docs []int64) error {
	if d.Mode == "none" && (len(emos) > 0 || len(docs) > 0) {
		return mtproto.ErrReactionInvalid
	}
	if d.Mode == "some" {
		ok := map[string]bool{}
		for _, e := range d.Some {
			ok[e] = true
		}
		for _, e := range emos {
			if !ok[e] {
				return mtproto.ErrReactionInvalid
			}
		}
		if len(docs) > 0 && !d.AllowCustom {
			known := map[int64]bool{}
			for _, id := range d.CustomDocs {
				known[id] = true
			}
			for _, id := range docs {
				if !known[id] {
					return mtproto.ErrReactionInvalid
				}
			}
		}
	}
	if d.Mode == "all" && !d.AllowCustom && len(docs) > 0 {
		return mtproto.ErrReactionInvalid
	}
	if d.Limit > 0 {
		uniq := map[string]bool{}
		for _, e := range emos {
			uniq[e] = true
		}
		for _, id := range docs {
			uniq["d"+strconv.FormatInt(id, 10)] = true
		}
		if int32(len(uniq)) > d.Limit {
			return mtproto.ErrReactionsTooMany
		}
	}
	return nil
}

func reactionUpdate(peer *mtproto.PeerUtil, msgID int32, doc *reactDoc, self int64) *mtproto.Update {
	return mtproto.MakeTLUpdateMessageReactions(&mtproto.Update{
		Peer_PEER:                  peer.ToPeer(),
		MsgId_INT32:                msgID,
		Reactions_MESSAGEREACTIONS: summary(doc, msgID, self, savedDialog(self, peer)),
	}).To_Update()
}

func summary(doc *reactDoc, msgID int32, self int64, tags bool) *mtproto.MessageReactions {
	type key struct {
		emo string
		doc int64
	}
	counts := map[key]int32{}
	chosen := map[key]int32{}
	chosenSet := map[key]bool{}
	var order []key
	var selfN int32
	var mine []reactionItem
	for _, item := range doc.Items {
		if item.MsgID != msgID {
			continue
		}
		k := key{item.Emoticon, item.DocumentID}
		if _, ok := counts[k]; !ok {
			order = append(order, k)
		}
		counts[k]++
		if item.UserID == self && !chosenSet[k] {
			chosenSet[k] = true
			chosen[k] = selfN
			selfN++
		}
		mine = append(mine, item)
	}
	var results []*mtproto.ReactionCount
	for _, k := range order {
		rc := mtproto.MakeTLReactionCount(&mtproto.ReactionCount{
			Reaction_REACTION: reactionOf(k.emo, k.doc),
			Reaction_STRING:   k.emo,
			Count:             counts[k],
		}).To_ReactionCount()
		if chosenSet[k] {
			// Bigger chosen_order is the newer reaction from this user.
			// https://core.telegram.org/api/reactions
			rc.ChosenOrder = &wrapperspb.Int32Value{Value: chosen[k]}
		}
		results = append(results, rc)
	}
	if results == nil {
		results = []*mtproto.ReactionCount{}
	}
	out := &mtproto.MessageReactions{Results: results}
	if len(results) > 0 {
		// Private chats and basic groups can open the reactor list.
		out.CanSeeList = true
		out.RecentReactions = recentPeerReactions(mine, self, doc.Read[self], recentReactionCap)
		if tags {
			out.ReactionsAsTags = true
		}
	}
	return mtproto.MakeTLMessageReactions(out).To_MessageReactions()
}
