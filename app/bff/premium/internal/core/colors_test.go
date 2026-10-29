package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/premium"
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
	same := peerColorsReply(premium.PeerColorHash, false)
	if same.GetPredicateName() != mtproto.Predicate_help_peerColorsNotModified {
		t.Fatalf("predicate = %s", same.GetPredicateName())
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
