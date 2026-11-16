// Package chattitle limits chat and channel titles the way Telegram does:
// by Unicode characters, never by raw bytes. A byte slice can cut a Chinese
// character or an emoji in half, and MySQL then rejects the value.
package chattitle

import "strings"

// MaxRunes is the official title length (channels.createChannel / editTitle).
const MaxRunes = 128

// MaxAboutRunes is the official description length.
const MaxAboutRunes = 255

// Clip trims space and keeps at most MaxRunes characters.
func Clip(title string) string {
	return clip(strings.TrimSpace(title), MaxRunes)
}

// ClipAbout keeps at most MaxAboutRunes characters. It does not trim:
// a description may intentionally start or end with space, and an empty
// string is the documented way to clear it.
func ClipAbout(about string) string {
	return clip(about, MaxAboutRunes)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
