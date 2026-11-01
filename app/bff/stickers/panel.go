package stickers_helper

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/internal/server/grpc/service"
)

// These functions are the bff handlers the session routes for the emoji panel.
// e2e calls them so the test stays outside the internal tree.

func GetEmojiGroups(hash int32) (*mtproto.Messages_EmojiGroups, error) {
	return service.NewEmojiCategories().MessagesGetEmojiGroups(context.Background(), &mtproto.TLMessagesGetEmojiGroups{Hash: hash})
}

func GetEmojiStatusGroups(hash int32) (*mtproto.Messages_EmojiGroups, error) {
	return service.NewEmojiCategories().MessagesGetEmojiStatusGroups(context.Background(), &mtproto.TLMessagesGetEmojiStatusGroups{Hash: hash})
}

func GetEmojiProfilePhotoGroups(hash int32) (*mtproto.Messages_EmojiGroups, error) {
	return service.NewEmojiCategories().MessagesGetEmojiProfilePhotoGroups(context.Background(), &mtproto.TLMessagesGetEmojiProfilePhotoGroups{Hash: hash})
}

func GetEmojiStickerGroups(hash int32) (*mtproto.Messages_EmojiGroups, error) {
	return service.NewEmojiCategories().MessagesGetEmojiStickerGroups(context.Background(), &mtproto.TLMessagesGetEmojiStickerGroups{Hash: hash})
}

func DefaultProfilePhotoEmojis(hash int64) (*mtproto.EmojiList, error) {
	return (&service.Service{}).AccountGetDefaultProfilePhotoEmojis(context.Background(), &mtproto.TLAccountGetDefaultProfilePhotoEmojis{Hash: hash})
}

func DefaultGroupPhotoEmojis(hash int64) (*mtproto.EmojiList, error) {
	return (&service.Service{}).AccountGetDefaultGroupPhotoEmojis(context.Background(), &mtproto.TLAccountGetDefaultGroupPhotoEmojis{Hash: hash})
}
