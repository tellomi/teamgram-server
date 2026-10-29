package catalog

import (
	"bytes"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// The emoji the iOS picker and double-tap must always be able to use.
var iosDefaultReactions = []string{
	"👍", "❤️", "🔥", "🥰", "👏", "😁", "🤔", "😢", "🎉", "🙏", "👎", "😱",
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
