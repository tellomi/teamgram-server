// Package diffstrict gates updates.getDifference corrections.
//
// TELLOMI_DIFFERENCE_STRICT defaults to off. Login and session sync keep
// the historical difference shape until an operator sets the variable to 1.
package diffstrict

import "os"

// Enabled is true only when TELLOMI_DIFFERENCE_STRICT=1.
func Enabled() bool {
	return os.Getenv("TELLOMI_DIFFERENCE_STRICT") == "1"
}

// TooLong reports that the pts gap cannot be closed by this response.
// A gap that fits in limit stays a normal difference or an empty one.
// fetched is how many updates this call loaded. Zero with a gap past the
// limit means the pts log no longer contains the client's position.
func TooLong(clientPts, serverPts, limit int32, fetched int) bool {
	if serverPts <= clientPts {
		return false
	}
	if limit <= 0 {
		limit = 5000
	}
	gap := serverPts - clientPts
	if gap <= limit {
		return false
	}
	return fetched == 0 || fetched >= int(limit)
}
