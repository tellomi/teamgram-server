package premium

// StoreColor maps a client color id onto the users.color column.
// The column uses 0 for "no color". Color id 0 is a real palette entry,
// so it is stored as 1. A negative id clears the color.
func StoreColor(colorID int32) int32 {
	if colorID < 0 {
		return 0
	}
	return colorID + 1
}

// LoadColor reverses StoreColor.
func LoadColor(stored int32) (colorID int32, set bool) {
	if stored <= 0 {
		return 0, false
	}
	return stored - 1, true
}
