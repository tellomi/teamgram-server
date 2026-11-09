package core

// contactHit is one contacts.search result.
type contactHit int

const (
	contactUser contactHit = iota
	contactBot
	contactGroup
	contactChannel
)

// contactHitAllowed applies contacts.search's broadcasts and bots flags.
// Neither flag returns users, bots, public groups, and public channels.
// broadcasts narrows the result to public channels. bots narrows it to bots.
// Both flags together return bots and public channels.
func contactHitAllowed(broadcasts, bots bool, hit contactHit) bool {
	if !broadcasts && !bots {
		return true
	}
	switch hit {
	case contactChannel:
		return broadcasts
	case contactBot:
		return bots
	default:
		return false
	}
}
