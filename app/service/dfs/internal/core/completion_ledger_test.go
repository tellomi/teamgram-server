package core

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	_ "image/gif"

	"github.com/teamgram/marmota/pkg/stores/kv"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/uploadledger"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/redis/redistest"
)

const completionDocumentID int64 = 9001

// onePixelGIF is a 1x1 GIF the real uploadGifMedia path can decode.
var onePixelGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
	0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00,
	0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
}

func TestIndependentDocumentCompletionReleasesLoggedInUser(t *testing.T) {
	led, c := newCompletionCore(t, completionFake{})
	const fileID int64 = 55
	body := []byte("not-an-image-document-body")
	writeCompletionPart(t, c, fileID, body)
	if _, err := led.Reserve(7, fileID, 30); err != nil {
		t.Fatal(err)
	}
	if _, err := led.Reserve(99, fileID, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := led.Reserve(7, completionDocumentID, 7); err != nil {
		t.Fatal(err)
	}
	doc, err := c.DfsUploadDocumentFileV2(&dfs.TLDfsUploadDocumentFileV2{
		Creator: 99,
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{
				Id_INT64: fileID,
				Parts:    1,
				Name:     "note.bin",
			}).To_InputFile(),
			MimeType: "application/octet-stream",
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{
					FileName: "note.bin",
				}).To_DocumentAttribute(),
			},
		}).To_InputMedia(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.GetId() != completionDocumentID {
		t.Fatalf("document id %d", doc.GetId())
	}
	if got := led.Book(7).Hold; got != 7 {
		t.Fatalf("logged-in hold %d, file %d was not released or the new document id was", got, fileID)
	}
	if got := led.Book(99).Hold; got != 20 {
		t.Fatalf("auth-key hold %d", got)
	}
	if got := led.Book(7).Entity; got != int64(len(body)) {
		t.Fatalf("entity %d", got)
	}
	if got := led.Book(99).Entity; got != 0 {
		t.Fatalf("auth key entity %d", got)
	}
}

func TestIndependentDocumentPutFailureDoesNotRecord(t *testing.T) {
	led, c := newCompletionCore(t, completionFake{failDocuments: true})
	const fileID int64 = 56
	writeCompletionPart(t, c, fileID, []byte("document-that-cannot-be-stored"))
	if _, err := led.Reserve(7, fileID, 30); err != nil {
		t.Fatal(err)
	}
	_, err := c.DfsUploadDocumentFileV2(&dfs.TLDfsUploadDocumentFileV2{
		Creator: 99,
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{
				Id_INT64: fileID,
				Parts:    1,
				Name:     "note.bin",
			}).To_InputFile(),
			MimeType: "application/octet-stream",
		}).To_InputMedia(),
	})
	if err == nil {
		t.Fatal("stored a document the object store rejected")
	}
	if got := led.Book(7).Entity; got != 0 {
		t.Fatalf("failed put recorded %d entity bytes", got)
	}
	if got := led.Book(7).Hold; got != 30 {
		t.Fatalf("failed put released hold %d", got)
	}
}

func TestIndependentGifCompletionReleasesUploadFile(t *testing.T) {
	led, c := newCompletionCore(t, completionFake{})
	const fileID int64 = 57
	writeCompletionPart(t, c, fileID, onePixelGIF)
	if _, err := led.Reserve(7, fileID, 30); err != nil {
		t.Fatal(err)
	}
	if _, err := led.Reserve(99, fileID, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := led.Reserve(7, completionDocumentID, 7); err != nil {
		t.Fatal(err)
	}
	doc, err := c.DfsUploadGifDocumentMedia(&dfs.TLDfsUploadGifDocumentMedia{
		Creator: 99,
		Media:   gifMedia(fileID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.GetId() != completionDocumentID {
		t.Fatalf("document id %d", doc.GetId())
	}
	if got := led.Book(7).Hold; got != 7 {
		t.Fatalf("logged-in hold %d, upload file was not the id that was released", got)
	}
	if got := led.Book(99).Hold; got != 20 {
		t.Fatalf("auth-key hold %d", got)
	}
	if got := led.Book(7).Entity; got != int64(len(onePixelGIF)) {
		t.Fatalf("entity %d", got)
	}
}

func TestIndependentGifThumbFailureRefundsAndKeepsHold(t *testing.T) {
	led, c := newCompletionCore(t, completionFake{failPhotos: true})
	const fileID int64 = 58
	writeCompletionPart(t, c, fileID, onePixelGIF)
	if _, err := led.Reserve(7, fileID, 30); err != nil {
		t.Fatal(err)
	}
	if _, err := led.Reserve(7, completionDocumentID, 7); err != nil {
		t.Fatal(err)
	}
	_, err := c.DfsUploadGifDocumentMedia(&dfs.TLDfsUploadGifDocumentMedia{
		Creator: 99,
		Media:   gifMedia(fileID),
	})
	if err == nil {
		t.Fatal("gif completed after the thumb put failed")
	}
	if got := led.Book(7).Entity; got != 0 {
		t.Fatalf("thumb failure left %d entity bytes", got)
	}
	if got := led.Book(7).Hold; got != 37 {
		t.Fatalf("hold %d", got)
	}
}

func TestIndependentStoreFailureDoesNotRecord(t *testing.T) {
	led := uploadledger.New(uploadledger.Limits{Entity: 100, Network: 100, Hold: 100, Window: time.Hour})
	uploadledger.Use(led)
	t.Cleanup(func() { uploadledger.Use(nil) })
	c := &DfsCore{}
	ctx := uploadledger.WithUser(context.Background(), 7)
	_, err := c.storeDocumentBytes(ctx, 99, []byte("0123456789abcdef"), false)
	if err == nil {
		t.Fatal("missing writer stored the document")
	}
	if got := led.Book(7).Entity; got != 0 {
		t.Fatalf("failed store recorded %d", got)
	}
}

func writeCompletionPart(t *testing.T, c *DfsCore, id int64, body []byte) {
	t.Helper()
	ok, err := c.DfsWriteFilePartData(&dfs.TLDfsWriteFilePartData{
		Creator:  99,
		FileId:   id,
		FilePart: 0,
		Bytes:    body,
	})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("write part: %v", err)
	}
}

func gifMedia(fileID int64) *mtproto.InputMedia {
	return mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
		File: mtproto.MakeTLInputFile(&mtproto.InputFile{
			Id_INT64: fileID,
			Parts:    1,
			Name:     "a.gif",
		}).To_InputFile(),
		MimeType: "image/gif",
		Attributes: []*mtproto.DocumentAttribute{
			mtproto.MakeTLDocumentAttributeAnimated(&mtproto.DocumentAttribute{}).To_DocumentAttribute(),
		},
	}).To_InputMedia()
}

func newCompletionCore(t *testing.T, fake completionFake) (*uploadledger.Ledger, *DfsCore) {
	t.Helper()
	srv := httptest.NewServer(&fake)
	t.Cleanup(srv.Close)
	client, err := minio_util.NewMinioClient(&minio_util.MinioConfig{
		Endpoint:            srv.URL,
		AccessKeyID:         "minio",
		SecretAccessKey:     "miniostorage",
		UseSSL:              false,
		Region:              "auto",
		ForcePathStyle:      true,
		BucketProbeAttempts: 1,
		Bucket: minio_util.BucketConfig{
			Attachments: "tellomi-attachments",
			PublicMedia: "tellomi-public-media",
			Backups:     "tellomi-backups",
		},
	})
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
	c.MD = &metadata.RpcMetadata{UserId: 7, PermAuthKeyId: 99}
	c.idHook = func() int64 { return completionDocumentID }
	led := uploadledger.New(uploadledger.Limits{
		Network: 1 << 20,
		Entity:  1 << 20,
		Hold:    1 << 20,
		Refs:    1000,
		Window:  time.Hour,
	})
	uploadledger.Use(led)
	t.Cleanup(func() { uploadledger.Use(nil) })
	return led, c
}

type completionFake struct {
	mu            sync.Mutex
	objects       map[string]map[string][]byte
	uploads       map[string]*completionUpload
	failDocuments bool
	failPhotos    bool
}

type completionUpload struct {
	bucket string
	key    string
	parts  map[int][]byte
}

func (s *completionFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.objects == nil {
		s.objects = map[string]map[string][]byte{}
		for _, name := range []string{"tellomi-attachments", "tellomi-public-media", "tellomi-backups"} {
			s.objects[name] = map[string][]byte{}
		}
		s.uploads = map[string]*completionUpload{}
	}
	s.mu.Unlock()
	bucket, key := completionSplit(r.URL.Path)
	if _, ok := s.objects[bucket]; bucket == "" || !ok {
		http.Error(w, "no such bucket", http.StatusNotFound)
		return
	}
	if s.rejected(key) {
		http.Error(w, "store rejected", http.StatusInternalServerError)
		return
	}
	q := r.URL.Query()
	switch {
	case r.Method == http.MethodHead && key == "":
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && key == "" && q.Has("location"):
		fmt.Fprint(w, "<LocationConstraint>auto</LocationConstraint>")
	case r.Method == http.MethodPost && q.Has("uploads"):
		s.initiate(w, bucket, key)
	case r.Method == http.MethodPut && q.Get("uploadId") != "" && q.Get("partNumber") != "":
		s.putPart(w, r, q.Get("uploadId"), q.Get("partNumber"))
	case r.Method == http.MethodPost && q.Get("uploadId") != "":
		s.complete(w, bucket, key, q.Get("uploadId"))
	case r.Method == http.MethodPut && key != "":
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.objects[bucket][key] = body
		s.mu.Unlock()
		w.Header().Set("ETag", `"put"`)
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.RequestURI(), http.StatusNotImplemented)
	}
}

func (s *completionFake) rejected(key string) bool {
	if s.failDocuments && strings.Contains(key, "documents/") {
		return true
	}
	return s.failPhotos && strings.Contains(key, "photos/")
}

func (s *completionFake) initiate(w http.ResponseWriter, bucket, key string) {
	s.mu.Lock()
	id := fmt.Sprintf("up-%d", len(s.uploads)+1)
	s.uploads[id] = &completionUpload{bucket: bucket, key: key, parts: map[int][]byte{}}
	s.mu.Unlock()
	writeCompletionXML(w, struct {
		XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
		Bucket   string   `xml:"Bucket"`
		Key      string   `xml:"Key"`
		UploadID string   `xml:"UploadId"`
	}{Bucket: bucket, Key: key, UploadID: id})
}

func (s *completionFake) putPart(w http.ResponseWriter, r *http.Request, id, part string) {
	body, _ := io.ReadAll(r.Body)
	n := 0
	fmt.Sscanf(part, "%d", &n)
	s.mu.Lock()
	up := s.uploads[id]
	if up != nil && s.rejected(up.key) {
		s.mu.Unlock()
		http.Error(w, "store rejected", http.StatusInternalServerError)
		return
	}
	if up != nil {
		up.parts[n] = body
	}
	s.mu.Unlock()
	if up == nil {
		http.Error(w, "no such upload", http.StatusNotFound)
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`"part-%d"`, n))
	w.WriteHeader(http.StatusOK)
}

func (s *completionFake) complete(w http.ResponseWriter, bucket, key, id string) {
	s.mu.Lock()
	up := s.uploads[id]
	if up == nil {
		s.mu.Unlock()
		http.Error(w, "no such upload", http.StatusNotFound)
		return
	}
	var buf bytes.Buffer
	for i := 1; i <= len(up.parts); i++ {
		buf.Write(up.parts[i])
	}
	s.objects[bucket][key] = buf.Bytes()
	delete(s.uploads, id)
	s.mu.Unlock()
	writeCompletionXML(w, struct {
		XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
		Location string   `xml:"Location"`
		Bucket   string   `xml:"Bucket"`
		Key      string   `xml:"Key"`
		ETag     string   `xml:"ETag"`
	}{Location: "https://example/" + key, Bucket: bucket, Key: key, ETag: `"done"`})
}

func completionSplit(p string) (string, string) {
	p = strings.TrimPrefix(p, "/")
	bucket, key, _ := strings.Cut(p, "/")
	return bucket, key
}

func writeCompletionXML(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(v)
}
