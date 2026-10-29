package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// emptyEmojiGroupHash is the hash of an empty emoji-group list. Clients that
// already stored it get messages.emojiGroupsNotModified.
const emptyEmojiGroupHash int32 = 1

// EmojiCategories answers messages.getEmoji*Groups. The catalog has no emoji
// groups, so every call is an empty list (or notModified when the client
// already has that hash).
type EmojiCategories struct {
	mtproto.UnimplementedRPCEmojiCategoriesServer
}

func NewEmojiCategories() *EmojiCategories {
	return &EmojiCategories{}
}

func emptyEmojiGroups(hash int32) *mtproto.Messages_EmojiGroups {
	if hash == emptyEmojiGroupHash {
		return mtproto.MakeTLMessagesEmojiGroupsNotModified(&mtproto.Messages_EmojiGroups{}).To_Messages_EmojiGroups()
	}
	return mtproto.MakeTLMessagesEmojiGroups(&mtproto.Messages_EmojiGroups{
		Hash:   emptyEmojiGroupHash,
		Groups: []*mtproto.EmojiGroup{},
	}).To_Messages_EmojiGroups()
}

func (s *EmojiCategories) MessagesGetEmojiGroups(ctx context.Context, in *mtproto.TLMessagesGetEmojiGroups) (*mtproto.Messages_EmojiGroups, error) {
	return emptyEmojiGroups(in.GetHash()), nil
}

func (s *EmojiCategories) MessagesGetEmojiStatusGroups(ctx context.Context, in *mtproto.TLMessagesGetEmojiStatusGroups) (*mtproto.Messages_EmojiGroups, error) {
	return emptyEmojiGroups(in.GetHash()), nil
}

func (s *EmojiCategories) MessagesGetEmojiProfilePhotoGroups(ctx context.Context, in *mtproto.TLMessagesGetEmojiProfilePhotoGroups) (*mtproto.Messages_EmojiGroups, error) {
	return emptyEmojiGroups(in.GetHash()), nil
}

func (s *EmojiCategories) MessagesGetEmojiStickerGroups(ctx context.Context, in *mtproto.TLMessagesGetEmojiStickerGroups) (*mtproto.Messages_EmojiGroups, error) {
	return emptyEmojiGroups(in.GetHash()), nil
}
