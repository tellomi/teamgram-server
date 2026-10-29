package stickers_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/bff/stickers/internal/server/grpc/service"
)

type Config struct {
	MySQL string `json:",optional"`
}

func New(c Config) *service.Service {
	catalog.Open(c.MySQL)
	return service.New()
}

func NewEmojiCategories() *service.EmojiCategories {
	return service.NewEmojiCategories()
}
