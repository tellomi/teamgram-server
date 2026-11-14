package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/premium"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestPeerColors(t *testing.T) {
	colors := peerColors(false)
	if colors.GetHash() != premium.PeerColorHash {
		t.Fatalf("hash = %d", colors.GetHash())
	}
	if len(colors.GetColors()) != len(nameColors) {
		t.Fatalf("name colors = %d", len(colors.GetColors()))
	}
	if colors.GetColors()[0].GetColorId() != 0 {
		t.Fatal("first name color id is not 0")
	}
	profile := peerColors(true)
	if len(profile.GetColors()) != len(profileColors) {
		t.Fatalf("profile colors = %d", len(profile.GetColors()))
	}
	for _, opt := range profile.GetColors() {
		light := opt.GetColors()
		dark := opt.GetDarkColors()
		if light.GetPredicateName() != mtproto.Predicate_help_peerColorProfileSet || dark.GetPredicateName() != mtproto.Predicate_help_peerColorProfileSet {
			t.Fatalf("profile set light %s dark %s", light.GetPredicateName(), dark.GetPredicateName())
		}
		if len(light.GetBgColors()) != 2 || len(dark.GetBgColors()) != 2 || len(light.GetPaletteColors()) != 2 || len(dark.GetStoryColors()) != 2 {
			t.Fatalf("palette lengths light %#v dark %#v", light, dark)
		}
		for _, c := range append(append([]int32{}, light.GetBgColors()...), dark.GetBgColors()...) {
			if colorLuma(c) > 0.65 {
				t.Fatalf("profile background #%06x is too light for white buttons", uint32(c))
			}
		}
	}
	for _, opt := range colors.GetColors() {
		if opt.GetDarkColors() == nil || len(opt.GetDarkColors().GetColors()) == 0 {
			t.Fatal("name color is missing dark_colors")
		}
	}
	same := peerColorsReply(premium.PeerColorHash, false)
	if same.GetPredicateName() != mtproto.Predicate_help_peerColorsNotModified {
		t.Fatalf("predicate = %s", same.GetPredicateName())
	}
}

func colorLuma(c int32) float64 {
	r := float64((c>>16)&0xff) / 255
	g := float64((c>>8)&0xff) / 255
	b := float64(c&0xff) / 255
	return 0.2126*r + 0.7152*g + 0.0722*b
}

func TestKnownPeerColorRejects999(t *testing.T) {
	if !knownPeerColor(false, 0) || !knownPeerColor(false, int32(len(nameColors)-1)) {
		t.Fatal("name color ids from help.getPeerColors were rejected")
	}
	if knownPeerColor(false, 999) || knownPeerColor(false, int32(len(nameColors))) || knownPeerColor(false, -1) {
		t.Fatal("unknown name color was accepted")
	}
	if !knownPeerColor(true, 0) || knownPeerColor(true, 999) {
		t.Fatal("profile color check does not follow help.getPeerProfileColors")
	}
	_, err := (&PremiumCore{}).AccountUpdateColor(&mtproto.TLAccountUpdateColor{
		Color_FLAGINT32: &wrapperspb.Int32Value{Value: 999},
	})
	if status.Convert(err).Message() != "COLOR_INVALID" {
		t.Fatalf("account.updateColor 999: %v", err)
	}
	_, err = (&PremiumCore{}).AccountUpdateColor(&mtproto.TLAccountUpdateColor{
		ForProfile: true,
		Color_FLAGPEERCOLOR: mtproto.MakeTLPeerColor(&mtproto.PeerColor{
			Color: &wrapperspb.Int32Value{Value: 999},
		}).To_PeerColor(),
	})
	if status.Convert(err).Message() != "COLOR_INVALID" {
		t.Fatalf("profile color 999: %v", err)
	}
}

func TestSelectedColor(t *testing.T) {
	id, emoji, set := selectedColor(&mtproto.TLAccountUpdateColor{
		Color_FLAGINT32:   &wrapperspb.Int32Value{Value: 0},
		BackgroundEmojiId: &wrapperspb.Int64Value{Value: 42},
	})
	if !set || id != 0 || emoji != 42 {
		t.Fatalf("color id=%d emoji=%d set=%v", id, emoji, set)
	}
	id, emoji, set = selectedColor(&mtproto.TLAccountUpdateColor{})
	if set || id != -1 || emoji != 0 {
		t.Fatalf("clear color id=%d emoji=%d set=%v", id, emoji, set)
	}
}

func TestEmojiStatusParts(t *testing.T) {
	doc, until := emojiStatusParts(mtproto.MakeTLEmojiStatusEmpty(&mtproto.EmojiStatus{}).To_EmojiStatus())
	if doc != 0 || until != 0 {
		t.Fatalf("empty status doc=%d until=%d", doc, until)
	}
	doc, until = emojiStatusParts(&mtproto.EmojiStatus{
		PredicateName:   mtproto.Predicate_emojiStatus,
		DocumentId:      99,
		Until_FLAGINT32: &wrapperspb.Int32Value{Value: 10},
	})
	if doc != 99 || until != 10 {
		t.Fatalf("status doc=%d until=%d", doc, until)
	}
}
