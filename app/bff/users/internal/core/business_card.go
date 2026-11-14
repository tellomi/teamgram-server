package core

import (
	"github.com/teamgram/proto/mtproto"
	profile "github.com/teamgram/teamgram-server/app/bff/business/profile"
)

// applyBusinessCard copies a business profile onto userFull.
// Hours, location, and intro are visible to anyone who can open the profile.
// Greeting and away messages carry the recipient list (and the exclude-user
// list, stored as users with exclude_selected). The official server omits
// those two objects for anyone other than the owner.
func applyBusinessCard(full *mtproto.UserFull, viewer, owner int64, card *profile.Card) {
	if full == nil || card == nil {
		return
	}
	full.BusinessWorkHours = card.WorkHours
	full.BusinessLocation = card.Location
	full.BusinessIntro = card.Intro
	if viewer != owner {
		full.BusinessGreetingMessage = nil
		full.BusinessAwayMessage = nil
		return
	}
	full.BusinessGreetingMessage = card.Greeting
	full.BusinessAwayMessage = card.Away
}
