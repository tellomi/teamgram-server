// Package premium is the membership policy for Telegram Premium features.
// Official clients are unchanged. A process-wide config decides whether every
// account is treated as Premium and which features are free for non-members.
// Per-user status and expiry stay in users.premium and users.premium_expire_date.
package premium

import (
	"fmt"
	"hash/fnv"
	"os"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
)

// Feature is one Premium-gated capability. The names are the config tokens.
type Feature string

const (
	FeatureEmojiStatus    Feature = "emoji_status"
	FeatureAbout          Feature = "about"
	FeaturePinnedChats    Feature = "pinned_chats"
	FeatureFolderPins     Feature = "folder_pins"
	FeatureVoiceToText    Feature = "voice_to_text"
	FeatureCustomEmoji    Feature = "custom_emoji"
	FeatureProfileColor   Feature = "profile_color"
	FeatureNameColor      Feature = "name_color"
	FeatureStories        Feature = "stories"
	FeatureSavedGifs      Feature = "saved_gifs"
	FeatureFavedStickers  Feature = "faved_stickers"
	FeatureDialogFilters  Feature = "dialog_filters"
	FeatureUpload         Feature = "upload"
	FeatureCaption        Feature = "caption"
	FeatureReactions      Feature = "reactions"
	FeatureChannels       Feature = "channels"
	FeaturePublicChannels Feature = "public_channels"
)

// AllFeatures is the stable config order.
var AllFeatures = []Feature{
	FeatureEmojiStatus,
	FeatureAbout,
	FeaturePinnedChats,
	FeatureFolderPins,
	FeatureVoiceToText,
	FeatureCustomEmoji,
	FeatureProfileColor,
	FeatureNameColor,
	FeatureStories,
	FeatureSavedGifs,
	FeatureFavedStickers,
	FeatureDialogFilters,
	FeatureUpload,
	FeatureCaption,
	FeatureReactions,
	FeatureChannels,
	FeaturePublicChannels,
}

const (
	// EnvEveryone is the global switch. Default true: every non-deleted user
	// is treated as a Premium member. The database row is still stored.
	EnvEveryone = "TELLOMI_PREMIUM_EVERYONE"
	// EnvFreeFeatures is a comma-separated list of Feature tokens that are
	// free for non-members. "*" marks every feature free. Default is empty,
	// so each feature stays paid and follows membership.
	EnvFreeFeatures = "TELLOMI_PREMIUM_FREE_FEATURES"
	// EnvPurchaseBlocked hides the store purchase UI. Default true.
	EnvPurchaseBlocked = "TELLOMI_PREMIUM_PURCHASE_BLOCKED"
)

const (
	AboutLength            = 140
	AboutLengthFree        = 70
	CaptionLength          = 4096
	CaptionLengthFree      = 1024
	PinnedDialogs          = 100
	PinnedDialogsFree      = 5
	FolderPinnedDialogs    = 200
	FolderPinnedFree       = 100
	DialogFilters          = 20
	DialogFiltersFree      = 10
	DialogFilterChats      = 200
	DialogFilterChatsFree  = 100
	SavedGifs              = 400
	SavedGifsFree          = 200
	FavedStickers          = 200
	FavedStickersFree      = 5
	Channels               = 1000
	ChannelsFree           = 500
	PublicChannels         = 20
	PublicChannelsFree     = 10
	ReactionsUserMax       = 3
	ReactionsUserMaxFree   = 1
	UploadMaxFileParts     = 8000
	UploadMaxFilePartsFree = 4000
	CustomEmojiPerMessage  = 100
	StoriesPerDay          = 100
	StoriesPerDayFree      = 3

	PeerColorHash       int32 = 0x70524301
	EmojiStatusListHash int64 = 1
)

// Config is the server-side Premium policy.
type Config struct {
	// Everyone treats every non-deleted account as a Premium member.
	Everyone bool
	// Free marks features that non-members may use. A missing entry is paid.
	Free map[Feature]bool
	// PurchaseBlocked hides Premium purchase UI in official clients.
	PurchaseBlocked bool
}

var current atomic.Value

func init() {
	current.Store(fromEnv())
	mtproto.SetMembershipFunc(func(stored bool, expireUnix, now int64) bool {
		return IsMember(stored, expireUnix, now)
	})
}

// Load returns the policy currently in effect.
func Load() Config {
	cfg, _ := current.Load().(Config)
	if cfg.Free == nil {
		cfg.Free = map[Feature]bool{}
	}
	return cfg
}

// Reload reads the environment again. Tests use this after t.Setenv.
func Reload() Config {
	cfg := fromEnv()
	current.Store(cfg)
	return cfg
}

func fromEnv() Config {
	cfg := Config{
		Everyone:        envBool(EnvEveryone, true),
		PurchaseBlocked: envBool(EnvPurchaseBlocked, true),
		Free:            map[Feature]bool{},
	}
	raw := strings.TrimSpace(os.Getenv(EnvFreeFeatures))
	if raw == "*" {
		for _, feature := range AllFeatures {
			cfg.Free[feature] = true
		}
		return cfg
	}
	known := map[Feature]struct{}{}
	for _, feature := range AllFeatures {
		known[feature] = struct{}{}
	}
	for _, part := range strings.Split(raw, ",") {
		name := Feature(strings.ToLower(strings.TrimSpace(part)))
		if name == "" {
			continue
		}
		if _, ok := known[name]; ok {
			cfg.Free[name] = true
		}
	}
	return cfg
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

// ActiveSubscription reports whether the database row is an unexpired Premium
// subscription. expireUnix 0 means the flag has no end date.
func ActiveSubscription(stored bool, expireUnix, now int64) bool {
	if !stored {
		return false
	}
	if expireUnix == 0 {
		return true
	}
	return now < expireUnix
}

// IsMember reports the effective membership shown as user.premium.
// The global switch grants membership without writing the database row.
func IsMember(stored bool, expireUnix, now int64) bool {
	if Load().Everyone {
		return true
	}
	return ActiveSubscription(stored, expireUnix, now)
}

// Allows reports whether this account may use the feature.
// A feature marked free is allowed for non-members. A paid feature requires
// effective membership: the caller may pass user.Premium(), and the global
// switch is also applied here so a stale false still follows config.
func Allows(feature Feature, member bool) bool {
	cfg := Load()
	if cfg.Free[feature] || cfg.Everyone || member {
		return true
	}
	return false
}

type featureLimit struct {
	feature Feature
	free    int
	paid    int
}

func limits() []featureLimit {
	return []featureLimit{
		{FeatureAbout, AboutLengthFree, AboutLength},
		{FeatureCaption, CaptionLengthFree, CaptionLength},
		{FeaturePinnedChats, PinnedDialogsFree, PinnedDialogs},
		{FeatureFolderPins, FolderPinnedFree, FolderPinnedDialogs},
		{FeatureDialogFilters, DialogFiltersFree, DialogFilters},
		{FeatureSavedGifs, SavedGifsFree, SavedGifs},
		{FeatureFavedStickers, FavedStickersFree, FavedStickers},
		{FeatureChannels, ChannelsFree, Channels},
		{FeaturePublicChannels, PublicChannelsFree, PublicChannels},
		{FeatureReactions, ReactionsUserMaxFree, ReactionsUserMax},
		{FeatureUpload, UploadMaxFilePartsFree, UploadMaxFileParts},
		{FeatureStories, StoriesPerDayFree, StoriesPerDay},
	}
}

func limitOf(feature Feature) (free, paid int, ok bool) {
	for _, item := range limits() {
		if item.feature == feature {
			return item.free, item.paid, true
		}
	}
	return 0, 0, false
}

// Limit is the server cap for a feature given effective membership.
func Limit(feature Feature, member bool) int {
	free, paid, ok := limitOf(feature)
	if !ok {
		return 0
	}
	if Allows(feature, member) {
		return paid
	}
	return free
}

// Advertised is the (default, premium) pair help.getAppConfig should publish.
// When non-members may use the feature, both keys are the raised cap so a
// client that ignores the badge still applies it.
func Advertised(feature Feature) (def, prem int) {
	free, paid, ok := limitOf(feature)
	if !ok {
		return 0, 0
	}
	prem = paid
	if Load().Free[feature] || Load().Everyone {
		return paid, paid
	}
	return free, paid
}

// CustomEmojiAllowance is stickers_premium_by_emoji_num.
func CustomEmojiAllowance() int {
	if Allows(FeatureCustomEmoji, false) || Load().Everyone {
		return CustomEmojiPerMessage
	}
	return 0
}

// AboutWithinLimit reports whether a biography fits the caller's cap.
func AboutWithinLimit(about string, member bool) bool {
	return utf8.RuneCountInString(about) <= Limit(FeatureAbout, member)
}

// AppConfigHash changes when the policy changes so clients refetch help.getAppConfig.
func AppConfigHash() int32 {
	cfg := Load()
	h := fnv.New32a()
	fmt.Fprintf(h, "everyone=%t purchase=%t", cfg.Everyone, cfg.PurchaseBlocked)
	for _, feature := range AllFeatures {
		def, prem := Advertised(feature)
		fmt.Fprintf(h, "|%s free=%t %d %d", feature, cfg.Free[feature], def, prem)
	}
	def, prem := DialogFilterChatLimit()
	fmt.Fprintf(h, "|chats %d %d emoji %d", def, prem, CustomEmojiAllowance())
	sum := int32(h.Sum32() & 0x7fffffff)
	if sum == 0 {
		return 1
	}
	return sum
}

// DialogFilterChatLimit is advertised separately because the folder-size cap
// shares the dialog-filter feature switch.
func DialogFilterChatLimit() (def, prem int) {
	if Load().Free[FeatureDialogFilters] || Load().Everyone {
		return DialogFilterChats, DialogFilterChats
	}
	return DialogFilterChatsFree, DialogFilterChats
}
