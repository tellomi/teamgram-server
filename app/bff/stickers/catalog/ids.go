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

func emojiFromCode(code string) string {
	v, err := strconv.ParseUint(code, 16, 32)
	if err != nil {
		return code
	}
	s := string(rune(v))
	switch strings.ToLower(code) {
	case "2764", "2705", "274c", "2615":
		return s + "\uFE0F"
	default:
		return s
	}
}
