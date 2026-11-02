package inmem

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	stickers "github.com/teamgram/teamgram-server/app/bff/stickers"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
)

// The emoji panel, status picker, and profile-photo picker call these RPCs
// while a chat is open. Each one used to return an empty list.
func TestEmojiPanelCatalog(t *testing.T) {
	cases := []struct {
		name string
		load func(hash int32) (*mtproto.Messages_EmojiGroups, error)
	}{
		{"messages.getEmojiGroups", stickers.GetEmojiGroups},
		{"messages.getEmojiStatusGroups", stickers.GetEmojiStatusGroups},
		{"messages.getEmojiProfilePhotoGroups", stickers.GetEmojiProfilePhotoGroups},
		{"messages.getEmojiStickerGroups", stickers.GetEmojiStickerGroups},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, err := tc.load(0)
			if err != nil {
				t.Fatal(err)
			}
			if page.GetPredicateName() != mtproto.Predicate_messages_emojiGroups || len(page.GetGroups()) == 0 || page.GetHash() == 0 {
				t.Fatalf("page = %s groups %d hash %d", page.GetPredicateName(), len(page.GetGroups()), page.GetHash())
			}
			if err := page.Encode(mtproto.NewEncodeBuf(8192), 232); err != nil {
				t.Fatal(err)
			}
			for _, g := range page.GetGroups() {
				if g.GetTitle() == "" || g.GetIconEmojiId() == 0 || len(g.GetEmoticons()) == 0 {
					t.Fatalf("group %#v", g)
				}
				docs := catalog.Must().Documents([]int64{g.GetIconEmojiId()})
				if len(docs.GetDatas()) != 1 || docs.Datas[0].GetId() != g.GetIconEmojiId() || docs.Datas[0].GetPredicateName() != mtproto.Predicate_document {
					t.Fatalf("icon %d = %#v", g.GetIconEmojiId(), docs)
				}
			}
			cached, err := tc.load(page.GetHash())
			if err != nil || cached.GetPredicateName() != mtproto.Predicate_messages_emojiGroupsNotModified {
				t.Fatalf("not modified: %v %s", err, cached.GetPredicateName())
			}
			if err := cached.Encode(mtproto.NewEncodeBuf(64), 232); err != nil {
				t.Fatal(err)
			}
		})
	}

	profile, err := stickers.DefaultProfilePhotoEmojis(0)
	if err != nil || len(profile.GetDocumentId()) == 0 {
		t.Fatalf("profile emojis: %v %#v", err, profile)
	}
	if err := profile.Encode(mtproto.NewEncodeBuf(1024), 232); err != nil {
		t.Fatal(err)
	}
	group, err := stickers.DefaultGroupPhotoEmojis(0)
	if err != nil || len(group.GetDocumentId()) == 0 || sameIDs(profile.GetDocumentId(), group.GetDocumentId()) {
		t.Fatalf("group emojis overlap profile: %v %v %v", err, profile.GetDocumentId(), group.GetDocumentId())
	}
	docs := catalog.Must().Documents(append(append([]int64{}, profile.GetDocumentId()...), group.GetDocumentId()...))
	for _, doc := range docs.GetDatas() {
		if doc.GetPredicateName() != mtproto.Predicate_document || doc.GetMimeType() == "" {
			t.Fatalf("document %#v", doc)
		}
	}
}

func sameIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
