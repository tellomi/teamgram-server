package core

import "github.com/teamgram/proto/mtproto"

// partMissOr keeps FILE_PART_<n>_MISSING when a caller would otherwise
// replace every read error with a generic media error. Other failures stay
// on the historical fallback.
func partMissOr(err, fallback error) error {
	if mtproto.IsFilePartMissing(err) {
		return err
	}
	if err == nil {
		return nil
	}
	return fallback
}
