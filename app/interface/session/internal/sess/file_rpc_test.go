package sess

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestFileRPCIsIsolatedFromOtherMethods(t *testing.T) {
	if !isFileRPC(&mtproto.TLUploadGetFile{}) || !isFileRPC(&mtproto.TLUploadSaveFilePart{}) {
		t.Fatal("file methods should leave the main RPC loop")
	}
	if isFileRPC(&mtproto.TLMessagesSendMessage{}) || isFileRPC(&mtproto.TLUpdatesGetDifference{}) || isFileRPC(&mtproto.TLUpdatesGetState{}) {
		t.Fatal("send and update methods must stay on the main RPC loop")
	}
}
