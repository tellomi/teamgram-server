package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestEmptyEmojiGroups(t *testing.T) {
	svc := NewEmojiCategories()
	ctx := context.Background()

	groups, err := svc.MessagesGetEmojiStickerGroups(ctx, &mtproto.TLMessagesGetEmojiStickerGroups{})
	if err != nil {
		t.Fatal(err)
	}
	if groups.GetPredicateName() != mtproto.Predicate_messages_emojiGroups {
		t.Fatalf("predicate = %s", groups.GetPredicateName())
	}
	if groups.GetHash() != emptyEmojiGroupHash || len(groups.GetGroups()) != 0 {
		t.Fatalf("groups = %#v", groups)
	}

	same, err := svc.MessagesGetEmojiStatusGroups(ctx, &mtproto.TLMessagesGetEmojiStatusGroups{Hash: emptyEmojiGroupHash})
	if err != nil {
		t.Fatal(err)
	}
	if same.GetPredicateName() != mtproto.Predicate_messages_emojiGroupsNotModified {
		t.Fatalf("cached hash predicate = %s", same.GetPredicateName())
	}

	photo, err := svc.MessagesGetEmojiProfilePhotoGroups(ctx, &mtproto.TLMessagesGetEmojiProfilePhotoGroups{Hash: 9})
	if err != nil {
		t.Fatal(err)
	}
	if photo.GetPredicateName() != mtproto.Predicate_messages_emojiGroups || photo.GetHash() != emptyEmojiGroupHash {
		t.Fatalf("profile groups = %#v", photo)
	}
}
