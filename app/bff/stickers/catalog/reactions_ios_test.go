package catalog

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// The emoji the iOS picker and double-tap must always be able to use.
var iosDefaultReactions = []string{
	"👍", "❤", "🔥", "🥰", "👏", "😁", "🤔", "😢", "🎉", "🙏", "👎", "😱",
}

func TestDefaultReactionsAlwaysAvailable(t *testing.T) {
	if DefaultReactionEmoji != "👍" {
		t.Fatalf("default reaction %q", DefaultReactionEmoji)
	}
	s := newService(builtinPacks())
	msg := s.Reactions(0)
	if msg.GetPredicateName() != mtproto.Predicate_messages_availableReactions {
		t.Fatalf("predicate %s", msg.GetPredicateName())
	}
	got := map[string]*mtproto.AvailableReaction{}
	for _, r := range msg.GetReactions() {
		got[r.GetReaction()] = r
	}
	if _, ok := got[DefaultReactionEmoji]; !ok {
		t.Fatal("default 👍 is missing")
	}
	if msg.GetReactions()[0].GetReaction() != DefaultReactionEmoji {
		t.Fatalf("first reaction %q, want 👍", msg.GetReactions()[0].GetReaction())
	}
	for _, emo := range iosDefaultReactions {
		r := got[emo]
		if r == nil {
			t.Fatalf("missing %s", emo)
		}
		if r.GetPremium() || r.GetInactive() {
			t.Fatalf("%s is premium or inactive", emo)
		}
		assertIOSReactionDocuments(t, emo, r)
		body, mime, ok := s.DocumentFile(r.GetAppearAnimation().GetId())
		if !ok || mime != "application/x-tgsticker" || !tgsCanvas512(body) {
			t.Fatalf("%s animation is not a downloadable 512 tgs", emo)
		}
		icon, iconMIME, ok := s.DocumentFile(r.GetStaticIcon().GetId())
		if !ok || iconMIME != "image/webp" || len(icon) == 0 {
			t.Fatalf("%s static icon is not a downloadable webp", emo)
		}
	}
	for _, emo := range iosDefaultReactions {
		r := got[emo]
		body, _, ok := s.DocumentFile(r.GetActivateAnimation().GetId())
		if !ok || bytes.Equal(body, fallbackTGS()) {
			t.Fatalf("%s activate animation is the empty fallback", emo)
		}
		center, mime, ok := s.DocumentFile(r.GetCenterIcon().GetId())
		if !ok || mime != "application/x-tgsticker" || !tgsCanvas512(center) || bytes.Equal(center, fallbackTGS()) {
			t.Fatalf("%s center icon is not a real tgs", emo)
		}
		if r.GetAppearAnimation().GetId() == r.GetSelectAnimation().GetId() || r.GetStaticIcon().GetId() == r.GetCenterIcon().GetId() {
			t.Fatalf("%s reuses one document id for different slots", emo)
		}
	}
}

// TestIOSParsesAvailableReactions walks the same fields TelegramCore reads.
// A nil document, or a document that is not a sticker file, makes the client
// drop the whole available-reactions vector and the picker stays empty.
func TestIOSParsesAvailableReactions(t *testing.T) {
	s := newService(builtinPacks())
	msg := s.Reactions(0)
	buf := mtproto.NewEncodeBuf(1 << 20)
	if err := msg.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	dbuf := mtproto.NewDecodeBuf(buf.GetBuf())
	obj := dbuf.Object()
	if err := dbuf.GetError(); err != nil {
		t.Fatal(err)
	}
	parsed, ok := obj.(*mtproto.TLMessagesAvailableReactions)
	if !ok || parsed == nil || parsed.To_Messages_AvailableReactions() == nil {
		t.Fatalf("decoded %T", obj)
	}
	reactions := parsed.To_Messages_AvailableReactions().GetReactions()
	if len(reactions) < len(iosDefaultReactions) {
		t.Fatalf("decoded %d reactions", len(reactions))
	}
	for _, r := range reactions {
		assertIOSReactionDocuments(t, r.GetReaction(), r)
	}
	if !tgsCanvas512(fallbackTGS()) {
		t.Fatal("fallback tgs is not 512x512")
	}
}

// TestCenterIconFitsReactionPill checks the document the official iOS pill
// draws. ReactionIconView scales a builtin center_icon to twice the 20pt
// slot and does not clip, so the glyph has to occupy the center half of
// the 512 canvas. static_icon stays the full-bleed WebP chip. The list
// hash includes the center document id, and the previous id still
// downloads the inset file.
func TestCenterIconFitsReactionPill(t *testing.T) {
	s := newService(builtinPacks())
	msg := s.Reactions(0)
	got := map[string]*mtproto.AvailableReaction{}
	var activateAndCenter []int64
	var activateOnly []int64
	for _, r := range msg.GetReactions() {
		got[r.GetReaction()] = r
		activateAndCenter = append(activateAndCenter, r.GetActivateAnimation().GetId(), r.GetCenterIcon().GetId())
		activateOnly = append(activateOnly, r.GetActivateAnimation().GetId())
	}
	if int32(hash64(activateAndCenter)) != msg.GetHash() {
		t.Fatal("available reactions hash does not cover center_icon")
	}
	if int32(hash64(activateOnly)) == msg.GetHash() {
		t.Fatal("hash ignores center_icon, so clients keep the old pill file")
	}
	for _, emo := range iosDefaultReactions {
		r := got[emo]
		if r == nil {
			t.Fatalf("missing %s", emo)
		}
		center := r.GetCenterIcon()
		static := r.GetStaticIcon()
		if center.GetId() == static.GetId() || center.GetId() == r.GetAppearAnimation().GetId() {
			t.Fatalf("%s center_icon reuses another slot", emo)
		}
		if center.GetMimeType() != "application/x-tgsticker" {
			t.Fatalf("%s center mime %s", emo, center.GetMimeType())
		}
		if static.GetMimeType() != "image/webp" {
			t.Fatalf("%s static mime %s", emo, static.GetMimeType())
		}
		assertAttrSize(t, emo, "center_icon", center, 512, 512)
		assertAttrSize(t, emo, "static_icon", static, 72, 72)
		if hasCustomEmoji(center) || hasCustomEmoji(static) {
			t.Fatalf("%s reaction icon is a custom emoji", emo)
		}
		body, mime, ok := s.DocumentFile(center.GetId())
		if !ok || mime != "application/x-tgsticker" || !tgsCanvas512(body) {
			t.Fatalf("%s center file", emo)
		}
		scale, ax, ay, px, py, name := lottieRoot(t, body)
		if name != "pill-inset" || scale != centerIconScale || ax != 256 || ay != 256 || px != 256 || py != 256 {
			t.Fatalf("%s center layout name=%s scale=%v anchor=%v,%v pos=%v,%v", emo, name, scale, ax, ay, px, py)
		}
		appear, _, ok := s.DocumentFile(r.GetAppearAnimation().GetId())
		if !ok {
			t.Fatalf("%s appear", emo)
		}
		if _, _, _, _, _, appearName := lottieRoot(t, appear); appearName == "pill-inset" {
			t.Fatalf("%s appear animation was inset", emo)
		}
		if bytes.Equal(body, appear) {
			t.Fatalf("%s center icon is the full-bleed appear animation", emo)
		}
		code := codeOfReaction(emo)
		legacy := legacyCenterID(code)
		if legacy == center.GetId() {
			t.Fatalf("%s kept the full-bleed document id", emo)
		}
		old, oldMIME, ok := s.DocumentFile(legacy)
		if !ok || oldMIME != mime || !bytes.Equal(old, body) {
			t.Fatalf("%s previous center id does not serve the inset file", emo)
		}
	}
}

func codeOfReaction(emo string) string {
	for _, spec := range reactionSpecs() {
		if emojiFromCode(spec.code) == emo {
			return spec.code
		}
	}
	return ""
}

func assertAttrSize(t *testing.T, emo, name string, doc *mtproto.Document, w, h int32) {
	t.Helper()
	for _, attr := range doc.GetAttributes() {
		if attr.GetPredicateName() == mtproto.Predicate_documentAttributeImageSize {
			if attr.GetW() == w && attr.GetH() == h {
				return
			}
			t.Fatalf("%s %s size %dx%d, want %dx%d", emo, name, attr.GetW(), attr.GetH(), w, h)
		}
	}
	t.Fatalf("%s %s has no image size", emo, name)
}

func hasCustomEmoji(doc *mtproto.Document) bool {
	for _, attr := range doc.GetAttributes() {
		if attr.GetPredicateName() == mtproto.Predicate_documentAttributeCustomEmoji {
			return true
		}
	}
	return false
}

func lottieRoot(t *testing.T, raw []byte) (scale, ax, ay, px, py float64, name string) {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var doc map[string]any
	if err = json.NewDecoder(io.LimitReader(r, 8<<20)).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	layers, _ := doc["layers"].([]any)
	if len(layers) != 1 {
		t.Fatalf("root layers %d", len(layers))
	}
	layer, _ := layers[0].(map[string]any)
	name, _ = layer["nm"].(string)
	ks, _ := layer["ks"].(map[string]any)
	scale = vecComponent(ks["s"], 0)
	ax = vecComponent(ks["a"], 0)
	ay = vecComponent(ks["a"], 1)
	px = vecComponent(ks["p"], 0)
	py = vecComponent(ks["p"], 1)
	return scale, ax, ay, px, py, name
}

func vecComponent(prop any, i int) float64 {
	m, _ := prop.(map[string]any)
	k, _ := m["k"].([]any)
	if i >= len(k) {
		return 0
	}
	f, _ := k[i].(float64)
	return f
}

func assertIOSReactionDocuments(t *testing.T, emo string, r *mtproto.AvailableReaction) {
	t.Helper()
	docs := []struct {
		name string
		doc  *mtproto.Document
		tgs  bool
	}{
		{"static_icon", r.GetStaticIcon(), false},
		{"appear_animation", r.GetAppearAnimation(), true},
		{"select_animation", r.GetSelectAnimation(), true},
		{"activate_animation", r.GetActivateAnimation(), true},
		{"effect_animation", r.GetEffectAnimation(), true},
		{"around_animation", r.GetAroundAnimation(), true},
		{"center_icon", r.GetCenterIcon(), true},
	}
	for _, item := range docs {
		if item.doc == nil || item.doc.GetPredicateName() != mtproto.Predicate_document {
			t.Fatalf("%s %s is not a document (%s)", emo, item.name, item.doc.GetPredicateName())
		}
		if item.doc.GetId() == 0 || item.doc.GetDcId() == 0 || item.doc.GetSize2_INT64() <= 0 {
			t.Fatalf("%s %s id/dc/size invalid", emo, item.name)
		}
		if item.doc.GetFileReference() == nil {
			t.Fatalf("%s %s file_reference is nil", emo, item.name)
		}
		wantMIME := "image/webp"
		if item.tgs {
			wantMIME = "application/x-tgsticker"
		}
		if item.doc.GetMimeType() != wantMIME {
			t.Fatalf("%s %s mime %s", emo, item.name, item.doc.GetMimeType())
		}
		var sawSize, sawSticker, sawCustom bool
		for _, attr := range item.doc.GetAttributes() {
			switch attr.GetPredicateName() {
			case mtproto.Predicate_documentAttributeImageSize:
				sawSize = true
				if item.tgs && (attr.GetW() != 512 || attr.GetH() != 512) {
					t.Fatalf("%s %s size %dx%d", emo, item.name, attr.GetW(), attr.GetH())
				}
			case mtproto.Predicate_documentAttributeSticker:
				sawSticker = true
				if attr.GetAlt() != emo {
					t.Fatalf("%s %s sticker alt %q", emo, item.name, attr.GetAlt())
				}
				if attr.GetStickerset().GetPredicateName() != mtproto.Predicate_inputStickerSetEmpty {
					t.Fatalf("%s %s stickerset %s", emo, item.name, attr.GetStickerset().GetPredicateName())
				}
			case mtproto.Predicate_documentAttributeCustomEmoji:
				sawCustom = true
			}
		}
		if !sawSize || !sawSticker || sawCustom {
			t.Fatalf("%s %s attributes size=%v sticker=%v custom=%v", emo, item.name, sawSize, sawSticker, sawCustom)
		}
	}
}
