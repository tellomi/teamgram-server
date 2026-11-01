package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
)

func TestEmojiGroupsFromCatalog(t *testing.T) {
	svc := NewEmojiCategories()
	ctx := context.Background()

	groups, err := svc.MessagesGetEmojiGroups(ctx, &mtproto.TLMessagesGetEmojiGroups{})
	if err != nil {
		t.Fatal(err)
	}
	if groups.GetPredicateName() != mtproto.Predicate_messages_emojiGroups || len(groups.GetGroups()) < 5 {
		t.Fatalf("emoji groups = %s len %d", groups.GetPredicateName(), len(groups.GetGroups()))
	}
	if err := groups.Encode(mtproto.NewEncodeBuf(4096), 232); err != nil {
		t.Fatal(err)
	}
	same, err := svc.MessagesGetEmojiGroups(ctx, &mtproto.TLMessagesGetEmojiGroups{Hash: groups.GetHash()})
	if err != nil || same.GetPredicateName() != mtproto.Predicate_messages_emojiGroupsNotModified {
		t.Fatalf("cached hash: %v %s", err, same.GetPredicateName())
	}

	status, err := svc.MessagesGetEmojiStatusGroups(ctx, &mtproto.TLMessagesGetEmojiStatusGroups{})
	if err != nil || len(status.GetGroups()) == 0 || status.GetHash() == groups.GetHash() {
		t.Fatalf("status groups: %v hash %d vs %d len %d", err, status.GetHash(), groups.GetHash(), len(status.GetGroups()))
	}
	photo, err := svc.MessagesGetEmojiProfilePhotoGroups(ctx, &mtproto.TLMessagesGetEmojiProfilePhotoGroups{})
	if err != nil || len(photo.GetGroups()) != 2 {
		t.Fatalf("profile groups: %v len %d", err, len(photo.GetGroups()))
	}
	stickers, err := svc.MessagesGetEmojiStickerGroups(ctx, &mtproto.TLMessagesGetEmojiStickerGroups{})
	if err != nil || len(stickers.GetGroups()) != len(groups.GetGroups()) {
		t.Fatalf("sticker groups: %v len %d", err, len(stickers.GetGroups()))
	}

	icon := groups.GetGroups()[0].GetIconEmojiId()
	docs := catalog.Must().Documents([]int64{icon})
	if len(docs.GetDatas()) != 1 || docs.Datas[0].GetPredicateName() != mtproto.Predicate_document {
		t.Fatalf("icon document %d = %#v", icon, docs)
	}
}

func TestDefaultPhotoEmojis(t *testing.T) {
	svc := &Service{}
	profile, err := svc.AccountGetDefaultProfilePhotoEmojis(context.Background(), &mtproto.TLAccountGetDefaultProfilePhotoEmojis{})
	if err != nil || profile.GetPredicateName() != mtproto.Predicate_emojiList || len(profile.GetDocumentId()) < 8 {
		t.Fatalf("profile: %v len %d", err, len(profile.GetDocumentId()))
	}
	if err := profile.Encode(mtproto.NewEncodeBuf(1024), 232); err != nil {
		t.Fatal(err)
	}
	cached, err := svc.AccountGetDefaultProfilePhotoEmojis(context.Background(), &mtproto.TLAccountGetDefaultProfilePhotoEmojis{Hash: profile.GetHash()})
	if err != nil || cached.GetPredicateName() != mtproto.Predicate_emojiListNotModified {
		t.Fatalf("profile cache: %v %s", err, cached.GetPredicateName())
	}
	group, err := svc.AccountGetDefaultGroupPhotoEmojis(context.Background(), &mtproto.TLAccountGetDefaultGroupPhotoEmojis{})
	if err != nil || len(group.GetDocumentId()) < 8 || group.GetHash() == profile.GetHash() {
		t.Fatalf("group: %v len %d hash %d", err, len(group.GetDocumentId()), group.GetHash())
	}
	docs := catalog.Must().Documents(profile.GetDocumentId())
	for i, doc := range docs.GetDatas() {
		if doc.GetPredicateName() != mtproto.Predicate_document {
			t.Fatalf("profile doc %d is %s", i, doc.GetPredicateName())
		}
	}
}
