package sess

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestFileRPCIsIsolatedFromOtherMethods(t *testing.T) {
	if !isFileRPC(&mtproto.TLUploadGetFile{}) || !isFileRPC(&mtproto.TLUploadSaveFilePart{}) {
		t.Fatal("file methods should leave the main RPC loop")
	}
	if isFileRPC(&mtproto.TLMessagesSendMessage{}) || isFileRPC(&mtproto.TLUpdatesGetDifference{}) || isFileRPC(&mtproto.TLUpdatesGetState{}) {
		t.Fatal("send and update methods must stay on the main RPC loop")
	}
	if fileRPCTimeout != 12*time.Second {
		t.Fatalf("file RPC timeout = %s, want 12s", fileRPCTimeout)
	}
	if cap(fileRPCSlots) != fileRPCConcurrency || fileRPCConcurrency < 1 {
		t.Fatalf("file RPC concurrency cap = %d", cap(fileRPCSlots))
	}
	if rpcTimeout(&mtproto.TLUpdatesGetDifference{}) != defaultRPCTimeout {
		t.Fatal("getDifference must use the normal RPC deadline")
	}
	if rpcTimeout(&mtproto.TLUploadGetFile{}) != fileRPCTimeout {
		t.Fatal("getFile must use the file RPC deadline")
	}
}

func TestRPCDeadlineReturnsInternal(t *testing.T) {
	req := &rpcApiMessage{reqMsgId: 7}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		time.Sleep(40 * time.Millisecond)
		req.setResult(&mtproto.TLRpcResult{ReqMsgId: 7, Result: mtproto.BoolTrue})
	}()
	select {
	case <-done:
		t.Fatal("handler finished before the deadline branch")
	case <-ctx.Done():
		req.setResult(internalRPCResult(req.reqMsgId))
	}
	if time.Since(started) > time.Second {
		t.Fatal("deadline wait hung")
	}
	rpcErr, ok := req.rpcResult.Result.(*mtproto.TLRpcError)
	if !ok || rpcErr.GetErrorCode() != 500 || rpcErr.GetErrorMessage() != "INTERNAL" {
		t.Fatalf("result = %#v", req.rpcResult.Result)
	}
	<-done
	rpcErr, ok = req.rpcResult.Result.(*mtproto.TLRpcError)
	if !ok || rpcErr.GetErrorMessage() != "INTERNAL" {
		t.Fatal("late handler overwrote the deadline error")
	}
}
