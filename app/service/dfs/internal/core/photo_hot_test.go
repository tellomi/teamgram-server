package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teamgram/marmota/pkg/stores/kv"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/svc"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/redis/redistest"
)

func TestPhotoSizePutsKeepsEveryLookupKeyOnTheHotTier(t *testing.T) {
	body := []byte("jpeg")
	record, puts := photoSizePuts(9, "y", "y/9.dat", body, body, "0/9.dat", false, true)
	if record != "0/9.dat" {
		t.Fatalf("record %s", record)
	}
	if len(puts) != 1 || puts[0].path != "y/9.dat" || string(puts[0].body) != "jpeg" {
		t.Fatalf("hot alias puts %+v", puts)
	}

	record, puts = photoSizePuts(9, "y", "y/9.dat", body, body, "0/9.dat", false, false)
	if record != "0/9.dat" || len(puts) != 0 {
		t.Fatalf("cold alias record %s puts %+v", record, puts)
	}

	record, puts = photoSizePuts(9, "m", "m/9.dat", []byte("medium"), []byte("orig"), "0/9.dat", false, true)
	if record != "m/9.dat" || len(puts) != 1 || puts[0].path != "m/9.dat" {
		t.Fatalf("derived record %s puts %+v", record, puts)
	}

	record, puts = photoSizePuts(9, "m", "m/3.dat", []byte("medium"), []byte("orig"), "0/3.dat", true, true)
	if record != "m/3.dat" || len(puts) != 0 {
		t.Fatalf("shared hot record %s puts %+v", record, puts)
	}
	if _, cold := photoSizePuts(9, "m", "m/3.dat", []byte("medium"), nil, "", true, false); len(cold) != 0 {
		t.Fatalf("shared cold puts %+v", cold)
	}
}

func TestPhotoDownloadPathsTriesRecordedThenConventional(t *testing.T) {
	got := photoDownloadPaths("0/9.dat", "y", 9)
	if len(got) != 2 || got[0] != "0/9.dat" || got[1] != "y/9.dat" {
		t.Fatalf("paths %v", got)
	}
	got = photoDownloadPaths("m/9.dat", "m", 9)
	if len(got) != 1 || got[0] != "m/9.dat" {
		t.Fatalf("same key %v", got)
	}
}

func TestUploadPhotoSizesAreLocalBeforeGetFile(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 120, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 120; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 40, A: 255})
		}
	}
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{name: "a.bmp", body: bmp(120, 80)},
		{name: "a.jpg", body: jpg.Bytes()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertUploadedSizesAreLocal(t, tc.name, tc.body)
		})
	}
}

func assertUploadedSizesAreLocal(t *testing.T, name string, raw []byte) {
	t.Helper()
	t.Setenv("TELLOMI_DFS_CHUNK_CACHE_DIR", t.TempDir())
	var hang atomic.Bool
	var gets atomic.Int32
	fake := &completionFake{}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hang.Load() && r.Method == http.MethodGet && bytes.Contains([]byte(r.URL.Path), []byte("/photos/")) {
			gets.Add(1)
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
			}
			return
		}
		fake.ServeHTTP(w, r)
	}))
	t.Cleanup(origin.Close)
	proxy := connectProxyServer(t)
	t.Cleanup(proxy.Close)

	cfg := &minio_util.MinioConfig{
		Endpoint:            origin.URL,
		AccessKeyID:         "minio",
		SecretAccessKey:     "miniostorage",
		Region:              "auto",
		ForcePathStyle:      true,
		BucketProbeAttempts: 1,
		Bucket: minio_util.BucketConfig{
			Attachments: "tellomi-attachments",
			PublicMedia: "tellomi-public-media",
			Backups:     "tellomi-backups",
		},
	}
	cfg.SetHotTierForTest(t.TempDir(), proxy.URL)
	client, err := minio_util.NewMinioClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rds := redistest.CreateRedis(t)
	store := kv.NewStore(kv.KvConf{{
		RedisConf: redis.RedisConf{Host: rds.Addr, Type: redis.NodeType},
		Weight:    100,
	}})
	d := dao.NewDaoWithStore(store)
	d.MinioUtil = client
	c := New(context.Background(), &svc.ServiceContext{Dao: d})
	const photoID int64 = 42
	c.idHook = func() int64 { return photoID }

	writePart(t, c, 7, 0, raw, 0)
	photo, err := c.DfsUploadPhotoFileV2(&dfs.TLDfsUploadPhotoFileV2{
		Creator: 7,
		File: mtproto.MakeTLInputFile(&mtproto.InputFile{
			Id_INT64: 7, Parts: 1, Name: name,
		}).To_InputFile(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(photo.GetSizes()) < 2 {
		t.Fatalf("sizes %#v", photo.GetSizes())
	}
	fake.mu.Lock()
	stored := len(fake.objects["tellomi-attachments"])
	fake.mu.Unlock()
	if stored != 0 {
		t.Fatalf("upload wrote %d objects to R2 while the hot tier is on", stored)
	}

	hang.Store(true)
	for _, sz := range photo.GetSizes() {
		key := fmt.Sprintf("%s/%d.dat", sz.GetType(), photo.GetId())
		want, ok := client.ReadHot(minio_util.ClassPhotos, key, 0, 1<<20)
		if !ok || len(want) == 0 {
			t.Fatalf("size %s missing from hot tier (%d bytes, ok %v)", sz.GetType(), len(want), ok)
		}
		start := time.Now()
		got, err := c.DfsDownloadFile(&dfs.TLDfsDownloadFile{
			Location: mtproto.MakeTLInputPhotoFileLocation(&mtproto.InputFileLocation{
				Id:         photo.GetId(),
				AccessHash: photo.GetAccessHash(),
				ThumbSize:  sz.GetType(),
			}).To_InputFileLocation(),
			Limit: 1 << 20,
		})
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("size %s: %v", sz.GetType(), err)
		}
		if !bytes.Equal(got.GetBytes(), want) {
			t.Fatalf("size %s: got %d bytes, hot tier has %d", sz.GetType(), len(got.GetBytes()), len(want))
		}
		if elapsed >= 500*time.Millisecond {
			t.Fatalf("size %s getFile took %s with R2 hanging", sz.GetType(), elapsed)
		}
	}
	if gets.Load() != 0 {
		t.Fatalf("getFile reached R2 %d times", gets.Load())
	}
}

func connectProxyServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "only CONNECT", http.StatusMethodNotAllowed)
			return
		}
		dest, err := net.DialTimeout("tcp", r.Host, time.Second)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer dest.Close()
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		if _, err = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		done := make(chan struct{}, 2)
		go func() {
			_, _ = io.Copy(dest, conn)
			done <- struct{}{}
		}()
		go func() {
			_, _ = io.Copy(conn, dest)
			done <- struct{}{}
		}()
		<-done
	}))
}

func bmp(w, h int) []byte {
	row := (w*3 + 3) &^ 3
	pixels := row * h
	buf := make([]byte, 54+pixels)
	buf[0], buf[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(buf[2:], uint32(len(buf)))
	binary.LittleEndian.PutUint32(buf[10:], 54)
	binary.LittleEndian.PutUint32(buf[14:], 40)
	binary.LittleEndian.PutUint32(buf[18:], uint32(w))
	binary.LittleEndian.PutUint32(buf[22:], uint32(h))
	binary.LittleEndian.PutUint16(buf[26:], 1)
	binary.LittleEndian.PutUint16(buf[28:], 24)
	for i := 54; i < len(buf); i++ {
		buf[i] = 0x7f
	}
	return buf
}
