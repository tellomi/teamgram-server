package mtproto

import "math/rand"

// DiceFace is the value a dice message stores. Football and basketball
// are 1–5. The other built-in dice stay 1–6.
func DiceFace(emoticon string) int32 {
	switch emoticon {
	case "🏀", "⚽":
		return rand.Int31()%5 + 1
	default:
		return rand.Int31()%6 + 1
	}
}
