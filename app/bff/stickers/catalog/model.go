package catalog

import "github.com/teamgram/proto/mtproto"

// Pack kinds. System kinds are fetched by inputStickerSet* constructors.
// They are not mixed into messages.getAllStickers.
const (
	KindSticker            = "sticker"
	KindCustomEmoji        = "custom_emoji"
	KindAnimatedEmoji      = "animated_emoji"
	KindAnimatedAnimations = "animated_emoji_animations"
	KindGenericAnimations  = "emoji_generic_animations"
	KindPremiumGifts       = "premium_gifts"
	KindDefaultStatuses    = "emoji_default_statuses"
	KindTopicIcons         = "emoji_default_topic_icons"
	KindChannelStatuses    = "emoji_channel_default_statuses"
)

const (
	ShortAnimatedEmoji      = "AnimatedEmoji"
	ShortAnimatedAnimations = "AnimatedEmojiAnimations"
	ShortGenericAnimations  = "EmojiGenericAnimations"
	ShortPremiumGifts       = "PremiumGifts"
	ShortDefaultStatuses    = "EmojiDefaultStatuses"
	ShortTopicIcons         = "EmojiDefaultTopicIcons"
	ShortChannelStatuses    = "EmojiChannelDefaultStatuses"
	ShortTwemoji            = "Twemoji"
	ShortFluent             = "FluentEmoji"
	ShortNoto               = "NotoEmoji"
)

// Pack is one sticker or emoji set.
type Pack struct {
	ID               int64
	AccessHash       int64
	ShortName        string
	Title            string
	Kind             string
	Official         bool
	Masks            bool
	Emojis           bool
	Animated         bool
	Videos           bool
	TextColor        bool
	Featured         bool
	InstalledDefault bool
	License          string
	Attribution      string
	SourceURL        string
	// Owner is the user who created the pack. Zero means an official pack.
	Owner   int64
	ThumbID int64
	Items   []*Item
}

// Item is one document inside a pack. The same document may be linked from
// more than one pack; the bytes live once in object storage.
type Item struct {
	DocumentID   int64
	AccessHash   int64
	Emoticon     string
	MIME         string
	FileName     string
	Width        int32
	Height       int32
	Animated     bool
	Video        bool
	CustomEmoji  bool
	StorageClass string
	ObjectPath   string
	AssetPath    string
	Keywords     []string
	Size         int32
	MaskCoords   *mtproto.MaskCoords
}

func (p *Pack) system() bool {
	switch p.Kind {
	case KindSticker, KindCustomEmoji:
		return false
	default:
		return true
	}
}
