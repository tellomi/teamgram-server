package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
)

// EmojiCategories answers messages.getEmoji*Groups from the sticker catalog.
// https://core.telegram.org/method/messages.getEmojiGroups
type EmojiCategories struct {
	mtproto.UnimplementedRPCEmojiCategoriesServer
}

func NewEmojiCategories() *EmojiCategories {
	return &EmojiCategories{}
}

func (s *EmojiCategories) MessagesGetEmojiGroups(ctx context.Context, in *mtproto.TLMessagesGetEmojiGroups) (*mtproto.Messages_EmojiGroups, error) {
	return cat().EmojiGroups(catalog.GroupEmoji, in.GetHash()), nil
}

func (s *EmojiCategories) MessagesGetEmojiStatusGroups(ctx context.Context, in *mtproto.TLMessagesGetEmojiStatusGroups) (*mtproto.Messages_EmojiGroups, error) {
	return cat().EmojiGroups(catalog.GroupStatus, in.GetHash()), nil
}

func (s *EmojiCategories) MessagesGetEmojiProfilePhotoGroups(ctx context.Context, in *mtproto.TLMessagesGetEmojiProfilePhotoGroups) (*mtproto.Messages_EmojiGroups, error) {
	return cat().EmojiGroups(catalog.GroupProfile, in.GetHash()), nil
}

func (s *EmojiCategories) MessagesGetEmojiStickerGroups(ctx context.Context, in *mtproto.TLMessagesGetEmojiStickerGroups) (*mtproto.Messages_EmojiGroups, error) {
	return cat().EmojiGroups(catalog.GroupSticker, in.GetHash()), nil
}
