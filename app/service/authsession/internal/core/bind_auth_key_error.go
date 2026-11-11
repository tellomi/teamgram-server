package core

import (
	"errors"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dao"
)

// mapBindAuthKeyUserError turns a bind failure into the mtproto error the
// client should see. The dao uses plain Go sentinels; NewRpcError maps
// those (and gRPC Unknown) to 500 INTERNAL, which official clients retry.
// AUTH_KEY_UNREGISTERED is the existing 401 for a key the caller may not
// use: not registered, or already bound to another user. The row for the
// current owner is left unchanged.
func mapBindAuthKeyUserError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, dao.ErrAuthKeyUserConflict) || errors.Is(err, dao.ErrAuthKeyNotRegistered) {
		return mtproto.ErrAuthKeyUnregistered
	}
	return err
}
