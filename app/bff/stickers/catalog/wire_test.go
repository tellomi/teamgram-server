package catalog

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestFeaturedAndStickerSetRoundTrip(t *testing.T) {
	s := Must()
	for _, emoji := range []bool{false, true} {
		featured := s.Featured(1, 0, emoji)
		for _, set := range featured.GetSets() {
			if set.GetSet().GetShortName() == ShortTwemoji || set.GetSet().GetShortName() == ShortFluent || set.GetSet().GetShortName() == ShortNoto {
				t.Fatalf("foreign seed still featured: %s", set.GetSet().GetShortName())
			}
		}
		got, ok := roundTrip(t, featured).(*mtproto.TLMessagesFeaturedStickers)
		if !ok {
			t.Fatalf("featured decoded as %T", roundTrip(t, featured))
		}
		if len(got.GetSets()) != len(featured.GetSets()) || len(got.GetUnread()) != len(featured.GetUnread()) {
			t.Fatalf("emoji=%v round trip sets %d/%d unread %d/%d", emoji, len(got.GetSets()), len(featured.GetSets()), len(got.GetUnread()), len(featured.GetUnread()))
		}
	}
	for _, name := range []string{ShortTwemoji, ShortAnimatedEmoji} {
		in := mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{ShortName: name}).To_InputStickerSet()
		set, err := s.GetSet(1, in, 0)
		if err != nil {
			t.Fatal(name, err)
		}
		if len(set.GetDocuments()) == 0 {
			t.Fatalf("%s has no documents", name)
		}
		got, ok := roundTrip(t, set).(*mtproto.TLMessagesStickerSet)
		if !ok {
			t.Fatalf("%s decoded as %T", name, roundTrip(t, set))
		}
		if len(got.GetDocuments()) != len(set.GetDocuments()) {
			t.Fatalf("%s documents %d want %d", name, len(got.GetDocuments()), len(set.GetDocuments()))
		}
		if got.GetDocuments()[0].GetId() == 0 {
			t.Fatalf("%s document id was dropped", name)
		}
	}
}

func roundTrip(t *testing.T, obj mtproto.TLObject) mtproto.TLObject {
	t.Helper()
	buf := mtproto.NewEncodeBuf(1 << 20)
	if err := obj.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	d := mtproto.NewDecodeBuf(append([]byte(nil), buf.GetBuf()...))
	out := d.Object()
	if err := d.GetError(); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out == nil {
		t.Fatal("decode returned nil")
	}
	return out
}
