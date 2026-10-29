package push

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Alert is one APNs payload plus the JSON sealed inside "p".
type Alert struct {
	// Outer is the JSON object POSTed to APNs. It carries aps.alert loc-key /
	// loc-args, mutable-content, and the custom from_id / chat_id / msg_id fields.
	Outer []byte
	// Inner is the JSON the NotificationService reads after decrypting "p".
	// It has no loc-key: the extension only renders aps.alert when loc-key is
	// absent (see NotificationService.swift).
	Inner []byte
}

// BuildAlert builds the Telegram-iOS notification for one message.
// secret may be nil; then Outer has no "p" and the extension keeps the
// outer loc-key text only after its time limit. A 256-byte secret produces "p".
func BuildAlert(m Incoming, d Decision, secret []byte) (Alert, error) {
	if !d.Preview {
		m.Text = ""
	}
	key := m.LocKeyFor(d.Preview)
	args := m.Args(d.Preview)
	title, subtitle, body := alertText(m, d.Preview)
	thread := threadID(m)
	custom := customIDs(m)

	innerAPS := map[string]any{
		"alert": map[string]any{
			"title": title,
			"body":  body,
		},
		"mutable-content": 1,
		"thread-id":       thread,
		"category":        "m",
	}
	if subtitle != "" {
		innerAPS["alert"].(map[string]any)["subtitle"] = subtitle
	}
	if !d.Silent && d.Sound != "" {
		innerAPS["sound"] = d.Sound
	}
	inner := map[string]any{
		"aps":     innerAPS,
		"msg_id":  strconv.FormatInt(int64(m.MsgID), 10),
		"custom":  custom,
		"user_id": m.UserID,
	}
	for k, v := range custom {
		inner[k] = v
	}
	if d.Silent {
		inner["silent"] = "1"
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return Alert{}, err
	}

	outerAlert := map[string]any{
		"loc-key":  key,
		"loc-args": args,
	}
	outerAPS := map[string]any{
		"alert":           outerAlert,
		"mutable-content": 1,
		"thread-id":       thread,
	}
	if !d.Silent && d.Sound != "" {
		outerAPS["sound"] = d.Sound
	}
	outer := map[string]any{
		"aps":     outerAPS,
		"msg_id":  strconv.FormatInt(int64(m.MsgID), 10),
		"custom":  custom,
		"user_id": m.UserID,
	}
	for k, v := range custom {
		outer[k] = v
	}
	if len(secret) >= 128 {
		p, err := EncryptNotification(secret, innerJSON)
		if err != nil {
			return Alert{}, err
		}
		outer["p"] = p
	}
	outerJSON, err := json.Marshal(outer)
	if err != nil {
		return Alert{}, err
	}
	return Alert{Outer: outerJSON, Inner: innerJSON}, nil
}

func alertText(m Incoming, preview bool) (title, subtitle, body string) {
	author := m.SenderName
	if author == "" {
		author = "User"
	}
	chat := m.ChatTitle
	if chat == "" {
		chat = "Chat"
	}
	body = previewBody(m, preview)
	switch {
	case m.ChannelID != 0:
		return chat, "", body
	case m.ChatID != 0:
		return chat, author, body
	default:
		return author, "", body
	}
}

func previewBody(m Incoming, preview bool) string {
	if preview && m.Text != "" && strings.HasSuffix(m.LocKey, "MESSAGE_TEXT") {
		return m.Text
	}
	switch {
	case strings.HasSuffix(m.LocKey, "MESSAGE_PHOTO"):
		return "sent you a photo"
	case strings.HasSuffix(m.LocKey, "MESSAGE_VIDEO"):
		return "sent you a video"
	case strings.HasSuffix(m.LocKey, "MESSAGE_GIF"):
		return "sent you a GIF"
	case strings.HasSuffix(m.LocKey, "MESSAGE_ROUND"):
		return "sent you a video message"
	case strings.HasSuffix(m.LocKey, "MESSAGE_AUDIO"):
		return "sent you a voice message"
	case strings.HasSuffix(m.LocKey, "MESSAGE_STICKER"):
		return "sent you a sticker"
	case strings.HasSuffix(m.LocKey, "MESSAGE_DOC"):
		return "sent you a file"
	case strings.HasSuffix(m.LocKey, "MESSAGE_GEO"):
		return "sent you a map"
	case strings.HasSuffix(m.LocKey, "MESSAGE_GEOLIVE"):
		return "started sharing their live location"
	case strings.HasSuffix(m.LocKey, "MESSAGE_CONTACT"):
		return "shared a contact"
	case strings.HasSuffix(m.LocKey, "MESSAGE_POLL"):
		return "sent you a poll"
	default:
		return "sent you a message"
	}
}

func threadID(m Incoming) string {
	switch {
	case m.ChannelID != 0:
		return strconv.FormatInt(m.ChannelID, 10)
	case m.ChatID != 0:
		return strconv.FormatInt(m.ChatID, 10)
	default:
		return strconv.FormatInt(m.SenderID, 10)
	}
}

// customIDs are strings. The NotificationService reads msg_id / from_id /
// chat_id / channel_id with `as? String`, and from_id wins over chat_id, so a
// group payload must not also set from_id.
func customIDs(m Incoming) map[string]string {
	msg := strconv.FormatInt(int64(m.MsgID), 10)
	out := map[string]string{"msg_id": msg}
	switch {
	case m.ChannelID != 0:
		out["channel_id"] = strconv.FormatInt(m.ChannelID, 10)
	case m.ChatID != 0:
		out["chat_id"] = strconv.FormatInt(m.ChatID, 10)
		if m.SenderID != 0 {
			out["chat_from_id"] = strconv.FormatInt(m.SenderID, 10)
		}
	default:
		out["from_id"] = strconv.FormatInt(m.SenderID, 10)
	}
	return out
}
