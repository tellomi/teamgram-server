// Package premium is the server-side policy for free Premium.
// Official clients are not modified. Limits here are what the server
// enforces and what help.getAppConfig advertises to those clients.
package premium

import "unicode/utf8"

const (
	// AboutLength is the Premium biography length, in Unicode code points.
	AboutLength = 140
	// CaptionLength is the Premium media caption length.
	CaptionLength = 4096
	// PinnedDialogs is the main-folder pin cap (stock clients default to 5).
	PinnedDialogs = 100
	// FolderPinnedDialogs is the archive-folder pin cap.
	FolderPinnedDialogs = 200
	// DialogFilters is the chat-folder cap.
	DialogFilters = 20
	// DialogFilterChats is how many chats one folder may contain.
	DialogFilterChats = 200
	// SavedGifs is the saved-GIF cap.
	SavedGifs = 400
	// FavedStickers is the favorite-sticker cap.
	FavedStickers = 200
	// Channels is how many channels and megagroups an account may join.
	Channels = 1000
	// PublicChannels is how many public usernames an account may own.
	PublicChannels = 20
	// ReactionsUserMax is how many reactions one person may add to a message.
	ReactionsUserMax = 3
	// UploadMaxFileParts is the Premium upload part cap (512 KiB parts).
	UploadMaxFileParts = 8000
	// CustomEmojiPerMessage is how many custom emoji a message may contain
	// before a client would otherwise ask for Premium.
	CustomEmojiPerMessage = 100

	// AppConfigHash is stable for the current help.getAppConfig document.
	// Bump it when the document changes so clients refresh.
	AppConfigHash int32 = 0x544c5032
	// PeerColorHash is stable for help.getPeerColors and help.getPeerProfileColors.
	PeerColorHash int32 = 0x70524301
	// EmojiStatusListHash is the hash of the built-in emoji-status catalog.
	EmojiStatusListHash int64 = 1
)

// AboutWithinLimit reports whether a biography fits the Premium length.
func AboutWithinLimit(about string) bool {
	return utf8.RuneCountInString(about) <= AboutLength
}

// StoreColor maps a client color id onto the users.color column.
// The column uses 0 for "no color". Color id 0 is a real palette entry,
// so it is stored as 1. A negative id clears the color.
func StoreColor(colorID int32) int32 {
	if colorID < 0 {
		return 0
	}
	return colorID + 1
}

// LoadColor reverses StoreColor.
func LoadColor(stored int32) (colorID int32, set bool) {
	if stored <= 0 {
		return 0, false
	}
	return stored - 1, true
}
