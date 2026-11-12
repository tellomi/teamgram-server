package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/svc"
	dfsclient "github.com/teamgram/teamgram-server/app/service/dfs/client"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/pkg/uploadledger"
)

type partSink struct {
	dfsclient.DfsClient
	written int
	fail    bool
}

func (s *partSink) DfsWriteFilePartData(context.Context, *dfs.TLDfsWriteFilePartData) (*mtproto.Bool, error) {
	if s.fail {
		return nil, context.DeadlineExceeded
	}
	return mtproto.BoolTrue, nil
}

func TestIndependentSavedPartKeepsTheHold(t *testing.T) {
	led := uploadledger.New(uploadledger.Limits{
		Network: 1000,
		Hold:    50,
		Entity:  1000,
		Window:  1 << 30,
	})
	uploadledger.Use(led)
	t.Cleanup(func() { uploadledger.Use(nil) })
	sink := &partSink{}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{DfsClient: sink}})
	c.MD = &metadata.RpcMetadata{UserId: 7, PermAuthKeyId: 9}
	body := make([]byte, 40)
	if _, err := c.UploadSaveFilePart(&mtproto.TLUploadSaveFilePart{FileId: 1, FilePart: 0, Bytes: body}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UploadSaveFilePart(&mtproto.TLUploadSaveFilePart{FileId: 1, FilePart: 1, Bytes: body}); err == nil {
		t.Fatal("wrote 80 bytes through a 50 byte hold")
	}
	if got := led.Book(7).Hold; got != 40 {
		t.Fatalf("hold %d, stored parts should stay reserved", got)
	}
}

func TestIndependentHandlerReleaseIsPerFile(t *testing.T) {
	led := uploadledger.New(uploadledger.Limits{Network: 1000, Hold: 1000, Entity: 1000})
	uploadledger.Use(led)
	t.Cleanup(func() { uploadledger.Use(nil) })
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{DfsClient: &partSink{}}})
	c.MD = &metadata.RpcMetadata{UserId: 7, PermAuthKeyId: 9}
	if _, err := c.UploadSaveFilePart(&mtproto.TLUploadSaveFilePart{FileId: 1, Bytes: make([]byte, 100)}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UploadSaveFilePart(&mtproto.TLUploadSaveFilePart{FileId: 2, Bytes: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := uploadledger.NoteClaim(7, 2, false, 1); err != nil {
		t.Fatal(err)
	}
	if got := led.Book(7).Hold; got != 100 {
		t.Fatalf("assembling the 1 byte file cleared hold %d", got)
	}
}
