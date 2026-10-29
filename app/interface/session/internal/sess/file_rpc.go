package sess

import (
	"time"

	"github.com/teamgram/proto/mtproto"
)

// fileRPCTimeout is how long a file RPC may run. It is shorter than a stuck
// S3 read, and file RPCs run off the session's main RPC loop so one download
// cannot hold updates.getDifference or a send.
const fileRPCTimeout = 3 * time.Second

func isFileRPC(msg mtproto.TLObject) bool {
	switch msg.(type) {
	case *mtproto.TLUploadGetFile,
		*mtproto.TLUploadGetFileHashes,
		*mtproto.TLUploadSaveFilePart,
		*mtproto.TLUploadSaveBigFilePart:
		return true
	default:
		return false
	}
}
