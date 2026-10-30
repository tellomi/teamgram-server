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
	doc, err := b.loadReact(ctx, peer)
	if err != nil {
		return nil, err
	}
	if err := doc.allow(emos, docs); err != nil {
		return nil, err
	}
	now := int32(b.now().Unix())
	kept := doc.Items[:0]
	for _, item := range doc.Items {
		if item.MsgID == msgID && item.UserID == self {
			continue
		}
		kept = append(kept, item)
	}
	doc.Items = kept
	for _, emo := range emos {
		doc.Items = append(doc.Items, reactionItem{MsgID: msgID, UserID: self, Emoticon: emo, Date: now, Big: big})
	}
	for _, id := range docs {
		doc.Items = append(doc.Items, reactionItem{MsgID: msgID, UserID: self, DocumentID: id, Date: now, Big: big})
	}
	if err := b.storeReact(ctx, peer, doc); err != nil {
		return nil, err
	}
	if savedDialog(self, peer) {
		b.syncSavedTagsLocked(ctx, self, doc)
	}
	return mtproto.MakeUpdatesByUpdates(reactionUpdate(peer, msgID, doc, self)), nil
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
	doc, err := b.loadReact(ctx, peer)
	if err != nil {
		return nil, err
	}
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
			Reactions: summary(doc, item.MsgID, self),
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
	doc, err := b.loadReact(ctx, peer)
	if err != nil {
		return err
	}
	if doc.Read == nil {
		doc.Read = map[int64]int32{}
	}
	doc.Read[self] = int32(b.now().Unix())
	return b.storeReact(ctx, peer, doc)
}

// SetAvailableReactions stores which reactions a private chat or basic group accepts.
func (b *Book) SetAvailableReactions(ctx context.Context, peer *mtproto.PeerUtil, mode string, allowCustom bool, emos []string, docs []int64, limit int32, paid *bool) error {
	if peer == nil || peer.IsChannel() {
		return mtproto.ErrPeerIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadReact(ctx, peer)
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
	return b.storeReact(ctx, peer, doc)
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
	doc, err := b.loadReact(ctx, peer)
	if err != nil {
		return nil, err
	}
	kept := doc.Items[:0]
	for _, item := range doc.Items {
		if item.MsgID == msgID && item.UserID == participant {
			continue
		}
		kept = append(kept, item)
	}
	doc.Items = kept
	if err := b.storeReact(ctx, peer, doc); err != nil {
		return nil, err
	}
	if savedDialog(self, peer) {
		b.syncSavedTagsLocked(ctx, self, doc)
	}
	return mtproto.MakeUpdatesByUpdates(reactionUpdate(peer, msgID, doc, self)), nil
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
	doc, err := b.loadReact(ctx, peer)
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
	if err := b.storeReact(ctx, peer, doc); err != nil {
		return err
	}
	if savedDialog(self, peer) {
		b.syncSavedTagsLocked(ctx, self, doc)
	}
	return nil
}

func (b *Book) loadReact(ctx context.Context, peer *mtproto.PeerUtil) (*reactDoc, error) {
	key := reactKey(peer.PeerType, peer.PeerId)
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

func (b *Book) storeReact(ctx context.Context, peer *mtproto.PeerUtil, doc *reactDoc) error {
	key := reactKey(peer.PeerType, peer.PeerId)
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
		Reactions_MESSAGEREACTIONS: summary(doc, msgID, self),
	}).To_Update()
}

func summary(doc *reactDoc, msgID int32, self int64) *mtproto.MessageReactions {
	type key struct {
		emo string
		doc int64
	}
	counts := map[key]int32{}
	chosen := map[key]bool{}
	var order []key
	for _, item := range doc.Items {
		if item.MsgID != msgID {
			continue
		}
		k := key{item.Emoticon, item.DocumentID}
		if _, ok := counts[k]; !ok {
			order = append(order, k)
		}
		counts[k]++
		if item.UserID == self {
			chosen[k] = true
		}
	}
	var results []*mtproto.ReactionCount
	for _, k := range order {
		var reaction *mtproto.Reaction
		if k.doc != 0 {
			reaction = mtproto.MakeTLReactionCustomEmoji(&mtproto.Reaction{DocumentId: k.doc}).To_Reaction()
		} else {
			reaction = mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: k.emo}).To_Reaction()
		}
		rc := mtproto.MakeTLReactionCount(&mtproto.ReactionCount{
			Reaction_REACTION: reaction,
			Reaction_STRING:   k.emo,
			Count:             counts[k],
		}).To_ReactionCount()
		if chosen[k] {
			rc.ChosenOrder = &wrapperspb.Int32Value{Value: 0}
		}
		results = append(results, rc)
	}
	if results == nil {
		results = []*mtproto.ReactionCount{}
	}
	return mtproto.MakeTLMessageReactions(&mtproto.MessageReactions{
		Results: results,
	}).To_MessageReactions()
}
