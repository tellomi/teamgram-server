package catalog

import (
	"strings"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// BuiltinBotUser is a reserved inline bot that has no users-table row.
// @gif and @venue keep the objects clients already cache. The other names
// are the sample bots listed next to those ids. A real row for the same id
// is left alone by the caller.
func BuiltinBotUser(id int64) *mtproto.User {
	switch id {
	case mtproto.BotGifId:
		return GifBotUser()
	case mtproto.BotVenueId:
		return VenueBotUser()
	}
	name := mtproto.GetBotNameById(id)
	if name == "" {
		return nil
	}
	return mtproto.MakeTLUser(&mtproto.User{
		Bot:            true,
		Verified:       true,
		Id:             id,
		AccessHash:     wrapperspb.Int64(stableID("builtin-bot-access", strings.ToLower(name))),
		FirstName:      wrapperspb.String(builtinFirstName(id)),
		Username:       wrapperspb.String(name),
		BotInfoVersion: wrapperspb.Int32(1),
	}).To_User()
}

func builtinFirstName(id int64) string {
	switch id {
	case mtproto.BotFatherId:
		return "BotFather"
	case mtproto.BotVidId:
		return "Video"
	case mtproto.BotPicId:
		return "Yandex"
	case mtproto.BotBingId:
		return "Bing"
	case mtproto.BotWikiId:
		return "Wikipedia"
	case mtproto.BotImdbId:
		return "IMDb"
	case mtproto.BotBoldId:
		return "Bold"
	case mtproto.BotYoutubeId:
		return "YouTube"
	case mtproto.BotMusicId:
		return "Music"
	case mtproto.BotStickerId:
		return "Stickers"
	default:
		return mtproto.GetBotNameById(id)
	}
}
