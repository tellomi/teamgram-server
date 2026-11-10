package catalog

import (
	"encoding/binary"
	"hash/fnv"
	"strconv"
	"strings"
)

func stableID(parts ...string) int64 {
	h := fnv.New64a()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	id := int64(h.Sum64() & 0x7fffffffffffffff)
	if id == 0 {
		return 1
	}
	return id
}

func hash64(ids []int64) int64 {
	h := fnv.New64a()
	var buf [8]byte
	for _, id := range ids {
		binary.LittleEndian.PutUint64(buf[:], uint64(id))
		_, _ = h.Write(buf[:])
	}
	out := int64(h.Sum64() & 0x7fffffffffffffff)
	if out == 0 {
		return 1
	}
	return out
}

// clientDocumentHashLimit is MediaDataController.calcDocumentsHash's default
// maxCount. getRecentStickers, getFavedStickers, getStickers, and
// getSavedGifs hash only the first 200 document ids.
const clientDocumentHashLimit = 200

// telegramHash is the list hash official clients compute.
// Android MediaDataController.calcHash (Java long >>>) and iOS
// combineInt64Hash (UInt64) both do, for each value v:
//
//	acc ^= acc >> 21
//	acc ^= acc << 35
//	acc ^= acc >> 4
//	acc += v
//
// Shifts are logical. An empty list is 0. A client hash of 0 still means
// "send the list"; callers keep that guard. Per-set StickerSet.hash stays
// on hash64.
func telegramHash(ids []int64) int64 {
	if len(ids) == 0 {
		return 0
	}
	var acc uint64
	for _, id := range ids {
		acc ^= acc >> 21
		acc ^= acc << 35
		acc ^= acc >> 4
		acc += uint64(id)
	}
	return int64(acc)
}

// telegramDocumentHash hashes at most the first 200 ids.
func telegramDocumentHash(ids []int64) int64 {
	if len(ids) > clientDocumentHashLimit {
		ids = ids[:clientDocumentHashLimit]
	}
	return telegramHash(ids)
}

func emojiFromCode(code string) string {
	v, err := strconv.ParseUint(code, 16, 32)
	if err != nil {
		return code
	}
	s := string(rune(v))
	switch strings.ToLower(code) {
	case "2615":
		// U+2615 is text-default. The fully-qualified hot beverage is
		// U+2615 U+FE0F. It is a sticker alt, not a reaction id.
		return s + "\uFE0F"
	default:
		// U+2764 is the bare heart. Official messages.getAvailableReactions
		// and both clients look up "❤", not "❤️". U+2705 and U+274C are
		// already fully-qualified without U+FE0F.
		return s
	}
}
