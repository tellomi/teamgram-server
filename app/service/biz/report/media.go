package report

import (
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
)

// MediaKeys lists object keys for a message or story media value.
// Photos use photos/, videos use videos/, other documents use documents/.
func MediaKeys(media *mtproto.MessageMedia) []string {
	if media == nil {
		return nil
	}
	var keys []string
	if p := media.GetPhoto_FLAGPHOTO(); p != nil && p.GetId() != 0 {
		keys = append(keys, "photos/"+strconv.FormatInt(p.GetId(), 10))
	}
	if d := media.GetDocument(); d != nil && d.GetId() != 0 {
		class := "documents"
		if strings.HasPrefix(strings.ToLower(d.GetMimeType()), "video/") {
			class = "videos"
		}
		keys = append(keys, class+"/"+strconv.FormatInt(d.GetId(), 10))
	}
	return keys
}

// AvatarKey is the object key for a profile photo.
func AvatarKey(photoID int64) string {
	if photoID == 0 {
		return ""
	}
	return "avatars/" + strconv.FormatInt(photoID, 10)
}

// ContentFromMessage copies text and media keys off a stored message.
func ContentFromMessage(id int64, msg *mtproto.Message) Content {
	c := Content{Kind: "message", ID: id}
	if msg == nil {
		return c
	}
	if msg.GetId() != 0 {
		c.ID = int64(msg.GetId())
	}
	c.Text = msg.GetMessage()
	c.MediaKeys = MediaKeys(msg.GetMedia())
	return c
}

// ContentFromStory copies a caption and media keys off a story.
func ContentFromStory(item *mtproto.StoryItem) Content {
	c := Content{Kind: "story"}
	if item == nil {
		return c
	}
	c.ID = int64(item.GetId())
	if item.GetCaption() != nil {
		c.Text = item.GetCaption().GetValue()
	}
	c.MediaKeys = MediaKeys(item.GetMedia())
	return c
}
