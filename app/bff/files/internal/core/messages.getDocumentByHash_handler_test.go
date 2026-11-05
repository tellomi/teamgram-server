package core

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/teamgram/teamgram-server/pkg/mediaotel"

	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestDocumentByHashSize(t *testing.T) {
	old := &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_338e2464,
		Size2_INT32: 42,
		Size2_INT64: 99,
	}
	if got := documentByHashSize(old); got != 42 {
		t.Fatalf("old constructor size %d", got)
	}
	cur := &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_b1f2061f,
		Size2_INT64: 1000,
		Size2_INT32: 1,
	}
	if got := documentByHashSize(cur); got != 1000 {
		t.Fatalf("current constructor size %d", got)
	}
	fallback := &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_b1f2061f,
		Size2_INT32: 7,
	}
	if got := documentByHashSize(fallback); got != 7 {
		t.Fatalf("fallback size %d", got)
	}
}

func TestGetDocumentByHashTraceHitAndEmpty(t *testing.T) {
	rec := spanRecorder(t)
	const (
		accessHash = int64(8675309)
		plaintext  = "user-plaintext-canary"
	)
	sha := bytesRepeat(0x11, 32)
	shaHex := hex.EncodeToString(sha)

	emptyDoc, err := lookupDocumentByHash(context.Background(), logx.WithContext(context.Background()), nil, nil, &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_b1f2061f,
		Sha256:      sha,
		MimeType:    plaintext,
		Size2_INT64: 64,
	})
	if err != nil || emptyDoc.GetPredicateName() != mtproto.Predicate_documentEmpty {
		t.Fatalf("nil lookup %+v %v", emptyDoc, err)
	}

	finder := &fakeHashFinder{}
	oldSize, err := lookupDocumentByHash(context.Background(), logx.WithContext(context.Background()), finder, nil, &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_338e2464,
		Sha256:      sha,
		MimeType:    plaintext,
		Size2_INT32: 42,
		Size2_INT64: 99,
	})
	if err != nil || oldSize.GetPredicateName() != mtproto.Predicate_documentEmpty {
		t.Fatalf("old constructor %+v %v", oldSize, err)
	}
	if finder.sha != shaHex || finder.size != 42 || finder.mime != plaintext {
		t.Fatalf("lookup args sha %s size %d mime %s", finder.sha, finder.size, finder.mime)
	}

	finder.id = 7
	docs := &fakeDocs{doc: mtproto.MakeTLDocument(&mtproto.Document{
		Id:         7,
		AccessHash: accessHash,
		MimeType:   plaintext,
		DcId:       1,
	}).To_Document()}
	hit, err := lookupDocumentByHash(context.Background(), logx.WithContext(context.Background()), finder, docs, &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_b1f2061f,
		Sha256:      sha,
		MimeType:    plaintext,
		Size2_INT64: 64,
	})
	if err != nil || hit.GetId() != 7 || hit.GetAccessHash() != accessHash {
		t.Fatalf("hit %+v %v", hit, err)
	}

	docs.doc = mtproto.MakeTLDocumentEmpty(&mtproto.Document{Id: 7, AccessHash: accessHash}).To_Document()
	miss, err := lookupDocumentByHash(context.Background(), logx.WithContext(context.Background()), finder, docs, &mtproto.TLMessagesGetDocumentByHash{
		Sha256:      sha,
		MimeType:    plaintext,
		Size2_INT64: 64,
	})
	if err != nil || miss.GetPredicateName() != mtproto.Predicate_documentEmpty {
		t.Fatalf("stored empty %+v %v", miss, err)
	}

	finder.err = errors.New("db down")
	if _, err = lookupDocumentByHash(context.Background(), logx.WithContext(context.Background()), finder, docs, &mtproto.TLMessagesGetDocumentByHash{
		Sha256:      sha,
		MimeType:    plaintext,
		Size2_INT64: 64,
	}); err == nil {
		t.Fatal("expected lookup error")
	}

	spans := rec.Ended()
	if len(spans) != 5 {
		t.Fatalf("spans %d", len(spans))
	}
	want := []string{
		mediaotel.LookupEmpty, // nil finder
		mediaotel.LookupEmpty, // id 0
		mediaotel.LookupHit,
		mediaotel.LookupEmpty, // documentEmpty row
		mediaotel.LookupError,
	}
	for i, result := range want {
		if spans[i].Name() != mediaotel.SpanDocumentByHash {
			t.Fatalf("span %s", spans[i].Name())
		}
		got := spanText(spans[i])
		if !strings.Contains(got, "lookup="+result) {
			t.Fatalf("span %d text %s, want lookup=%s", i, got, result)
		}
		if strings.Contains(got, shaHex) || strings.Contains(got, plaintext) || strings.Contains(got, "8675309") || strings.Contains(got, "access_hash") {
			t.Fatalf("span leaked forbidden data: %s", got)
		}
	}
	if !strings.Contains(spanText(spans[1]), "bytes=42") {
		t.Fatalf("old constructor size not recorded: %s", spanText(spans[1]))
	}
	if !strings.Contains(spanText(spans[2]), "bytes=64") || !strings.Contains(spanText(spans[2]), "duration_ms=") {
		t.Fatalf("hit span %s", spanText(spans[2]))
	}
	if strings.Contains(spanText(spans[2]), "lookup="+mediaotel.LookupEmpty) {
		t.Fatal("hit span looks like documentEmpty")
	}
}

func TestMessagesGetDocumentByHashNilStoreIsEmpty(t *testing.T) {
	rec := spanRecorder(t)
	c := New(context.Background(), &svc.ServiceContext{})
	doc, err := c.MessagesGetDocumentByHash(&mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_b1f2061f,
		Sha256:      bytesRepeat(0x22, 32),
		MimeType:    "image/gif",
		Size2_INT64: 3,
	})
	if err != nil || doc.GetPredicateName() != mtproto.Predicate_documentEmpty {
		t.Fatalf("doc %+v %v", doc, err)
	}
	spans := rec.Ended()
	if len(spans) != 1 || !strings.Contains(spanText(spans[0]), "lookup="+mediaotel.LookupEmpty) {
		t.Fatalf("spans %d", len(spans))
	}
}

type fakeHashFinder struct {
	id   int64
	err  error
	sha  string
	size int64
	mime string
}

func (f *fakeHashFinder) FindDocument(ctx context.Context, shaHex string, size int64, mime string) (int64, error) {
	f.sha = shaHex
	f.size = size
	f.mime = mime
	if f.err != nil {
		return 0, f.err
	}
	return f.id, nil
}

type fakeDocs struct {
	doc *mtproto.Document
}

func (f *fakeDocs) MediaGetDocument(ctx context.Context, in *media.TLMediaGetDocument) (*mtproto.Document, error) {
	return f.doc, nil
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func spanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	prev := otel.GetTracerProvider()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prev)
	})
	return rec
}

func spanText(sp sdktrace.ReadOnlySpan) string {
	var b strings.Builder
	b.WriteString(sp.Name())
	for _, kv := range sp.Attributes() {
		b.WriteByte(' ')
		b.WriteString(string(kv.Key))
		b.WriteByte('=')
		b.WriteString(kv.Value.Emit())
	}
	for _, ev := range sp.Events() {
		b.WriteByte(' ')
		b.WriteString(ev.Name)
		for _, kv := range ev.Attributes {
			b.WriteByte(' ')
			b.WriteString(string(kv.Key))
			b.WriteByte('=')
			b.WriteString(kv.Value.Emit())
		}
	}
	return b.String()
}
