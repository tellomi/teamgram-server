// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package plugin

import (
	"unicode"

	"github.com/teamgram/proto/mtproto"
)

// textIndex maps a string to UTF-16 offsets. Telegram entity offsets are
// counted in UTF-16 code units, not bytes or runes.
type textIndex struct {
	runes []rune
	u16   []int // u16[i] is the UTF-16 offset of runes[i]; the last entry is the total length
}

func indexText(s string) textIndex {
	rs := []rune(s)
	u := make([]int, len(rs)+1)
	n := 0
	for i, r := range rs {
		u[i] = n
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	u[len(rs)] = n
	return textIndex{runes: rs, u16: u}
}

func (t textIndex) span(i, j int) (int32, int32, bool) {
	if i < 0 || j <= i || j >= len(t.u16) {
		return 0, 0, false
	}
	off := t.u16[i]
	end := t.u16[j]
	if end <= off {
		return 0, 0, false
	}
	return int32(off), int32(end - off), true
}

// isHashtagLetter matches Telegram's hashtag alphabet: letters, decimal
// digits, underscore, ZWNJ, middle dot, and the Sinhala block.
func isHashtagLetter(r rune) bool {
	if r == '_' || r == 0x200c || r == 0x00b7 || (r >= 0x0d80 && r <= 0x0dff) {
		return true
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isWordChar(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

func isASCIIWord(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}

// findHashtags returns hashtags under Telegram's rules: '#' followed by 1 to
// 256 hashtag letters, at least one of which is a letter, not glued to a
// preceding hashtag letter. '#' followed only by a space (or by nothing)
// yields no entity, so a Douyin share like "怎样# 来长沙" cannot produce a
// zero-length messageEntityHashtag.
func findHashtags(t textIndex) []*mtproto.MessageEntity {
	rs := t.runes
	var out []*mtproto.MessageEntity
	for i := 0; i < len(rs); i++ {
		if rs[i] != '#' {
			continue
		}
		if i > 0 && isHashtagLetter(rs[i-1]) {
			continue
		}
		j := i + 1
		count := 0
		wasLetter := false
		for j < len(rs) && count < 256 && isHashtagLetter(rs[j]) {
			if unicode.IsLetter(rs[j]) {
				wasLetter = true
			}
			j++
			count++
		}
		k := j
		for k < len(rs) && isHashtagLetter(rs[k]) {
			k++
		}
		if count < 1 || !wasLetter {
			continue
		}
		entityEnd := j
		// A @username suffix is part of the hashtag only when the body was
		// not truncated at 256 characters.
		if k == j && k < len(rs) && rs[k] == '@' {
			u := k + 1
			for u < len(rs) && isASCIIWord(rs[u]) {
				u++
			}
			ulen := u - (k + 1)
			if ulen >= 3 && ulen <= 32 {
				entityEnd = u
				k = u
			}
		}
		if k < len(rs) && rs[k] == '#' {
			continue
		}
		off, length, ok := t.span(i, entityEnd)
		if !ok || length <= 0 {
			continue
		}
		out = append(out, mtproto.MakeTLMessageEntityHashtag(&mtproto.MessageEntity{
			Offset: off,
			Length: length,
			// Hash tag search reads this field back out of stored JSON.
			// It is not part of the TL encoding of messageEntityHashtag.
			Url: string(rs[i:entityEnd]),
		}).To_MessageEntity())
	}
	return out
}

// findCashtags returns cashtags under Telegram's rules: '$' plus 1 to 8
// ASCII capitals (or the ticker 1INCH), not glued to a preceding hashtag
// letter. "$" or "$ " produces nothing.
func findCashtags(t textIndex) []*mtproto.MessageEntity {
	rs := t.runes
	var out []*mtproto.MessageEntity
	for i := 0; i < len(rs); i++ {
		if rs[i] != '$' {
			continue
		}
		if i > 0 && (isHashtagLetter(rs[i-1]) || rs[i-1] == '$') {
			continue
		}
		j := i + 1
		if j+5 <= len(rs) && string(rs[j:j+5]) == "1INCH" {
			j += 5
		} else {
			for j < len(rs) && j-(i+1) < 8 && rs[j] >= 'A' && rs[j] <= 'Z' {
				j++
			}
		}
		size := j - (i + 1)
		if size < 1 || size > 8 {
			continue
		}
		entityEnd := j
		ptr := j
		if j < len(rs) && rs[j] == '@' {
			u := j + 1
			for u < len(rs) && isASCIIWord(rs[u]) {
				u++
			}
			ulen := u - (j + 1)
			if ulen >= 3 && ulen <= 32 {
				entityEnd = u
				ptr = u
			}
		}
		if ptr < len(rs) && (isHashtagLetter(rs[ptr]) || rs[ptr] == '$') {
			continue
		}
		off, length, ok := t.span(i, entityEnd)
		if !ok || length <= 0 {
			continue
		}
		out = append(out, mtproto.MakeTLMessageEntityCashtag(&mtproto.MessageEntity{
			Offset: off,
			Length: length,
		}).To_MessageEntity())
	}
	return out
}

// findBotCommands returns /commands under Telegram's rules. The command body
// is 1 to 64 ASCII word characters, so a bare "/" is never an entity.
func findBotCommands(t textIndex) []*mtproto.MessageEntity {
	rs := t.runes
	var out []*mtproto.MessageEntity
	for i := 0; i < len(rs); i++ {
		if rs[i] != '/' {
			continue
		}
		if i > 0 && (isWordChar(rs[i-1]) || rs[i-1] == '/' || rs[i-1] == '<' || rs[i-1] == '>') {
			continue
		}
		j := i + 1
		for j < len(rs) && j-(i+1) < 64 && isASCIIWord(rs[j]) {
			j++
		}
		size := j - (i + 1)
		if size < 1 || size > 64 {
			continue
		}
		entityEnd := j
		if j < len(rs) && rs[j] == '@' {
			u := j + 1
			for u < len(rs) && isASCIIWord(rs[u]) {
				u++
			}
			ulen := u - (j + 1)
			// An @suffix of the wrong size invalidates the whole command.
			if ulen < 3 || ulen > 32 {
				continue
			}
			entityEnd = u
			j = u
		}
		if j < len(rs) && (isWordChar(rs[j]) || rs[j] == '/' || rs[j] == '<' || rs[j] == '>') {
			continue
		}
		off, length, ok := t.span(i, entityEnd)
		if !ok || length <= 0 {
			continue
		}
		out = append(out, mtproto.MakeTLMessageEntityBotCommand(&mtproto.MessageEntity{
			Offset: off,
			Length: length,
		}).To_MessageEntity())
	}
	return out
}
