package chatbox

import (
	"context"
	"sort"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Saved-message reaction tags.
// https://core.telegram.org/method/messages.getSavedReactionTags
// https://core.telegram.org/method/messages.updateSavedReactionTag
//
// Counts come from reactions on the user's Saved Messages dialog.
// updateSavedReactionTag sets or clears the title shown in the tag bar.
// A missing title flag clears the title. A tag with a title and a zero
// count is still returned.

const maxTagTitleRunes = 64

type tagDoc struct {
	Counts map[string]int32  `json:"counts,omitempty"`
	Titles map[string]string `json:"titles,omitempty"`
	Kind   map[string]string `json:"kind,omitempty"` // emoji text or "doc"
	Doc    map[string]int64  `json:"doc,omitempty"`
}

func tagStoreKey(user int64) string { return "tags:" + userKey(user) }

func savedDialog(self int64, peer *mtproto.PeerUtil) bool {
	if peer == nil || self == 0 {
		return false
	}
	if peer.PeerType == mtproto.PEER_SELF {
		return true
	}
	return peer.PeerType == mtproto.PEER_USER && peer.PeerId == self
}

// SyncSavedTags rebuilds tag counts from the saved-messages reaction list.
// Caller holds b.mu.
func (b *Book) syncSavedTagsLocked(ctx context.Context, self int64, doc *reactDoc) {
	if doc == nil {
		return
	}
	tags := b.tagDocLocked(ctx, self)
	counts := map[string]int32{}
	if tags.Kind == nil {
		tags.Kind = map[string]string{}
	}
	if tags.Doc == nil {
		tags.Doc = map[string]int64{}
	}
	for _, item := range doc.Items {
		if item.UserID != self {
			continue
		}
		key, kind, id := tagIdentity(item.Emoticon, item.DocumentID)
		if key == "" {
			continue
		}
		counts[key]++
		tags.Kind[key] = kind
		if id != 0 {
			tags.Doc[key] = id
		}
	}
	tags.Counts = counts
	b.tags[self] = tags
	_ = b.save(ctx, tagStoreKey(self), tags)
}

// SavedReactionTags is messages.getSavedReactionTags.
// peer, when set, limits the list to that saved dialog. Tags are only
// counted on Saved Messages, so any other peer returns an empty list.
func (b *Book) SavedReactionTags(ctx context.Context, self int64, peer *mtproto.PeerUtil, hash int64) (*mtproto.Messages_SavedReactionTags, error) {
	if peer != nil && peer.PeerType != mtproto.PEER_EMPTY && peer.PeerType != mtproto.PEER_UNKNOWN && !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if peer != nil && peer.IsUserOrChatOrChannel() && !savedDialog(self, peer) {
		return savedTagsResult(nil, hash), nil
	}
	tags := b.tagDocLocked(ctx, self)
	list := materializeTags(tags)
	return savedTagsResult(list, hash), nil
}

// UpdateSavedReactionTag sets or clears a tag title.
// A nil title clears it. https://core.telegram.org/method/messages.updateSavedReactionTag
func (b *Book) UpdateSavedReactionTag(ctx context.Context, self int64, reaction *mtproto.Reaction, title *wrapperspb.StringValue) error {
	emo, docID, err := ParseReaction(reaction)
	if err != nil {
		return err
	}
	var label string
	if title != nil {
		label = title.GetValue()
		if len([]rune(label)) > maxTagTitleRunes {
			return mtproto.ErrMessageTooLong
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	tags := b.tagDocLocked(ctx, self)
	key, kind, id := tagIdentity(emo, docID)
	if tags.Kind == nil {
		tags.Kind = map[string]string{}
	}
	if tags.Doc == nil {
		tags.Doc = map[string]int64{}
	}
	if tags.Titles == nil {
		tags.Titles = map[string]string{}
	}
	tags.Kind[key] = kind
	if id != 0 {
		tags.Doc[key] = id
	}
	if label == "" {
		delete(tags.Titles, key)
	} else {
		tags.Titles[key] = label
	}
	b.tags[self] = tags
	return b.save(ctx, tagStoreKey(self), tags)
}

func (b *Book) tagDocLocked(ctx context.Context, self int64) *tagDoc {
	doc := b.tags[self]
	if doc == nil {
		doc = &tagDoc{}
	}
	_ = b.load(ctx, tagStoreKey(self), doc)
	if doc.Counts == nil {
		doc.Counts = map[string]int32{}
	}
	if doc.Titles == nil {
		doc.Titles = map[string]string{}
	}
	b.tags[self] = doc
	return doc
}

type tagRow struct {
	key   string
	title string
	count int32
	emo   string
	doc   int64
}

func materializeTags(doc *tagDoc) []tagRow {
	seen := map[string]bool{}
	var rows []tagRow
	for key, count := range doc.Counts {
		seen[key] = true
		rows = append(rows, tagRow{
			key: key, title: doc.Titles[key], count: count,
			emo: doc.Kind[key], doc: doc.Doc[key],
		})
	}
	for key, title := range doc.Titles {
		if seen[key] || title == "" {
			continue
		}
		rows = append(rows, tagRow{
			key: key, title: title, emo: doc.Kind[key], doc: doc.Doc[key],
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count != rows[j].count {
			return rows[i].count > rows[j].count
		}
		return rows[i].key < rows[j].key
	})
	return rows
}

func savedTagsResult(rows []tagRow, hash int64) *mtproto.Messages_SavedReactionTags {
	sum := hashTagRows(rows)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesSavedReactionTagsNotModified(&mtproto.Messages_SavedReactionTags{}).To_Messages_SavedReactionTags()
	}
	tags := make([]*mtproto.SavedReactionTag, 0, len(rows))
	for _, row := range rows {
		var reaction *mtproto.Reaction
		if row.doc != 0 && row.emo != "emoji" {
			reaction = mtproto.MakeTLReactionCustomEmoji(&mtproto.Reaction{DocumentId: row.doc}).To_Reaction()
		} else {
			reaction = mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: row.emo}).To_Reaction()
		}
		tag := &mtproto.SavedReactionTag{
			Reaction: reaction,
			Count:    row.count,
		}
		if row.title != "" {
			tag.Title = wrapperspb.String(row.title)
		}
		tags = append(tags, mtproto.MakeTLSavedReactionTag(tag).To_SavedReactionTag())
	}
	return mtproto.MakeTLMessagesSavedReactionTags(&mtproto.Messages_SavedReactionTags{
		Tags: tags,
		Hash: sum,
	}).To_Messages_SavedReactionTags()
}

func hashTagRows(rows []tagRow) int64 {
	var h int64 = 1
	for _, row := range rows {
		h = h*20261 + int64(row.count)
		h = h*20261 + int64(len(row.title))
		for _, r := range row.key {
			h = h*20261 + int64(r)
		}
	}
	if h < 0 {
		h = -h
	}
	if h == 0 {
		return 1
	}
	return h
}

func tagIdentity(emo string, doc int64) (key, kind string, id int64) {
	if doc != 0 {
		return "doc:" + strconv.FormatInt(doc, 10), "doc", doc
	}
	if emo == "" {
		return "", "", 0
	}
	return "emoji:" + emo, emo, 0
}
