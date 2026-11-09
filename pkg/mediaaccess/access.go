// Package mediaaccess checks that a caller presents the access_hash issued
// with a photo or document. Telegram keeps one hash per file and only
// returns it to users who can see the media. A matching hash is the
// capability: forwards, channel posts, and a second upload (its own id and
// hash) keep working. file_reference is checked only when this server has
// issued a non-empty one.
package mediaaccess

import (
	"bytes"
	"os"
	"strings"

	"github.com/teamgram/proto/mtproto"

	"github.com/zeromicro/go-zero/core/logx"
)

// EnvEnforce selects the mode. Unset, "enforce", "on", "1", and "true"
// reject a bad hash. "log" records the denial and allows the call so
// production can roll back without a new image. Any other value enforces.
const EnvEnforce = "TELLOMI_MEDIA_ACCESS_ENFORCE"

// LogOnly reports whether denials should be recorded and then allowed.
func LogOnly() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(EnvEnforce)), "log")
}

// Gate returns err. In log mode it records err and returns nil.
// The log line names the RPC error and not the access_hash.
func Gate(log logx.Logger, err error) error {
	if err == nil || !LogOnly() {
		return err
	}
	if log != nil {
		log.Errorf("media access: %v (%s=log)", err, EnvEnforce)
	} else {
		logx.Errorf("media access: %v (%s=log)", err, EnvEnforce)
	}
	return nil
}

// SameHash is an exact compare. Zero is not a wildcard.
func SameHash(got, want int64) bool {
	return got == want
}

// Reference checks file_reference when the server stored a non-empty one.
// An empty stored reference means the field is not issued; clients may send
// empty bytes. A mismatch is FILE_REFERENCE_EXPIRED, which is what official
// servers return for a reference the client must refresh.
func Reference(got, stored []byte) error {
	if len(stored) == 0 {
		return nil
	}
	if bytes.Equal(got, stored) {
		return nil
	}
	return mtproto.ErrFileReferenceExpired
}

// PhotoDownload authorizes inputPhotoFileLocation and the legacy photo
// location. web is a link-preview image whose issued access_hash is its id.
func PhotoDownload(id, got int64, gotRef []byte, found bool, stored int64, storedRef []byte, web bool) error {
	if id == 0 {
		return mtproto.ErrLocationInvalid
	}
	if web {
		if got != id {
			return mtproto.ErrFileIdInvalid
		}
		return nil
	}
	if !found || !SameHash(got, stored) {
		return mtproto.ErrFileIdInvalid
	}
	return Reference(gotRef, storedRef)
}

// DocumentDownload authorizes inputDocumentFileLocation. found is true when
// the id is a stored document, a public sticker or GIF, or another issued
// asset. A missing id and a wrong hash are both FILE_ID_INVALID: clients
// stop instead of refreshing an empty file_reference forever.
func DocumentDownload(id, got int64, gotRef []byte, found bool, stored int64, storedRef []byte) error {
	if id == 0 {
		return mtproto.ErrLocationInvalid
	}
	if !found || !SameHash(got, stored) {
		return mtproto.ErrFileIdInvalid
	}
	return Reference(gotRef, storedRef)
}

// EncryptedDownload authorizes inputEncryptedFileLocation.
func EncryptedDownload(id, got, stored int64, found bool) error {
	if id == 0 || !found || !SameHash(got, stored) {
		return mtproto.ErrFileIdInvalid
	}
	return nil
}

// ProfilePhoto authorizes inputPeerPhotoFileLocation for a user the caller
// can see. allowed is that user's profile photo ids.
func ProfilePhoto(photoID int64, allowed []int64, peerOK bool) error {
	if !peerOK {
		return mtproto.ErrPeerIdInvalid
	}
	if photoID == 0 {
		return mtproto.ErrLocationInvalid
	}
	for _, id := range allowed {
		if id == photoID {
			return nil
		}
	}
	return mtproto.ErrLocationInvalid
}

// ChatPhoto authorizes a basic-group avatar. inputPeerChat has no
// access_hash, so the caller must be a current member and the photo id
// must be the group's photo.
func ChatPhoto(photoID, chatPhoto int64, member bool) error {
	if !member {
		return mtproto.ErrPeerIdInvalid
	}
	if photoID == 0 || photoID != chatPhoto {
		return mtproto.ErrLocationInvalid
	}
	return nil
}

// ChannelPhoto authorizes a channel or supergroup avatar. The peer
// access_hash must be the one issued for that channel, and photoID must be
// its current photo.
func ChannelPhoto(photoID, channelPhoto, gotHash, storedHash int64, found bool) error {
	if !found || !SameHash(gotHash, storedHash) {
		return mtproto.ErrChannelInvalid
	}
	if photoID == 0 || photoID != channelPhoto {
		return mtproto.ErrLocationInvalid
	}
	return nil
}

// MismatchAllowed is the send/upload side of log mode. A wrong hash is
// MEDIA_INVALID while enforcing. In log mode the caller may continue and
// must attach the stored object (its real access_hash), not the hash the
// client invented.
func MismatchAllowed(kind string, id int64) bool {
	if !LogOnly() {
		return false
	}
	logx.Errorf("media access: %s %d rejected hash (%s=log)", kind, id, EnvEnforce)
	return true
}
