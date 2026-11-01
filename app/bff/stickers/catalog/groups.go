package catalog

import (
	"encoding/binary"
	"hash/fnv"

	"github.com/teamgram/proto/mtproto"
)

// Group kinds for messages.getEmoji*Groups.
// https://core.telegram.org/method/messages.getEmojiGroups
// https://core.telegram.org/method/messages.getEmojiStatusGroups
// https://core.telegram.org/method/messages.getEmojiProfilePhotoGroups
// https://core.telegram.org/method/messages.getEmojiStickerGroups
const (
	GroupEmoji   = "emoji"
	GroupStatus  = "status"
	GroupProfile = "profile"
	GroupSticker = "sticker"
)

type emojiCategory struct {
	title string
	codes []string
}

// Categories follow the emoji panel the official clients draw. Each code is
// a custom emoji in the built-in Noto pack, so icon_emoji_id is a document
// messages.getCustomEmojiDocuments can return.
var emojiCategories = []emojiCategory{
	{"Smileys", []string{"1f600", "1f602", "1f60d", "1f618", "1f60e", "1f622", "1f621"}},
	{"People", []string{"1f44d", "1f44e", "1f64f"}},
	{"Nature", []string{"1f436", "1f431", "1f308"}},
	{"Food", []string{"1f355", "2615"}},
	{"Activities", []string{"1f525", "1f389", "1f680"}},
	{"Objects", []string{"1f4a1", "1f381"}},
	{"Symbols", []string{"2764", "2705", "274c", "1f31f"}},
}

var profileCategories = []emojiCategory{
	{"Smileys", []string{"1f600", "1f602", "1f60d", "1f618", "1f60e", "1f622", "1f621"}},
	{"People", []string{"1f44d", "1f44e", "1f64f"}},
}

var groupPhotoCategories = []emojiCategory{
	{"Nature", []string{"1f436", "1f431", "1f308"}},
	{"Food", []string{"1f355", "2615"}},
	{"Activities", []string{"1f525", "1f389", "1f680"}},
	{"Objects", []string{"1f4a1", "1f381"}},
	{"Symbols", []string{"2764", "2705", "274c", "1f31f"}},
}

// EmojiGroups implements messages.getEmoji*Groups from the built-in catalog.
func (s *Service) EmojiGroups(kind string, hash int32) *mtproto.Messages_EmojiGroups {
	s.mu.Lock()
	defer s.mu.Unlock()
	groups := s.emojiGroupsLocked(kind)
	sum := emojiGroupHash(kind, groups)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesEmojiGroupsNotModified(&mtproto.Messages_EmojiGroups{}).To_Messages_EmojiGroups()
	}
	return mtproto.MakeTLMessagesEmojiGroups(&mtproto.Messages_EmojiGroups{
		Hash:   sum,
		Groups: groups,
	}).To_Messages_EmojiGroups()
}

// PhotoEmojis implements account.getDefaultProfilePhotoEmojis (group=false)
// and account.getDefaultGroupPhotoEmojis (group=true).
// https://core.telegram.org/method/account.getDefaultProfilePhotoEmojis
// https://core.telegram.org/method/account.getDefaultGroupPhotoEmojis
func (s *Service) PhotoEmojis(group bool, hash int64) *mtproto.EmojiList {
	s.mu.Lock()
	defer s.mu.Unlock()
	cats := profileCategories
	if group {
		cats = groupPhotoCategories
	}
	var ids []int64
	for _, cat := range cats {
		for _, code := range cat.codes {
			if id, ok := s.notoIDLocked(code); ok {
				ids = append(ids, id)
			}
		}
	}
	sum := hash64(ids)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLEmojiListNotModified(&mtproto.EmojiList{}).To_EmojiList()
	}
	if ids == nil {
		ids = []int64{}
	}
	return mtproto.MakeTLEmojiList(&mtproto.EmojiList{
		Hash:       sum,
		DocumentId: ids,
	}).To_EmojiList()
}

func (s *Service) emojiGroupsLocked(kind string) []*mtproto.EmojiGroup {
	cats := emojiCategories
	if kind == GroupProfile {
		cats = profileCategories
	}
	var out []*mtproto.EmojiGroup
	for _, cat := range cats {
		var emos []string
		var icon int64
		for _, code := range cat.codes {
			if !s.includeCodeLocked(kind, code) {
				continue
			}
			id, ok := s.notoIDLocked(code)
			if !ok {
				continue
			}
			if icon == 0 {
				icon = id
			}
			emos = append(emos, emojiFromCode(code))
		}
		if len(emos) == 0 || icon == 0 {
			continue
		}
		out = append(out, mtproto.MakeTLEmojiGroup(&mtproto.EmojiGroup{
			Title:       cat.title,
			IconEmojiId: icon,
			Emoticons:   emos,
		}).To_EmojiGroup())
	}
	if out == nil {
		out = []*mtproto.EmojiGroup{}
	}
	return out
}

func (s *Service) includeCodeLocked(kind, code string) bool {
	if _, ok := s.notoIDLocked(code); !ok {
		return false
	}
	emo := emojiFromCode(code)
	switch kind {
	case GroupStatus:
		id, _ := s.notoIDLocked(code)
		return s.packHasLocked(KindDefaultStatuses, id)
	case GroupSticker:
		return s.docByFamily(emo, "twemoji") != nil || s.docByFamily(emo, "fluent") != nil
	default:
		return true
	}
}

func (s *Service) notoIDLocked(code string) (int64, bool) {
	link := s.docByFamily(emojiFromCode(code), "noto")
	if link == nil || link.item == nil || link.item.DocumentID == 0 {
		return 0, false
	}
	return link.item.DocumentID, true
}

func (s *Service) packHasLocked(kind string, docID int64) bool {
	for _, p := range s.packs {
		if p.Kind != kind {
			continue
		}
		for _, it := range p.Items {
			if it.DocumentID == docID {
				return true
			}
		}
	}
	return false
}

func emojiGroupHash(kind string, groups []*mtproto.EmojiGroup) int32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(kind))
	var buf [8]byte
	for _, g := range groups {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(g.GetTitle()))
		binary.LittleEndian.PutUint64(buf[:], uint64(g.GetIconEmojiId()))
		_, _ = h.Write(buf[:])
		for _, emo := range g.GetEmoticons() {
			_, _ = h.Write([]byte(emo))
			_, _ = h.Write([]byte{0})
		}
	}
	v := int32(h.Sum32() & 0x7fffffff)
	if v == 0 {
		return 1
	}
	return v
}
