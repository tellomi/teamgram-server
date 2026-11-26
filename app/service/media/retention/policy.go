package retention

import (
	"strings"
	"time"
)

// Kind is the media table a candidate came from.
type Kind string

const (
	KindDocument  Kind = "document"
	KindPhoto     Kind = "photo"
	KindEncrypted Kind = "encrypted"
)

// Subscription is one user who can still see the media. Premium and
// ExpireDate do not extend retention: a subscriber expires on the same
// upload clock as a free user. The fields stay so a caller can pass a
// premium holder and tests can prove Decide ignores it.
type Subscription struct {
	UserID     int64
	Premium    bool
	ExpireDate int64 // unix seconds; ignored
}

// Media is one candidate row.
type Media struct {
	Kind       Kind
	ID         int64 // document_id / photo_id / encrypted_file_id
	UploadedAt int64 // unix seconds of the original upload
	Size       int64
	MimeType   string
	Attributes string // documents.attributes JSON
	SizeTypes  []string
}

// Reason explains a decision. It is logged and counted.
type Reason string

const (
	ReasonYoung        Reason = "young"
	ReasonExempt       Reason = "exempt"
	ReasonUnreferenced Reason = "unreferenced"
	ReasonExpire       Reason = "expire"
)

// exemptAttrs are document attributes for sticker/GIF/emoji media.
var exemptAttrs = []string{
	"documentAttributeSticker",
	"documentAttributeCustomEmoji",
	"documentAttributeAnimated",
}

var exemptMimes = []string{
	"image/gif",
	"application/x-tgsticker",
	"application/x-tgwallpattern",
	"application/x-tgtheme",
}

// avatarSizes are the ABC sizes dfs writes only for profile and chat photos.
var avatarSizes = map[string]bool{"a": true, "b": true, "c": true, "d": true}

// Exempt reports whether m is never removed regardless of age: stickers,
// custom emoji, GIFs, wallpapers/themes and avatars.
func Exempt(m Media) bool {
	switch m.Kind {
	case KindDocument:
		mime := strings.ToLower(strings.TrimSpace(m.MimeType))
		for _, x := range exemptMimes {
			if mime == x {
				return true
			}
		}
		for _, a := range exemptAttrs {
			if strings.Contains(m.Attributes, `"`+a+`"`) {
				return true
			}
		}
	case KindPhoto:
		for _, t := range m.SizeTypes {
			if avatarSizes[t] {
				return true
			}
		}
	}
	return false
}

// Decide applies the policy to one media item. holders are the users that
// can still see it in some chat; nil/empty means nobody references it.
// Premium on a holder does not change the result.
func Decide(m Media, holders []Subscription, now time.Time, cfg Config) Reason {
	if now.Sub(time.Unix(m.UploadedAt, 0)) < cfg.Retention() {
		return ReasonYoung
	}
	if Exempt(m) {
		return ReasonExempt
	}
	if len(holders) == 0 {
		return ReasonUnreferenced
	}
	return ReasonExpire
}

// Objects lists the object keys (class, path) removed for an expired item.
// Thumbnails stay: photo sizes s (small), i (stripped) and j (path) and the
// document thumbs under photos/<type>/<id>.dat.
func Objects(m Media) [][2]string {
	id := itoa(m.ID)
	switch m.Kind {
	case KindDocument:
		return [][2]string{{"documents", id + ".dat"}}
	case KindPhoto:
		out := [][2]string{{"photos", "0/" + id + ".dat"}}
		for _, t := range []string{"m", "x", "y", "w"} {
			out = append(out, [2]string{"photos", t + "/" + id + ".dat"})
		}
		return out
	case KindEncrypted:
		return [][2]string{{"encryptedfiles", id + ".dat"}}
	}
	return nil
}

// FullPhotoSize reports whether thumb names a full-size photo that is gone
// after expiry. Small and stripped thumbs stay downloadable.
func FullPhotoSize(thumb string) bool {
	switch thumb {
	case "0", "m", "x", "y", "w":
		return true
	}
	return false
}
