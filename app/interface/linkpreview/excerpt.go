package linkpreview

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
)

const excerptRunes = 180

func excerptText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	fields := strings.FieldsFunc(s, unicode.IsSpace)
	s = strings.Join(fields, " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= excerptRunes {
		return s
	}
	return string(r[:excerptRunes-1]) + "…"
}

func capRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	if n < 1 {
		return ""
	}
	return string(r[:n])
}

// renderPost extracts the public text and media hint from a channel message.
// Contact phone numbers are not copied into the excerpt.
func renderPost(msg *mtproto.Message) (Post, bool) {
	if msg == nil {
		return Post{}, false
	}
	if msg.GetPredicateName() == mtproto.Predicate_messageEmpty || msg.GetPredicateName() == mtproto.Predicate_messageService {
		if strings.TrimSpace(msg.Message) == "" && msg.GetMedia() == nil {
			return Post{}, false
		}
	}
	if len(msg.RestrictionReason) > 0 {
		return Post{}, false
	}
	post := Post{MessageID: msg.Id, Date: msg.Date}
	if msg.Views != nil {
		v := msg.Views.GetValue()
		post.Views = &v
	}
	media := msg.GetMedia()
	post.Excerpt = excerptText(msg.Message)
	if media == nil || media.GetPredicateName() == "" || media.GetPredicateName() == mtproto.Predicate_messageMediaEmpty {
		if post.Excerpt == "" {
			return Post{}, false
		}
		return post, true
	}
	switch media.GetPredicateName() {
	case mtproto.Predicate_messageMediaPhoto:
		post.MediaType = "photo"
		if p := media.GetPhoto_FLAGPHOTO(); p != nil {
			post.mediaPhotoID = p.GetId()
		}
	case mtproto.Predicate_messageMediaDocument:
		post.MediaType = "document"
		if media.GetVideo_FLAGBOOLEAN() || media.GetRound() {
			post.MediaType = "video"
		} else if media.GetVoice() {
			post.MediaType = "voice"
		}
		if d := media.GetDocument(); d != nil {
			post.mediaDocumentID = d.GetId()
		}
	case mtproto.Predicate_messageMediaWebPage:
		post.MediaType = "webpage"
		if wp := media.GetWebpage(); wp != nil {
			if post.Excerpt == "" && wp.GetTitle() != nil {
				post.Excerpt = excerptText(wp.GetTitle().GetValue())
			}
			if p := wp.GetPhoto(); p != nil {
				post.mediaPhotoID = p.GetId()
			}
		}
	case mtproto.Predicate_messageMediaPoll:
		post.MediaType = "poll"
		if post.Excerpt == "" && media.GetPoll() != nil {
			q := media.GetPoll().GetQuestion()
			if q == "" {
				q = media.GetPoll().GetQuestion_STRING()
			}
			if q == "" && media.GetPoll().GetQuestion_TEXTWITHENTITIES() != nil {
				q = media.GetPoll().GetQuestion_TEXTWITHENTITIES().GetText()
			}
			post.Excerpt = excerptText(q)
		}
	case mtproto.Predicate_messageMediaGeo, mtproto.Predicate_messageMediaGeoLive, mtproto.Predicate_messageMediaVenue:
		post.MediaType = "geo"
		if post.Excerpt == "" {
			post.Excerpt = excerptText(media.GetTitle())
		}
	case mtproto.Predicate_messageMediaContact:
		post.MediaType = "contact"
		name := strings.TrimSpace(media.GetFirstName() + " " + media.GetLastName())
		if post.Excerpt == "" {
			post.Excerpt = excerptText(name)
		}
	default:
		post.MediaType = "other"
	}
	if post.Excerpt == "" && post.MediaType == "" {
		return Post{}, false
	}
	return post, true
}

func storyCaption(item *mtproto.StoryItem) string {
	if item == nil || item.GetCaption() == nil {
		return ""
	}
	return excerptText(item.GetCaption().GetValue())
}

func storyPublic(item *mtproto.StoryItem) bool {
	if item == nil {
		return false
	}
	if item.Public {
		return true
	}
	for _, r := range item.GetPrivacy() {
		if r.GetPredicateName() == mtproto.Predicate_privacyValueAllowAll {
			return true
		}
	}
	return false
}

func storyVisible(item *mtproto.StoryItem, deleted bool, now int32) bool {
	if deleted || item == nil || !storyPublic(item) {
		return false
	}
	if item.Pinned {
		return true
	}
	if item.ExpireDate == 0 {
		return true
	}
	return item.ExpireDate > now
}

// rulesArePublic reports whether a stored privacy JSON allows anonymous viewers.
// An empty value is the server default (allow all). Any other rule set that
// does not include allow-all is treated as private.
func rulesArePublic(raw string) bool {
	raw = strings.TrimSpace(raw)
	switch raw {
	case "", "null", "[]", "{}":
		return true
	}
	if strings.Contains(raw, "privacyValueAllowAll") {
		return true
	}
	return false
}
