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

// appendPackKey mixes a pack into a list hash. An unfiltered pack contributes
// only its id. A pack that lost items also contributes the visible document
// hash, so clients do not keep a notModified result that still shows them.
// An empty visible pack is omitted.
func appendPackKey(ids []int64, p *Pack) ([]int64, bool) {
	visible := p.visibleItems()
	if len(visible) == 0 {
		return ids, false
	}
	ids = append(ids, p.ID)
	if len(visible) != len(p.Items) {
		ids = append(ids, hash64(documentIDs(visible)))
	}
	return ids, true
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
