package linkpreview

// SchemaVersion is the JSON contract version. See docs/link-preview.md.
const SchemaVersion = 1

const (
	KindUser       = "user"
	KindBot        = "bot"
	KindChannel    = "channel"
	KindGroup      = "group"
	KindInvite     = "invite"
	KindMessage    = "message"
	KindFeed       = "feed"
	KindStickerSet = "sticker_set"
	KindEmojiSet   = "emoji_set"
	KindProxy      = "proxy"
	KindSocks      = "socks"
	KindShare      = "share"
	KindBoost      = "boost"
	KindStory      = "story"
)

const (
	ErrNotFound         = "not_found"
	ErrInvalidPath      = "invalid_path"
	ErrUnauthorized     = "unauthorized"
	ErrRateLimited      = "rate_limited"
	ErrInternal         = "internal"
	ErrMediaUnavailable = "media_unavailable"
)

// Card is the JSON body of GET /linkpreview/v1/resolve.
// Fields that do not apply to kind are omitted.
type Card struct {
	OK            bool    `json:"ok"`
	SchemaVersion int     `json:"schema_version"`
	Error         string  `json:"error,omitempty"`
	Kind          string  `json:"kind,omitempty"`
	Path          string  `json:"path,omitempty"`
	Link          string  `json:"link,omitempty"`
	Peer          *Peer   `json:"peer,omitempty"`
	Invite        *Invite `json:"invite,omitempty"`
	Post          *Post   `json:"post,omitempty"`
	Posts         *[]Post `json:"posts,omitempty"`
	Pack          *Pack   `json:"pack,omitempty"`
	Proxy         *Proxy  `json:"proxy,omitempty"`
	Socks         *Socks  `json:"socks,omitempty"`
	Share         *Share  `json:"share,omitempty"`
	Story         *Story  `json:"story,omitempty"`
}

// Peer is a public user, bot, channel, or supergroup.
type Peer struct {
	Kind        string `json:"kind"`
	Title       string `json:"title,omitempty"`
	Username    string `json:"username,omitempty"`
	About       string `json:"about,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Verified    bool   `json:"verified"`
	Scam        bool   `json:"scam"`
	Fake        bool   `json:"fake"`
	Premium     bool   `json:"premium"`
	MemberCount *int32 `json:"member_count,omitempty"`
	Broadcast   bool   `json:"broadcast,omitempty"`
	Megagroup   bool   `json:"megagroup,omitempty"`
	Start       *Start `json:"start,omitempty"`
}

// Start is a bot deep-link parameter (start, startgroup, startchannel, startattach).
type Start struct {
	Param string `json:"param"`
	Value string `json:"value"`
}

// Invite is the public preview of a chat invite link.
type Invite struct {
	Title         string `json:"title,omitempty"`
	About         string `json:"about,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	MemberCount   int32  `json:"member_count"`
	RequestNeeded bool   `json:"request_needed"`
	Broadcast     bool   `json:"broadcast,omitempty"`
	Megagroup     bool   `json:"megagroup,omitempty"`
}

// Post is one public channel or supergroup message.
type Post struct {
	MessageID int32  `json:"message_id"`
	Date      int32  `json:"date,omitempty"`
	Excerpt   string `json:"excerpt,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	MediaURL  string `json:"media_url,omitempty"`
	Views     *int32 `json:"views,omitempty"`

	mediaPhotoID    int64
	mediaDocumentID int64
}

// Pack is a public sticker or custom-emoji set.
type Pack struct {
	ShortName string `json:"short_name"`
	Title     string `json:"title,omitempty"`
	Count     int32  `json:"count"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Emoji     bool   `json:"emoji"`
}

// Proxy is an MTProto proxy share link. Secret is the value from the link itself.
type Proxy struct {
	Server string `json:"server"`
	Port   int    `json:"port"`
	Secret string `json:"secret"`
}

// Socks is a SOCKS5 proxy share link. User and pass come from the link itself.
type Socks struct {
	Server string `json:"server"`
	Port   int    `json:"port"`
	User   string `json:"user,omitempty"`
	Pass   string `json:"pass,omitempty"`
}

// Share is a t.me/share/url intent.
type Share struct {
	URL  string `json:"url"`
	Text string `json:"text,omitempty"`
}

// Story is one public story.
type Story struct {
	StoryID    int32  `json:"story_id"`
	Date       int32  `json:"date,omitempty"`
	ExpireDate int32  `json:"expire_date,omitempty"`
	Caption    string `json:"caption,omitempty"`
	MediaType  string `json:"media_type,omitempty"`
	MediaURL   string `json:"media_url,omitempty"`
	Pinned     bool   `json:"pinned"`

	mediaPhotoID    int64
	mediaDocumentID int64
}

func errorCard(code, path string) *Card {
	return &Card{OK: false, SchemaVersion: SchemaVersion, Error: code, Path: path}
}
