package sess

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// fileRPCTimeout is how long a file RPC may run off the session's main RPC
// loop. 3s was tight for a cold object-store read and made clients retry
// upload.getFile forever. 12s still returns before the client gives up, and
// the call cannot hold updates.getDifference or a send.
const fileRPCTimeout = 12 * time.Second

// fileRPCConcurrency bounds how many file RPCs one process runs at once.
// They already leave the session loop; without a cap a stuck client can
// start an unbounded number of object-store reads.
const fileRPCConcurrency = 32

var fileRPCSlots = make(chan struct{}, fileRPCConcurrency)

// defaultRPCTimeout is the deadline for every non-file RPC. A handler that
// never returns becomes rpc_error 500 INTERNAL, which clients retry. Silence
// leaves the UI on "Updating...".
const defaultRPCTimeout = 10 * time.Second

func acquireFileSlot(ctx context.Context) bool {
	select {
	case fileRPCSlots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

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
