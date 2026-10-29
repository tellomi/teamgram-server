package core

import (
	"errors"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/langpack"
)

func rpcErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, langpack.ErrUnknownPack):
		return mtproto.ErrLangPackInvalid
	case errors.Is(err, langpack.ErrUnknownLang):
		return mtproto.ErrLangCodeNotSupported
	default:
		return err
	}
}
