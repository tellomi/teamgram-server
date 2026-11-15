package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/premium"
)

func TestDefaultEmojiStatusesComeFromTheCatalog(t *testing.T) {
	got := emojiStatusList(0, false)
	if len(got.GetStatuses()) == 0 {
		t.Fatal("default emoji statuses are empty")
	}
	if got.GetHash() == 0 || got.GetHash() == premium.EmojiStatusListHash {
		t.Fatalf("hash %d still matches the old empty list", got.GetHash())
	}
	again := emojiStatusList(got.GetHash(), false)
	if again.GetPredicateName() != mtproto.Predicate_account_emojiStatusesNotModified {
		t.Fatalf("repeat hash returned %s", again.GetPredicateName())
	}
	stale := emojiStatusList(premium.EmojiStatusListHash, false)
	if len(stale.GetStatuses()) == 0 {
		t.Fatal("a client that cached the empty-list hash stayed empty")
	}
	channel := emojiStatusList(0, true)
	if len(channel.GetStatuses()) == 0 {
		t.Fatal("channel default emoji statuses are empty")
	}
}
