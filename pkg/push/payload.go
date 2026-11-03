package push

import (
	"encoding/json"
	"strconv"
)

// Alert is one APNs payload plus the JSON sealed inside "p".
type Alert struct {
	// Outer is the JSON object POSTed to APNs. It carries aps.alert loc-key /
	// loc-args, a localized body fallback, mutable-content, and the custom
	// from_id / chat_id / msg_id fields.
	Outer []byte
	// Inner is the JSON the NotificationService reads after decrypting "p".
	// It has no loc-key: the extension only renders aps.alert when loc-key is
	// absent (see NotificationService.swift). The title and body are already
	// in the recipient's language.
	Inner []byte
	// Android is the FCM data object. Every value is a string, which is what
	// the FCM HTTP API requires. loc_key / loc_args follow
	// https://core.telegram.org/api/push-updates. "p" is that same object
	// sealed, which is the only field tellomi/Telegram reads.
	Android []byte
	// Huawei is the raw "p" string. HuaweiPushListenerService passes the whole
	// data payload to the decryptor, not a JSON wrapper. Empty without a secret.
	Huawei string
}

// BuildAlert builds the Telegram notification for one message.
// lang is the recipient device's lang_code (initConnection / registerDevice).
// secret may be nil; then Outer has no "p" and the extension keeps the
// outer loc-key text only after its time limit. A 256-byte secret produces "p".
func BuildAlert(m Incoming, d Decision, secret []byte, lang string) (Alert, error) {
	if !d.Preview {
		m.Text = ""
	}
	key := m.LocKeyFor(d.Preview)
	args := m.Args(d.Preview)
	title, subtitle, body := alertText(m, key, lang, d.Preview)
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
		// body is the localized fallback. Apple shows loc-key when that string
		// exists and ignores body; the Notification Service shows this text
		// from the decrypted payload, which is what a Chinese user actually sees.
		"body": body,
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
	android, huawei, err := androidPayload(m, d, key, args, body, custom, secret)
	if err != nil {
		return Alert{}, err
	}
	return Alert{Outer: outerJSON, Inner: innerJSON, Android: android, Huawei: huawei}, nil
}

// androidPayload is the object tellomi/Telegram decrypts from "p": loc_key and
// loc_args (underscore), plus custom. The bytes returned are an FCM data map
// whose values are all strings. Huawei is that same ciphertext alone.
func androidPayload(m Incoming, d Decision, key string, args []string, body string, custom map[string]string, secret []byte) ([]byte, string, error) {
	customObj := map[string]any{}
	for k, v := range custom {
		customObj[k] = v
	}
	if d.Silent {
		customObj["silent"] = 1
	}
	if m.Mentioned {
		customObj["mention"] = 1
	}
	plain := map[string]any{
		"loc_key":  key,
		"loc_args": args,
		"custom":   customObj,
		"user_id":  m.UserID,
		"body":     body,
	}
	if !d.Silent && d.Sound != "" {
		plain["sound"] = d.Sound
	}
	plainJSON, err := json.Marshal(plain)
	if err != nil {
		return nil, "", err
	}
	var p string
	if len(secret) >= 128 {
		p, err = EncryptNotification(secret, plainJSON)
		if err != nil {
			return nil, "", err
		}
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, "", err
	}
	customJSON, err := json.Marshal(customObj)
	if err != nil {
		return nil, "", err
	}
	data := map[string]string{
		"loc_key":  key,
		"loc_args": string(argsJSON),
		"custom":   string(customJSON),
		"user_id":  strconv.FormatInt(m.UserID, 10),
		"body":     body,
	}
	if !d.Silent && d.Sound != "" {
		data["sound"] = d.Sound
	}
	if p != "" {
		data["p"] = p
	}
	out, err := json.Marshal(data)
	if err != nil {
		return nil, "", err
	}
	return out, p, nil
}

func alertText(m Incoming, key, lang string, preview bool) (title, subtitle, body string) {
	author := m.SenderName
	if author == "" {
		author = "User"
	}
	chat := m.ChatTitle
	if chat == "" {
		chat = "Chat"
	}
	switch {
	case m.ChannelID != 0:
		title = chat
	case m.ChatID != 0:
		title, subtitle = chat, author
	default:
		title = author
	}
	body = formatPushBody(lang, key, m.Args(preview), title, subtitle)
	return title, subtitle, body
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
