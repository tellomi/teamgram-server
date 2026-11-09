package catalog

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"hash/fnv"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/emojiblock"
	"github.com/zeromicro/go-zero/core/logx"
)

// DefaultReactionEmoji is the double-tap reaction. help.getConfig
// reactions_default is this emoji. messages.getAvailableReactions includes
// it unless the emoji blocklist removes it.
const DefaultReactionEmoji = "\U0001F44D"

// requiredReactions is the set official clients expect in the picker.
// 👍 is first because it is the default quick reaction.
var requiredReactions = []struct {
	code  string
	title string
}{
	{"1f44d", "Thumbs up"},
	{"2764", "Red heart"},
	{"1f525", "Fire"},
	{"1f970", "Smiling face with hearts"},
	{"1f44f", "Clapping hands"},
	{"1f601", "Beaming face with smiling eyes"},
	{"1f914", "Thinking face"},
	{"1f622", "Crying face"},
	{"1f389", "Party popper"},
	{"1f64f", "Folded hands"},
	{"1f44e", "Thumbs down"},
	{"1f631", "Face screaming in fear"},
}

var (
	fallbackOnce     sync.Once
	fallbackTGSBytes []byte
)

// fallbackTGS is a 512×512 Lottie with one empty shape layer. iOS accepts it
// as application/x-tgsticker when a real Noto file is missing.
func fallbackTGS() []byte {
	fallbackOnce.Do(func() {
		raw := []byte(`{"v":"5.5.2","fr":60,"ip":0,"op":30,"w":512,"h":512,"nm":"fallback","ddd":0,"assets":[],"layers":[{"ddd":0,"ind":1,"ty":4,"nm":"s","sr":1,"ks":{"o":{"a":0,"k":100},"r":{"a":0,"k":0},"p":{"a":0,"k":[256,256,0]},"a":{"a":0,"k":[0,0,0]},"s":{"a":0,"k":[100,100,100]}},"ao":0,"shapes":[],"ip":0,"op":30,"st":0,"bm":0}],"markers":[]}`)
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		_, _ = w.Write(raw)
		_ = w.Close()
		fallbackTGSBytes = buf.Bytes()
	})
	return fallbackTGSBytes
}

// reactionSlots are the documents on availableReaction. static_icon is a
// full-bleed WebP. The other six are gzipped Lottie. center is the pill
// icon: the same Lottie, inset so the glyph fills the center half of the
// canvas. iOS draws that document at twice the pill slot.
var reactionSlots = []struct {
	name string
	tgs  bool
}{
	{"static", false},
	{"appear", true},
	{"select", true},
	{"activate", true},
	{"effect", true},
	{"around", true},
	{"center", true},
}

func reactionReady(code string) bool {
	_, ok := reactionAssets(code)
	return ok
}

func reactionSlotID(code, slot string) int64 {
	if slot == "center" {
		// New id so a client that cached the full-bleed center icon
		// downloads this inset file. The previous id is still served.
		return stableID("reaction-center-inset", code)
	}
	return stableID("reaction", slot, code)
}

// legacyCenterID is the center_icon id from before the pill inset.
// upload.getFile still returns the inset bytes for it.
func legacyCenterID(code string) int64 {
	return stableID("reaction", "center", code)
}

type reactionFiles struct {
	webp  []byte
	slots map[string][]byte
}

var reactionFileCache sync.Map

// reactionAssets is the static WebP and one Lottie per slot. ok is false
// when either file is missing: those reactions are left out of the picker
// instead of showing a blank animation or the thumbs-up icon.
func reactionAssets(code string) (reactionFiles, bool) {
	if v, ok := reactionFileCache.Load(code); ok {
		files := v.(reactionFiles)
		return files, len(files.webp) > 0 && len(files.slots) == 6
	}
	var files reactionFiles
	webp, ok := readAsset("assets/twemoji/" + code + ".webp")
	base, okTGS := readAsset("assets/noto/" + code + ".tgs")
	if !ok || !okTGS || !tgsCanvas512(base) {
		reactionFileCache.Store(code, files)
		return files, false
	}
	files.webp = webp
	files.slots = map[string][]byte{}
	for _, slot := range reactionSlots {
		if !slot.tgs {
			continue
		}
		body, ok := varyTGS(base, slot.name)
		if !ok {
			files.slots = nil
			reactionFileCache.Store(code, reactionFiles{})
			return reactionFiles{}, false
		}
		files.slots[slot.name] = body
	}
	reactionFileCache.Store(code, files)
	return files, true
}

func reactionSlotBytes(code, slot string, tgs bool) (body []byte, mime string) {
	files, ok := reactionAssets(code)
	if !ok {
		return nil, ""
	}
	if tgs {
		return files.slots[slot], "application/x-tgsticker"
	}
	return files.webp, "image/webp"
}

func reactionSpecs() []struct {
	code  string
	title string
} {
	out := append([]struct {
		code  string
		title string
	}{}, requiredReactions...)
	seen := map[string]bool{}
	for _, spec := range out {
		seen[emojiFromCode(spec.code)] = true
	}
	for _, def := range emojiCatalog {
		if def.Reaction == "" {
			continue
		}
		alt := emojiFromCode(def.Code)
		if alt == "" || seen[alt] {
			continue
		}
		seen[alt] = true
		out = append(out, struct {
			code  string
			title string
		}{def.Code, def.Reaction})
	}
	return out
}

// Reactions is messages.getAvailableReactions. Every entry has its own
// static icon and a distinct appear/select/activate/effect/around/center
// animation. A code without those files is omitted.
func (s *Service) Reactions(hash int32) *mtproto.Messages_AvailableReactions {
	var reactions []*mtproto.AvailableReaction
	var ids []int64
	for _, spec := range reactionSpecs() {
		alt := emojiFromCode(spec.code)
		if emojiblock.Blocked(alt) {
			continue
		}
		docs := map[string]*mtproto.Document{}
		okDocs := true
		for _, slot := range reactionSlots {
			body, mime := reactionSlotBytes(spec.code, slot.name, slot.tgs)
			if len(body) == 0 || (slot.tgs && !tgsCanvas512(body)) {
				okDocs = false
				break
			}
			w, h := int32(72), int32(72)
			name := spec.code + ".webp"
			if slot.tgs {
				w, h = 512, 512
				name = spec.code + "-" + slot.name + ".tgs"
			}
			docs[slot.name] = reactionIconDocument(reactionSlotID(spec.code, slot.name), mime, name, alt, w, h, len(body))
		}
		if !okDocs {
			continue
		}
		reactions = append(reactions, mtproto.MakeTLAvailableReaction(&mtproto.AvailableReaction{
			Reaction:          alt,
			Title:             spec.title,
			StaticIcon:        docs["static"],
			AppearAnimation:   docs["appear"],
			SelectAnimation:   docs["select"],
			ActivateAnimation: docs["activate"],
			EffectAnimation:   docs["effect"],
			AroundAnimation:   docs["around"],
			CenterIcon:        docs["center"],
		}).To_AvailableReaction())
		// The center id is part of the hash. Changing the pill file has to
		// miss messages.availableReactionsNotModified, or the client keeps
		// drawing the previous document.
		ids = append(ids, reactionSlotID(spec.code, "activate"), reactionSlotID(spec.code, "center"))
	}
	sum := int32(hash64(ids))
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesAvailableReactionsNotModified(&mtproto.Messages_AvailableReactions{}).To_Messages_AvailableReactions()
	}
	if reactions == nil {
		reactions = []*mtproto.AvailableReaction{}
	}
	return mtproto.MakeTLMessagesAvailableReactions(&mtproto.Messages_AvailableReactions{
		Hash:      sum,
		Reactions: reactions,
	}).To_Messages_AvailableReactions()
}

// DocumentFile returns the bytes for a seeded sticker, emoji, or reaction icon.
// upload.getFile serves these directly so a missing R2 object cannot stall the session.
// Reaction ids are resolved before the sticker map: a hash collision must not
// hand the client a different sticker, which it then draws beside the emoji.
func (s *Service) DocumentFile(id int64) (body []byte, mime string, ok bool) {
	if body, mime, ok = reactionFile(id); ok {
		return body, mime, true
	}
	if body, mime, ok = GifFile(id); ok {
		return body, mime, true
	}
	s.mu.Lock()
	link := s.docs[id]
	s.mu.Unlock()
	if link != nil && link.item != nil {
		b, err := assetFS.ReadFile(link.item.AssetPath)
		if err != nil || len(b) == 0 {
			return nil, "", false
		}
		return b, link.item.MIME, true
	}
	return nil, "", false
}

func reactionFile(id int64) ([]byte, string, bool) {
	for _, spec := range reactionSpecs() {
		for _, slot := range reactionSlots {
			if id != reactionSlotID(spec.code, slot.name) && !legacyCenterHit(spec.code, slot.name, id) {
				continue
			}
			body, mime := reactionSlotBytes(spec.code, slot.name, slot.tgs)
			if len(body) == 0 || (slot.tgs && !tgsCanvas512(body)) {
				return nil, "", false
			}
			return body, mime, true
		}
		// messages.getAvailableEffects used to point at this id. Keep serving
		// the same Lottie so a client that cached the old document can download it.
		if id == stableID("reaction", "anim", spec.code) {
			body, mime := reactionSlotBytes(spec.code, "effect", true)
			if len(body) == 0 || !tgsCanvas512(body) {
				return nil, "", false
			}
			return body, mime, true
		}
	}
	return nil, "", false
}

// reactionIconDocument is a sticker document with no custom-emoji attribute.
// iOS draws a custom-emoji document that it cannot decode as a coin beside the emoji.
func reactionIconDocument(id int64, mime, name, alt string, w, h int32, size int) *mtproto.Document {
	set := mtproto.MakeTLInputStickerSetEmpty(&mtproto.InputStickerSet{}).To_InputStickerSet()
	attrs := []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeImageSize(&mtproto.DocumentAttribute{
			W: w,
			H: h,
		}).To_DocumentAttribute(),
		mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{
			FileName: name,
		}).To_DocumentAttribute(),
		mtproto.MakeTLDocumentAttributeSticker(&mtproto.DocumentAttribute{
			Alt:        alt,
			Stickerset: set,
		}).To_DocumentAttribute(),
	}
	return mtproto.MakeTLDocument(&mtproto.Document{
		Id:            id,
		AccessHash:    stableID("reaction-access", mime, name),
		FileReference: reactionFileReference(id),
		Date:          stickerDate,
		MimeType:      mime,
		Size2_INT32:   int32(size),
		Size2_INT64:   int64(size),
		Size2:         int64(size),
		DcId:          1,
		Attributes:    attrs,
	}).To_Document()
}

func legacyCenterHit(code, slot string, id int64) bool {
	return slot == "center" && id == legacyCenterID(code)
}

func reactionFileReference(id int64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(id))
	return buf[:]
}

const maxRecentReactions = 50

type reactionStore interface {
	saveReactionUse(emoticon string, count int64) error
	saveReactionRecent(userID int64, emoticon string, date int32) error
	clearReactionRecent(userID int64) error
}

// defaultReactionOrder is the available reaction list in catalog order.
// Top reactions fall back to this when nothing has been used yet.
func defaultReactionOrder() []string {
	var out []string
	seen := map[string]bool{}
	for _, def := range emojiCatalog {
		if def.Reaction == "" {
			continue
		}
		emo := emojiFromCode(def.Code)
		if emo == "" || seen[emo] || !reactionReady(def.Code) {
			continue
		}
		seen[emo] = true
		out = append(out, emo)
	}
	return out
}

// TopReactions returns available reactions, most used first.
// Equal counts keep the default catalog order. hash 0 always returns the list.
func (s *Service) TopReactions(limit int32, hash int64) *mtproto.Messages_Reactions {
	s.mu.Lock()
	defer s.mu.Unlock()
	order := defaultReactionOrder()
	index := make(map[string]int, len(order))
	for i, emo := range order {
		index[emo] = i
	}
	for i := 1; i < len(order); i++ {
		j := i
		for j > 0 {
			left, right := order[j-1], order[j]
			cl, cr := s.reactionUses[left], s.reactionUses[right]
			if cl > cr || (cl == cr && index[left] <= index[right]) {
				break
			}
			order[j-1], order[j] = order[j], order[j-1]
			j--
		}
	}
	return reactionPage(order, limit, hash)
}

// RecentReactions returns the emojis this user has sent, newest first.
func (s *Service) RecentReactions(userID int64, limit int32, hash int64) *mtproto.Messages_Reactions {
	s.mu.Lock()
	defer s.mu.Unlock()
	var emos []string
	if st := s.users[userID]; st != nil {
		emos = append(emos, st.reactionRecent...)
	}
	return reactionPage(emos, limit, hash)
}

// NoteReaction records emojis the user just sent and bumps the global counts.
func (s *Service) NoteReaction(userID int64, emoticons []string) {
	if userID == 0 || len(emoticons) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reactionUses == nil {
		s.reactionUses = map[string]int64{}
	}
	st := s.state(userID)
	date := s.now()
	var store reactionStore
	if s.persist != nil {
		store, _ = s.persist.(reactionStore)
	}
	for _, emo := range emoticons {
		if emo == "" {
			continue
		}
		s.reactionUses[emo]++
		if store != nil {
			if err := store.saveReactionUse(emo, s.reactionUses[emo]); err != nil {
				logx.Errorf("reaction count %s: %v", emo, err)
			}
		}
		next := make([]string, 0, len(st.reactionRecent)+1)
		next = append(next, emo)
		for _, have := range st.reactionRecent {
			if have != emo {
				next = append(next, have)
			}
		}
		st.reactionRecent = next
		if len(st.reactionRecent) > maxRecentReactions {
			st.reactionRecent = st.reactionRecent[:maxRecentReactions]
		}
		if store != nil {
			if err := store.saveReactionRecent(userID, emo, date); err != nil {
				logx.Errorf("reaction recent %d %s: %v", userID, emo, err)
			}
		}
	}
}

// ClearRecentReactions drops the user's recent reaction list.
func (s *Service) ClearRecentReactions(userID int64) {
	if userID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.users[userID]; st != nil {
		st.reactionRecent = nil
	}
	if store, ok := s.persist.(reactionStore); ok {
		if err := store.clearReactionRecent(userID); err != nil {
			logx.Errorf("reaction recent clear %d: %v", userID, err)
		}
	}
}

func reactionPage(emos []string, limit int32, hash int64) *mtproto.Messages_Reactions {
	emos = withoutBlocked(emos)
	if limit > 0 && int(limit) < len(emos) {
		emos = emos[:limit]
	}
	sum := reactionHash(emos)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesReactionsNotModified(&mtproto.Messages_Reactions{}).To_Messages_Reactions()
	}
	out := make([]*mtproto.Reaction, 0, len(emos))
	for _, emo := range emos {
		out = append(out, mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: emo}).To_Reaction())
	}
	return mtproto.MakeTLMessagesReactions(&mtproto.Messages_Reactions{
		Hash:      sum,
		Reactions: out,
	}).To_Messages_Reactions()
}

func reactionHash(emos []string) int64 {
	h := fnv.New64a()
	for _, emo := range emos {
		_, _ = h.Write([]byte(emo))
		_, _ = h.Write([]byte{0})
	}
	out := int64(h.Sum64() & 0x7fffffffffffffff)
	if out == 0 {
		return 1
	}
	return out
}
