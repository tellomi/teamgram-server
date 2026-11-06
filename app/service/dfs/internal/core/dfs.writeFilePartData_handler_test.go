package core

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/svc"

	"github.com/teamgram/marmota/pkg/stores/kv"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/redis/redistest"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func newPartCore(t *testing.T) *DfsCore {
	t.Helper()
	rds := redistest.CreateRedis(t)
	store := kv.NewStore(kv.KvConf{{
		RedisConf: redis.RedisConf{Host: rds.Addr, Type: redis.NodeType},
		Weight:    100,
	}})
	return New(context.Background(), &svc.ServiceContext{Dao: dao.NewDaoWithStore(store)})
}

func partBytes(n int, b byte) []byte {
	return bytes.Repeat([]byte{b}, n)
}

func writePart(t *testing.T, c *DfsCore, id int64, part int32, body []byte, total int32) {
	t.Helper()
	in := &dfs.TLDfsWriteFilePartData{
		Creator:  7,
		FileId:   id,
		FilePart: part,
		Bytes:    body,
	}
	if total > 0 {
		in.Big = true
		in.FileTotalParts = wrapperspb.Int32(total)
	}
	ok, err := c.DfsWriteFilePartData(in)
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("write part %d (%d bytes, total %d): %v", part, len(body), total, err)
	}
}

func rpcMessage(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}

func TestClientPartSizesMatchCheckFilePartSize(t *testing.T) {
	// Live Tellomi clients send these non-last sizes. They must keep passing
	// the helper; this test is the stop-and-report check from the upload task.
	for _, kib := range []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512} {
		if err := model.CheckFilePartSize(int32(kib * 1024)); err != nil {
			t.Fatalf("%d KiB rejected by CheckFilePartSize: %v", kib, err)
		}
	}
	if err := model.CheckFilePartSize(1024); err != nil {
		t.Fatalf("1024-byte streaming part: %v", err)
	}
}

func TestWriteFilePartRejectsEmptyAndIllegalSizes(t *testing.T) {
	c := newPartCore(t)
	const id int64 = 41

	_, err := c.DfsWriteFilePartData(&dfs.TLDfsWriteFilePartData{Creator: 7, FileId: id, FilePart: 0, Bytes: nil})
	if err != mtproto.ErrFilePartEmpty {
		t.Fatalf("empty: %v", err)
	}
	_, err = c.DfsWriteFilePartData(&dfs.TLDfsWriteFilePartData{
		Creator: 7, FileId: id, FilePart: 2901, Bytes: partBytes(1024, 1),
		FileTotalParts: wrapperspb.Int32(3000), Big: true,
	})
	if err != mtproto.ErrFilePartInvalid {
		t.Fatalf("part 2901: %v", err)
	}

	nonLast := func(n int, part int32) error {
		_, err := c.DfsWriteFilePartData(&dfs.TLDfsWriteFilePartData{
			Creator: 7, FileId: id, FilePart: part, Bytes: partBytes(n, 2),
			Big: true, FileTotalParts: wrapperspb.Int32(4),
		})
		return err
	}
	if rpcMessage(nonLast(1000, 0)) != "FILE_PART_LENGTH_INVALID" {
		t.Fatalf("1000 bytes: %v", nonLast(1000, 0))
	}
	// 100 KiB is a multiple of 1024 and does not divide 512 KiB.
	if got := rpcMessage(nonLast(100*1024, 1)); got != "FILE_PART_SIZE_INVALID" {
		t.Fatalf("100 KiB: %s", got)
	}
	// 600 KiB is over the cap. CheckFilePartSize reports SIZE_INVALID, not
	// TOO_BIG, because 524288 % partSize != 0 is tested first.
	if got := rpcMessage(nonLast(600*1024, 2)); got != status.Convert(model.CheckFilePartSize(600*1024)).Message() {
		t.Fatalf("600 KiB: %s", got)
	}

	// Nothing from the rejected writes may be assembled later.
	_, err = c.svcCtx.Dao.OpenFile(context.Background(), 7, id, 1)
	if rpcMessage(err) != "FILE_PART_0_MISSING" {
		t.Fatalf("rejected writes were stored: %v", err)
	}
}

func TestWriteFilePartAcceptsStandardAndShortLastParts(t *testing.T) {
	c := newPartCore(t)

	for _, kib := range []int{16, 32, 128, 256, 512} {
		id := int64(1000 + kib)
		writePart(t, c, id, 0, partBytes(kib*1024, byte(kib)), 2)
	}

	// One-part saveFilePart: the whole file is the remainder. 50 KiB does
	// not divide 512 KiB, and there is no file_total_parts.
	writePart(t, c, 50, 0, partBytes(50*1024, 'J'), 0)

	// Big-file remainder after 128 KiB chunks.
	writePart(t, c, 72, 0, partBytes(128*1024, 'A'), 2)
	writePart(t, c, 72, 1, partBytes(72*1024, 'B'), 2)

	// A remainder may not exceed 512 KiB.
	_, err := c.DfsWriteFilePartData(&dfs.TLDfsWriteFilePartData{
		Creator: 7, FileId: 99, FilePart: 0, Bytes: partBytes(600*1024, 'Z'),
		Big: true, FileTotalParts: wrapperspb.Int32(1),
	})
	if err != mtproto.ErrFilePartTooBig {
		t.Fatalf("oversized only-part: %v", err)
	}

	got, err := c.svcCtx.Dao.OpenFile(context.Background(), 7, 72, 2)
	if err != nil {
		t.Fatal(err)
	}
	body, err := got.ReadAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := append(partBytes(128*1024, 'A'), partBytes(72*1024, 'B')...)
	if !bytes.Equal(body, want) {
		t.Fatalf("assembled %d bytes, want %d", len(body), len(want))
	}
}

func TestWriteFilePartOutOfOrder(t *testing.T) {
	c := newPartCore(t)
	const id int64 = 77
	// Last remainder first, then part 0, then the middle part.
	writePart(t, c, id, 2, partBytes(100, 'C'), 3)
	writePart(t, c, id, 0, partBytes(16*1024, 'A'), 3)
	writePart(t, c, id, 1, partBytes(16*1024, 'B'), 3)

	r, err := c.svcCtx.Dao.OpenFile(context.Background(), 7, id, 3)
	if err != nil {
		t.Fatal(err)
	}
	body, err := r.ReadAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := append(append(partBytes(16*1024, 'A'), partBytes(16*1024, 'B')...), partBytes(100, 'C')...)
	if !bytes.Equal(body, want) {
		t.Fatalf("out of order assemble %d bytes, want %d", len(body), len(want))
	}
}

func TestWriteFilePartParallel(t *testing.T) {
	c := newPartCore(t)
	const id int64 = 88
	var wg sync.WaitGroup
	errc := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := c.DfsWriteFilePartData(&dfs.TLDfsWriteFilePartData{
			Creator: 7, FileId: id, FilePart: 1, Bytes: partBytes(32*1024, 'B'),
			Big: true, FileTotalParts: wrapperspb.Int32(2),
		})
		errc <- err
	}()
	go func() {
		defer wg.Done()
		_, err := c.DfsWriteFilePartData(&dfs.TLDfsWriteFilePartData{
			Creator: 7, FileId: id, FilePart: 0, Bytes: partBytes(32*1024, 'A'),
			Big: true, FileTotalParts: wrapperspb.Int32(2),
		})
		errc <- err
	}()
	wg.Wait()
	close(errc)
	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err := c.svcCtx.Dao.OpenFile(context.Background(), 7, id, 2)
	if err != nil {
		t.Fatal(err)
	}
	body, err := r.ReadAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := append(partBytes(32*1024, 'A'), partBytes(32*1024, 'B')...)
	if !bytes.Equal(body, want) {
		t.Fatalf("parallel assemble %d bytes, want %d", len(body), len(want))
	}
}

func TestFinalizeMissingPartDoesNotAssemble(t *testing.T) {
	c := newPartCore(t)
	ctx := context.Background()

	// Non-big upload: the saved hash has parts 0 and 1, and the client
	// declares 3. Counting fields would hide the missing tail and assemble
	// a short object.
	writePart(t, c, 1, 0, partBytes(16*1024, 'A'), 0)
	writePart(t, c, 1, 1, partBytes(100, 'B'), 0)
	if _, err := c.svcCtx.Dao.OpenFile(ctx, 7, 1, 3); rpcMessage(err) != "FILE_PART_2_MISSING" {
		t.Fatalf("missing tail: %v", err)
	}
	photo, err := c.DfsUploadPhotoFileV2(&dfs.TLDfsUploadPhotoFileV2{
		Creator: 7,
		File: mtproto.MakeTLInputFile(&mtproto.InputFile{
			Id_INT64: 1, Parts: 3, Name: "a.jpg",
		}).To_InputFile(),
	})
	if photo != nil || rpcMessage(err) != "FILE_PART_2_MISSING" {
		t.Fatalf("photo finalize photo=%v err=%v", photo, err)
	}

	// The two parts that did arrive still assemble, with the short remainder
	// intact and without zero padding out to the standard part size.
	r, err := c.svcCtx.Dao.OpenFile(ctx, 7, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	body, err := r.ReadAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := append(partBytes(16*1024, 'A'), partBytes(100, 'B')...)
	if !bytes.Equal(body, want) {
		t.Fatalf("short file %d bytes, want %d", len(body), len(want))
	}

	// Middle hole with a known total. ReadAll must not splice in empty bytes.
	writePart(t, c, 2, 0, partBytes(16*1024, 'A'), 3)
	writePart(t, c, 2, 2, partBytes(80, 'C'), 3)
	r, err = c.svcCtx.Dao.OpenFile(ctx, 7, 2, 3)
	if r != nil || rpcMessage(err) != "FILE_PART_1_MISSING" {
		t.Fatalf("missing middle reader=%v err=%v", r, err)
	}
	info, err := c.svcCtx.Dao.GetFileInfo(ctx, 7, 2)
	if err != nil {
		t.Fatal(err)
	}
	body, err = c.svcCtx.Dao.NewSSDBReader(info).ReadAll(ctx)
	if body != nil || rpcMessage(err) != "FILE_PART_1_MISSING" {
		t.Fatalf("ReadAll body=%d err=%v", len(body), err)
	}
}

func TestPartMissOrKeepsOfficialError(t *testing.T) {
	miss := mtproto.NewFilePartXMissing(4)
	if got := partMissOr(miss, mtproto.ErrMediaInvalid); rpcMessage(got) != "FILE_PART_4_MISSING" {
		t.Fatalf("kept %v", got)
	}
	if got := partMissOr(mtproto.ErrInputRequestInvalid, mtproto.ErrMediaInvalid); got != mtproto.ErrMediaInvalid {
		t.Fatalf("fallback %v", got)
	}
}
