package secretchat

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// Materialize stores an inputEncryptedFileUploaded the same way
// messages.uploadEncryptedFile does. The files service installs it.
// Nil keeps the previous echo of id and access hash with size 0.
var Materialize func(ctx context.Context, creator int64, chatID int32, file *mtproto.InputEncryptedFile) (*mtproto.EncryptedFile, error)
