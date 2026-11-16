package chatbox

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/hashx"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Quick replies are per-user shortcut inboxes.
// https://core.telegram.org/api/business#quick-reply-shortcuts
//
// messages.sendMessage, sendMedia, and sendMultiMedia add a message when
// quick_reply_shortcut is set and the peer is the current user.
// messages.sendQuickReplyMessages copies those messages into another chat.

const (
	maxQuickReplies        = 100
	maxQuickReplyMessages  = 20
	maxQuickReplyNameRunes = 32
)

type quickDoc struct {
	NextID    int32           `json:"next_id"`
	NextMsg   int32           `json:"next_msg"`
	Shortcuts []quickShortcut `json:"shortcuts,omitempty"`
}

type quickShortcut struct {
	ID       int32      `json:"id"`
	Name     string     `json:"name"`
	Messages []quickMsg `json:"messages,omitempty"`
}

type quickMsg struct {
	ID  int32  `json:"id"`
	Raw []byte `json:"raw"`
}

func quickKey(user int64) string { return "quick:" + userKey(user) }

// CheckQuickReplyShortcut reports whether shortcut is a free, valid name.
// https://core.telegram.org/method/messages.checkQuickReplyShortcut
func (b *Book) CheckQuickReplyShortcut(ctx context.Context, user int64, name string) (*mtproto.Bool, error) {
	norm, ok := normalizeShortcut(name)
	if !ok {
		return nil, mtproto.ErrShortcutInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadQuick(ctx, user)
	if err != nil {
		return nil, err
	}
	if findShortcut(doc, norm) >= 0 {
		return mtproto.BoolFalse, nil
	}
	return mtproto.BoolTrue, nil
}

// QuickReplies is messages.getQuickReplies.
// A matching hash returns messages.quickRepliesNotModified.
func (b *Book) QuickReplies(ctx context.Context, user int64, hash int64) (*mtproto.Messages_QuickReplies, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadQuick(ctx, user)
	if err != nil {
		return nil, err
	}
	replies, tops := shortcutView(doc)
	sum := hashQuick(replies, tops)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesQuickRepliesNotModified(&mtproto.Messages_QuickReplies{}).To_Messages_QuickReplies(), nil
	}
	return mtproto.MakeTLMessagesQuickReplies(&mtproto.Messages_QuickReplies{
		QuickReplies: replies,
		Messages:     tops,
		Chats:        []*mtproto.Chat{},
		Users:        []*mtproto.User{},
	}).To_Messages_QuickReplies(), nil
}

// QuickReplyMessages is messages.getQuickReplyMessages.
func (b *Book) QuickReplyMessages(ctx context.Context, user int64, shortcutID int32, ids []int32, hash int64) (*mtproto.Messages_Messages, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadQuick(ctx, user)
	if err != nil {
		return nil, err
	}
	sc := shortcutByID(doc, shortcutID)
	if sc == nil {
		return nil, mtproto.ErrShortcutInvalid
	}
	msgs, err := filterQuick(sc, ids)
	if err != nil {
		return nil, err
	}
	sum := hashQuickMessages(msgs)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesMessagesNotModified(&mtproto.Messages_Messages{
			Count: int32(len(msgs)),
		}).To_Messages_Messages(), nil
	}
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: msgs,
		Topics:   []*mtproto.ForumTopic{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages(), nil
}

// EditQuickReplyShortcut renames a shortcut.
// https://core.telegram.org/method/messages.editQuickReplyShortcut
func (b *Book) EditQuickReplyShortcut(ctx context.Context, user int64, shortcutID int32, name string) (*mtproto.Bool, error) {
	norm, ok := normalizeShortcut(name)
	if !ok {
		return nil, mtproto.ErrShortcutInvalid
	}
	err := b.updateQuick(ctx, user, func(doc *quickDoc) error {
		idx := indexShortcut(doc, shortcutID)
		if idx < 0 {
			return mtproto.ErrShortcutInvalid
		}
		if other := findShortcut(doc, norm); other >= 0 && other != idx {
			return mtproto.ErrShortcutInvalid
		}
		doc.Shortcuts[idx].Name = norm
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// DeleteQuickReplyShortcut removes a shortcut and its messages.
func (b *Book) DeleteQuickReplyShortcut(ctx context.Context, user int64, shortcutID int32) (*mtproto.Bool, error) {
	err := b.updateQuick(ctx, user, func(doc *quickDoc) error {
		idx := indexShortcut(doc, shortcutID)
		if idx < 0 {
			return mtproto.ErrShortcutInvalid
		}
		doc.Shortcuts = append(doc.Shortcuts[:idx], doc.Shortcuts[idx+1:]...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// ReorderQuickReplies applies order as a permutation of every shortcut id.
func (b *Book) ReorderQuickReplies(ctx context.Context, user int64, order []int32) (*mtproto.Bool, error) {
	err := b.updateQuick(ctx, user, func(doc *quickDoc) error {
		if len(order) != len(doc.Shortcuts) {
			return mtproto.ErrShortcutInvalid
		}
		byID := map[int32]quickShortcut{}
		for _, sc := range doc.Shortcuts {
			byID[sc.ID] = sc
		}
		next := make([]quickShortcut, 0, len(order))
		seen := map[int32]bool{}
		for _, id := range order {
			sc, ok := byID[id]
			if !ok || seen[id] {
				return mtproto.ErrShortcutInvalid
			}
			seen[id] = true
			next = append(next, sc)
		}
		doc.Shortcuts = next
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// QuickReplyEdit is one messages.editMessage applied to a shortcut.
// Nil text, entities, markup, and media leave those fields as they are.
// A non-nil media replaces the media, including messageMediaEmpty.
// Markup is applied only when the caller already decided it is allowed.
type QuickReplyEdit struct {
	Text     *string
	Entities []*mtproto.MessageEntity
	Markup   *mtproto.ReplyMarkup
	Media    *mtproto.MessageMedia
}

// EditQuickReplyMessage rewrites text and entities. Media and reply markup
// go through EditQuickReply.
func (b *Book) EditQuickReplyMessage(ctx context.Context, user int64, shortcutID, msgID int32, text *string, entities []*mtproto.MessageEntity) (*mtproto.Updates, error) {
	return b.EditQuickReply(ctx, user, shortcutID, msgID, QuickReplyEdit{Text: text, Entities: entities})
}

// EditQuickReply rewrites one message inside a shortcut.
// messages.editMessage with quick_reply_shortcut_id addresses this inbox,
// not the saved-messages box. The shortcut ids start at 1 and otherwise
// collide with a forwarded saved message, which returned
// MESSAGE_AUTHOR_REQUIRED. The shortcut owner is the editor. A from_id left
// on a captured message is not an authorship check.
//
// An identical resubmit returns MESSAGE_NOT_MODIFIED and the current
// update, and does not bump edit_date or write another message. The caller
// can push that update so a missed sync still reaches other sessions.
// A failed save leaves the previous document in place.
//
// Every quick: writer reloads and reapplies when the stored bytes changed.
// Capture, delete, rename, and reorder share this path with the edit, so a
// stale snapshot cannot put an older message body back. Save on other
// document keys is unchanged.
func (b *Book) EditQuickReply(ctx context.Context, user int64, shortcutID, msgID int32, edit QuickReplyEdit) (*mtproto.Updates, error) {
	if shortcutID == 0 || msgID == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	var ups *mtproto.Updates
	err := b.updateQuick(ctx, user, func(doc *quickDoc) error {
		next, err := stageQuickEdit(doc, shortcutID, msgID, edit, int32(b.now().Unix()))
		if err != nil {
			if err == mtproto.ErrMessageNotModified && next != nil {
				ups = mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateQuickReplyMessage(&mtproto.Update{
					Message_MESSAGE: next,
				}).To_Update())
			}
			return err
		}
		ups = mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateQuickReplyMessage(&mtproto.Update{
			Message_MESSAGE: next,
		}).To_Update())
		return nil
	})
	return ups, err
}

// stageQuickEdit mutates doc when the edit changes the message.
// MESSAGE_NOT_MODIFIED returns the stored message and leaves doc unchanged.
func stageQuickEdit(doc *quickDoc, shortcutID, msgID int32, edit QuickReplyEdit, now int32) (*mtproto.Message, error) {
	sc := shortcutByID(doc, shortcutID)
	if sc == nil {
		return nil, mtproto.ErrShortcutInvalid
	}
	for i := range sc.Messages {
		if sc.Messages[i].ID != msgID {
			continue
		}
		cur := &mtproto.Message{}
		if err := proto.Unmarshal(sc.Messages[i].Raw, cur); err != nil {
			return nil, err
		}
		next := proto.Clone(cur).(*mtproto.Message)
		if edit.Text != nil {
			next.Message = *edit.Text
			if edit.Entities == nil {
				next.Entities = nil
			}
		}
		if edit.Entities != nil {
			next.Entities = edit.Entities
		}
		if edit.Markup != nil {
			next.ReplyMarkup = edit.Markup
		}
		if edit.Media != nil {
			next.Media = mtproto.KeepTodoCompletions(next.GetMedia(), edit.Media)
		}
		if edit.Text != nil && *edit.Text == "" && !quickMediaHoldsCaption(next.GetMedia()) {
			return nil, mtproto.ErrMessageEmpty
		}
		if mtproto.MessageContentUnchanged(cur, next) {
			return cur, mtproto.ErrMessageNotModified
		}
		next.EditDate = wrapperspb.Int32(now)
		raw, err := proto.Marshal(next)
		if err != nil {
			return nil, err
		}
		sc.Messages[i].Raw = raw
		return next, nil
	}
	return nil, mtproto.ErrMessageIdInvalid
}

// quickCAS is a compare-and-swap on one document. SaveIf reports false when
// prev is no longer the stored bytes.
type quickCAS interface {
	SaveIf(ctx context.Context, key string, prev, next []byte) (bool, error)
}

func (b *Book) readQuick(ctx context.Context, user int64) (*quickDoc, []byte, error) {
	if b.db == nil {
		doc := &quickDoc{}
		if src := b.quick[user]; src != nil {
			raw, err := json.Marshal(src)
			if err != nil {
				return nil, nil, err
			}
			if err := json.Unmarshal(raw, doc); err != nil {
				return nil, nil, err
			}
		}
		return doc, nil, nil
	}
	raw, ok, err := b.db.Load(ctx, quickKey(user))
	if err != nil {
		return nil, nil, err
	}
	doc := &quickDoc{}
	if !ok || len(raw) == 0 {
		return doc, nil, nil
	}
	if err := json.Unmarshal(raw, doc); err != nil {
		return nil, nil, err
	}
	return doc, append([]byte(nil), raw...), nil
}

func (b *Book) commitQuick(ctx context.Context, user int64, prev []byte, doc *quickDoc) (bool, error) {
	if b.db == nil {
		b.quick[user] = doc
		return true, nil
	}
	next, err := json.Marshal(doc)
	if err != nil {
		return false, err
	}
	if cas, ok := b.db.(quickCAS); ok {
		applied, err := cas.SaveIf(ctx, quickKey(user), prev, next)
		if err != nil || !applied {
			return false, err
		}
		b.quick[user] = doc
		return true, nil
	}
	if err := b.save(ctx, quickKey(user), doc); err != nil {
		return false, err
	}
	b.quick[user] = doc
	return true, nil
}

// hashQuickMessages is the messages.getQuickReplyMessages hash.
// TDLib QuickReplyManager::get_quick_reply_messages_hash mixes each message
// id and its edit_date with the pagination vector hash
// (core.telegram.org/api/offsets, MediaDataController.calcHash). The send
// date does not change on edit, so a hash of id and date stayed
// NotModified after the text changed.
//
// Two edits in the same unix second keep the same edit_date, so this hash
// stays valid and a client that missed the push can still be told
// NotModified. That is the client hash. edit_date is not moved forward to
// avoid it, and the hash is not given extra fields.
func hashQuickMessages(msgs []*mtproto.Message) int64 {
	var acc int64
	for _, m := range msgs {
		if m == nil {
			continue
		}
		acc = hashx.CombineInt64Hash2(acc, int64(m.GetId()))
		acc = hashx.CombineInt64Hash2(acc, int64(m.GetEditDate().GetValue()))
	}
	return acc
}

// quickMediaHoldsCaption reports media that can remain when the text is cleared.
// messageMediaEmpty is the same as no media for MESSAGE_EMPTY.
func quickMediaHoldsCaption(media *mtproto.MessageMedia) bool {
	if media == nil {
		return false
	}
	return media.GetPredicateName() != mtproto.Predicate_messageMediaEmpty
}

// DeleteQuickReplyMessages drops messages from a shortcut.
// https://core.telegram.org/method/messages.deleteQuickReplyMessages
func (b *Book) DeleteQuickReplyMessages(ctx context.Context, user int64, shortcutID int32, ids []int32) (*mtproto.Updates, error) {
	if len(ids) == 0 {
		return nil, mtproto.ErrMessageIdsEmpty
	}
	var ups *mtproto.Updates
	err := b.updateQuick(ctx, user, func(doc *quickDoc) error {
		sc := shortcutByID(doc, shortcutID)
		if sc == nil {
			return mtproto.ErrShortcutInvalid
		}
		drop := map[int32]bool{}
		for _, id := range ids {
			if id <= 0 || !quickHas(sc, id) {
				return mtproto.ErrMessageIdInvalid
			}
			drop[id] = true
		}
		kept := make([]quickMsg, 0, len(sc.Messages))
		for _, m := range sc.Messages {
			if !drop[m.ID] {
				kept = append(kept, m)
			}
		}
		sc.Messages = kept
		card := shortcutCard(sc)
		ups = mtproto.MakeUpdatesByUpdates(
			mtproto.MakeTLUpdateNewQuickReply(&mtproto.Update{QuickReply: card}).To_Update(),
			mtproto.MakeTLUpdateDeleteQuickReplyMessages(&mtproto.Update{
				ShortcutId: shortcutID,
				Messages:   append([]int32(nil), ids...),
			}).To_Update(),
		)
		return nil
	})
	return ups, err
}

// PrepareQuickReplySend copies shortcut messages for messages.sendQuickReplyMessages.
// The caller delivers them through messenger.msg.
func (b *Book) PrepareQuickReplySend(ctx context.Context, user int64, peer *mtproto.PeerUtil, shortcutID int32, ids []int32, random []int64) ([]*mtproto.Message, error) {
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(ids) == 0 {
		return nil, mtproto.ErrMessageIdsEmpty
	}
	if len(random) != len(ids) {
		return nil, mtproto.ErrRandomIdInvalid
	}
	seenRand := map[int64]bool{}
	for _, id := range random {
		if id == 0 || seenRand[id] {
			return nil, mtproto.ErrRandomIdInvalid
		}
		seenRand[id] = true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadQuick(ctx, user)
	if err != nil {
		return nil, err
	}
	sc := shortcutByID(doc, shortcutID)
	if sc == nil {
		return nil, mtproto.ErrShortcutInvalid
	}
	out := make([]*mtproto.Message, 0, len(ids))
	for _, id := range ids {
		raw := quickRaw(sc, id)
		if raw == nil {
			return nil, mtproto.ErrMessageIdInvalid
		}
		msg := &mtproto.Message{}
		if err := proto.Unmarshal(raw, msg); err != nil {
			return nil, err
		}
		msg.Id = 0
		msg.QuickReplyShortcutId = nil
		msg.Out = true
		msg.PeerId = peer.ToPeer()
		msg.FromId = mtproto.MakePeerUser(user)
		out = append(out, msg)
	}
	return out, nil
}

// CaptureQuickReply stores messages into a shortcut instead of a chat.
// peer must be the current user, matching the quick-reply send rules.
func (b *Book) CaptureQuickReply(ctx context.Context, user int64, peer *mtproto.PeerUtil, shortcut *mtproto.InputQuickReplyShortcut, msgs []*mtproto.Message) (*mtproto.Updates, error) {
	if shortcut == nil {
		return nil, mtproto.ErrShortcutInvalid
	}
	if peer == nil || !peer.IsSelfUser(user) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(msgs) == 0 {
		return nil, mtproto.ErrMessageEmpty
	}
	var ups *mtproto.Updates
	err := b.updateQuick(ctx, user, func(doc *quickDoc) error {
		idx, err := resolveShortcut(doc, shortcut)
		if err != nil {
			return err
		}
		if idx < 0 {
			if len(doc.Shortcuts) >= maxQuickReplies {
				return mtproto.ErrQuickRepliesTooMuch
			}
			norm, ok := normalizeShortcut(shortcut.GetShortcut())
			if !ok {
				return mtproto.ErrShortcutInvalid
			}
			doc.NextID++
			doc.Shortcuts = append(doc.Shortcuts, quickShortcut{ID: doc.NextID, Name: norm})
			idx = len(doc.Shortcuts) - 1
		}
		sc := &doc.Shortcuts[idx]
		if len(sc.Messages)+len(msgs) > maxQuickReplyMessages {
			return mtproto.ErrQuickRepliesTooMuch
		}
		var updates []*mtproto.Update
		now := int32(b.now().Unix())
		for _, src := range msgs {
			if src == nil {
				return mtproto.ErrMessageEmpty
			}
			if src.GetMessage() == "" && src.GetMedia() == nil {
				return mtproto.ErrMessageEmpty
			}
			doc.NextMsg++
			msg := proto.Clone(src).(*mtproto.Message)
			msg.Id = doc.NextMsg
			msg.Date = now
			msg.Out = true
			msg.QuickReplyShortcutId = wrapperspb.Int32(sc.ID)
			raw, err := proto.Marshal(msg)
			if err != nil {
				return err
			}
			sc.Messages = append(sc.Messages, quickMsg{ID: msg.Id, Raw: raw})
			updates = append(updates, mtproto.MakeTLUpdateQuickReplyMessage(&mtproto.Update{
				Message_MESSAGE: msg,
			}).To_Update())
		}
		card := shortcutCard(sc)
		updates = append([]*mtproto.Update{
			mtproto.MakeTLUpdateNewQuickReply(&mtproto.Update{QuickReply: card}).To_Update(),
		}, updates...)
		ups = mtproto.MakeUpdatesByUpdates(updates...)
		return nil
	})
	return ups, err
}

// updateQuick applies mutate to the quick: document and stores it only when
// the loaded bytes are still current. A lost compare reloads and runs mutate
// again. A mutate error is returned as-is and is not retried. This is the
// only production write path for a quick: key.
func (b *Book) updateQuick(ctx context.Context, user int64, mutate func(*quickDoc) error) error {
	for try := 0; try < 8; try++ {
		b.mu.Lock()
		doc, prev, err := b.readQuick(ctx, user)
		if err != nil {
			b.mu.Unlock()
			return err
		}
		if err = mutate(doc); err != nil {
			b.mu.Unlock()
			return err
		}
		applied, err := b.commitQuick(ctx, user, prev, doc)
		b.mu.Unlock()
		if err != nil {
			return err
		}
		if applied {
			return nil
		}
	}
	return mtproto.ErrInternalServerError
}

func (b *Book) loadQuick(ctx context.Context, user int64) (*quickDoc, error) {
	doc := b.quick[user]
	if doc == nil {
		doc = &quickDoc{}
	}
	if err := b.load(ctx, quickKey(user), doc); err != nil {
		return nil, err
	}
	b.quick[user] = doc
	return doc, nil
}

func resolveShortcut(doc *quickDoc, in *mtproto.InputQuickReplyShortcut) (int, error) {
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputQuickReplyShortcutId:
		idx := indexShortcut(doc, in.GetShortcutId())
		if idx < 0 {
			return -1, mtproto.ErrShortcutInvalid
		}
		return idx, nil
	case mtproto.Predicate_inputQuickReplyShortcut, "":
		norm, ok := normalizeShortcut(in.GetShortcut())
		if !ok {
			return -1, mtproto.ErrShortcutInvalid
		}
		return findShortcut(doc, norm), nil
	default:
		return -1, mtproto.ErrShortcutInvalid
	}
}

func normalizeShortcut(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxQuickReplyNameRunes {
		return "", false
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}
		return "", false
	}
	return strings.ToLower(name), true
}

func findShortcut(doc *quickDoc, norm string) int {
	for i, sc := range doc.Shortcuts {
		if strings.EqualFold(sc.Name, norm) {
			return i
		}
	}
	return -1
}

func indexShortcut(doc *quickDoc, id int32) int {
	for i, sc := range doc.Shortcuts {
		if sc.ID == id {
			return i
		}
	}
	return -1
}

func shortcutByID(doc *quickDoc, id int32) *quickShortcut {
	idx := indexShortcut(doc, id)
	if idx < 0 {
		return nil
	}
	return &doc.Shortcuts[idx]
}

func shortcutView(doc *quickDoc) ([]*mtproto.QuickReply, []*mtproto.Message) {
	replies := make([]*mtproto.QuickReply, 0, len(doc.Shortcuts))
	var tops []*mtproto.Message
	for i := range doc.Shortcuts {
		sc := &doc.Shortcuts[i]
		replies = append(replies, shortcutCard(sc))
		if top := topQuick(sc); top != nil {
			tops = append(tops, top)
		}
	}
	if tops == nil {
		tops = []*mtproto.Message{}
	}
	return replies, tops
}

func shortcutCard(sc *quickShortcut) *mtproto.QuickReply {
	topID := int32(0)
	if top := topQuick(sc); top != nil {
		topID = top.GetId()
	}
	return mtproto.MakeTLQuickReply(&mtproto.QuickReply{
		ShortcutId: sc.ID,
		Shortcut:   sc.Name,
		TopMessage: topID,
		Count:      int32(len(sc.Messages)),
	}).To_QuickReply()
}

func topQuick(sc *quickShortcut) *mtproto.Message {
	if len(sc.Messages) == 0 {
		return nil
	}
	return decodeQuick(sc.Messages[len(sc.Messages)-1].Raw)
}

func filterQuick(sc *quickShortcut, ids []int32) ([]*mtproto.Message, error) {
	if len(ids) == 0 {
		out := make([]*mtproto.Message, 0, len(sc.Messages))
		for _, m := range sc.Messages {
			if msg := decodeQuick(m.Raw); msg != nil {
				out = append(out, msg)
			}
		}
		return out, nil
	}
	out := make([]*mtproto.Message, 0, len(ids))
	for _, id := range ids {
		raw := quickRaw(sc, id)
		if raw == nil {
			return nil, mtproto.ErrMessageIdInvalid
		}
		msg := decodeQuick(raw)
		if msg == nil {
			return nil, mtproto.ErrMessageIdInvalid
		}
		out = append(out, msg)
	}
	return out, nil
}

func quickHas(sc *quickShortcut, id int32) bool { return quickRaw(sc, id) != nil }

func quickRaw(sc *quickShortcut, id int32) []byte {
	for _, m := range sc.Messages {
		if m.ID == id {
			return m.Raw
		}
	}
	return nil
}

func decodeQuick(raw []byte) *mtproto.Message {
	if len(raw) == 0 {
		return nil
	}
	msg := &mtproto.Message{}
	if err := proto.Unmarshal(raw, msg); err != nil {
		return nil
	}
	return msg
}

func hashQuick(replies []*mtproto.QuickReply, tops []*mtproto.Message) int64 {
	var h int64 = 1
	for _, r := range replies {
		h = h*20261 + int64(r.GetShortcutId())
		h = h*20261 + int64(r.GetTopMessage())
		h = h*20261 + int64(r.GetCount())
	}
	for _, m := range tops {
		h = h*20261 + int64(m.GetId())
		// edit_date changes when the top message is edited. id alone stayed
		// NotModified and the client kept the previous preview.
		h = h*20261 + int64(m.GetEditDate().GetValue())
	}
	if h < 0 {
		h = -h
	}
	if h == 0 {
		return 1
	}
	return h
}
