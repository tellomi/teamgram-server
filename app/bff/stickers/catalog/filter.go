package catalog

import "github.com/teamgram/teamgram-server/pkg/emojiblock"

// visibleItems is the pack with blocklisted emoji removed. When nothing is
// blocked the original slice is returned, so sticker-set hashes stay put.
func (p *Pack) visibleItems() []*Item {
	if p == nil || len(p.Items) == 0 {
		return nil
	}
	blocked := 0
	for _, item := range p.Items {
		if itemBlocked(item) {
			blocked++
		}
	}
	if blocked == 0 {
		return p.Items
	}
	out := make([]*Item, 0, len(p.Items)-blocked)
	for _, item := range p.Items {
		if !itemBlocked(item) {
			out = append(out, item)
		}
	}
	return out
}

func itemBlocked(item *Item) bool {
	return item == nil || emojiblock.Blocked(item.Emoticon)
}

func documentIDs(items []*Item) []int64 {
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ids = append(ids, item.DocumentID)
	}
	return ids
}

// appendPackKey mixes a pack into the messages.searchStickerSets hash.
// The pack id, the visible document ids, and the title all contribute.
// An empty visible pack is omitted. Installed-list hashes do not use this;
// official clients hash each set's 32-bit StickerSet.hash instead.
func appendPackKey(ids []int64, p *Pack) ([]int64, bool) {
	visible := p.visibleItems()
	if len(visible) == 0 {
		return ids, false
	}
	return append(ids, p.ID, hash64(documentIDs(visible)), titleKey(p.Title)), true
}

func titleKey(title string) int64 {
	return stableID("sticker-title", title)
}

func withoutBlocked(emos []string) []string {
	n := 0
	for _, emo := range emos {
		if emojiblock.Blocked(emo) {
			n++
		}
	}
	if n == 0 {
		return emos
	}
	out := make([]string, 0, len(emos)-n)
	for _, emo := range emos {
		if emojiblock.Blocked(emo) {
			continue
		}
		out = append(out, emo)
	}
	return out
}

// emojiKeywordVersion stays 1 for the built-in blocklist. Extra entries
// change it so clients refetch keywords instead of keeping a cached list.
func emojiKeywordVersion() int32 {
	if emojiblock.Fingerprint() == emojiblock.BuiltinFingerprint() {
		return 1
	}
	v := int32(emojiblock.Fingerprint() & 0x7fffffff)
	if v <= 1 {
		return 2
	}
	return v
}
