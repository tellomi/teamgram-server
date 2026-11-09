package catalog

import (
	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// VenueBotUser is the built-in @venue inline bot (display name 位置).
// Clients learn the username from help.getConfig venue_search_username.
func VenueBotUser() *mtproto.User {
	return mtproto.MakeTLUser(&mtproto.User{
		Bot:                  true,
		Verified:             true,
		BotInlineGeo:         true,
		Id:                   mtproto.BotVenueId,
		AccessHash:           wrapperspb.Int64(stableID("venue-access", "bot")),
		FirstName:            wrapperspb.String("位置"),
		Username:             wrapperspb.String(mtproto.BotVenueName),
		BotInlinePlaceholder: wrapperspb.String("搜索地点"),
		BotInfoVersion:       wrapperspb.Int32(1),
	}).To_User()
}
