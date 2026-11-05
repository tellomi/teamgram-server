package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/teamgram-server/app/service/mediablob"
	"github.com/teamgram/teamgram-server/pkg/mediaotel"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

const (
	traceSHA       = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tracePlaintext = "user-plaintext-canary"
)

func TestStoreClaimedDocumentHitSkipsWrite(t *testing.T) {
	rec := uploadSpanRecorder(t)
	called := false
	claim := mediablob.Claim{
		Reused: true,
		Size:   32,
		Class:  mediablob.ClassDocuments,
		SHA256: traceSHA,
		Path:   tracePlaintext,
	}
	got, err := storeClaimedDocument(context.Background(), nil, claim, false, func(context.Context) (int64, error) {
		called = true
		return 32, nil
	})
	if err != nil || called || !got.Reused {
		t.Fatalf("hit write=%v err=%v reused=%v", called, err, got.Reused)
	}
	sp := oneUploadSpan(t, rec)
	text := uploadSpanText(sp)
	if !strings.Contains(text, "dedup="+mediaotel.DedupHit) || !strings.Contains(text, "reused=true") || !strings.Contains(text, "object_written=false") || !strings.Contains(text, "bytes=32") || !strings.Contains(text, "bytes_written=0") || !strings.Contains(text, "storage_class="+mediaotel.ClassDocuments) || !strings.Contains(text, "claim") || strings.Contains(text, "object.write") {
		t.Fatalf("hit span %s", text)
	}
	assertUploadSpanSafe(t, text)
}

func TestStoreClaimedDocumentMissWrites(t *testing.T) {
	rec := uploadSpanRecorder(t)
	claim := mediablob.Claim{
		Reused: false,
		Size:   40,
		SHA256: traceSHA,
		Path:   tracePlaintext,
	}
	got, err := storeClaimedDocument(context.Background(), nil, claim, false, func(context.Context) (int64, error) {
		return 40, nil
	})
	if err != nil || got.Reused {
		t.Fatalf("miss err=%v reused=%v", err, got.Reused)
	}
	text := uploadSpanText(oneUploadSpan(t, rec))
	if !strings.Contains(text, "dedup="+mediaotel.DedupMiss) || !strings.Contains(text, "reused=false") || !strings.Contains(text, "object_written=true") || !strings.Contains(text, "bytes=40") || !strings.Contains(text, "bytes_written=40") || !strings.Contains(text, "object.write") || !strings.Contains(text, "duration_ms=") {
		t.Fatalf("miss span %s", text)
	}
	assertUploadSpanSafe(t, text)
}

func TestStoreClaimedDocumentWriteErrorIsNotATraceFailure(t *testing.T) {
	rec := uploadSpanRecorder(t)
	_, err := storeClaimedDocument(context.Background(), nil, mediablob.Claim{Size: 4, SHA256: traceSHA}, false, func(context.Context) (int64, error) {
		return 0, errors.New("disk")
	})
	if err == nil || !strings.Contains(err.Error(), "disk") {
		t.Fatalf("err %v", err)
	}
	text := uploadSpanText(oneUploadSpan(t, rec))
	if !strings.Contains(text, "dedup="+mediaotel.DedupMiss) || !strings.Contains(text, "object_written=false") {
		t.Fatalf("span %s", text)
	}
	assertUploadSpanSafe(t, text)
}

func TestStoreClaimedDocumentAsyncMissStillUploads(t *testing.T) {
	rec := uploadSpanRecorder(t)
	done := make(chan struct{})
	_, err := storeClaimedDocument(context.Background(), nil, mediablob.Claim{Size: 9, SHA256: traceSHA, Path: tracePlaintext}, true, func(context.Context) (int64, error) {
		defer close(done)
		return 9, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("async write did not run")
	}
	var text string
	deadline := time.Now().Add(2 * time.Second)
	for {
		spans := rec.Ended()
		if len(spans) == 1 {
			text = uploadSpanText(spans[0])
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("spans %d", len(rec.Ended()))
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(text, "object_written=true") || !strings.Contains(text, "bytes_written=9") || !strings.Contains(text, "dedup="+mediaotel.DedupMiss) {
		t.Fatalf("async span %s", text)
	}
	assertUploadSpanSafe(t, text)
}

func TestStoreClaimedDocumentWithoutExporterDoesNotFail(t *testing.T) {
	// The default provider is a no-op unless a service installed an
	// exporter. These calls must succeed and must not require a collector.
	hit, err := storeClaimedDocument(context.Background(), nil, mediablob.Claim{Reused: true, Size: 1}, false, func(context.Context) (int64, error) {
		t.Fatal("hit wrote the object")
		return 0, nil
	})
	if err != nil || !hit.Reused {
		t.Fatalf("hit %+v %v", hit, err)
	}
	miss, err := storeClaimedDocument(context.Background(), nil, mediablob.Claim{Size: 2}, false, func(context.Context) (int64, error) {
		return 2, nil
	})
	if err != nil || miss.Reused {
		t.Fatalf("miss %+v %v", miss, err)
	}
}

func uploadSpanRecorder(t *testing.T) *tracetest.SpanRecorder {
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

func oneUploadSpan(t *testing.T, rec *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()
	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans %d", len(spans))
	}
	if spans[0].Name() != mediaotel.SpanUpload {
		t.Fatalf("name %s", spans[0].Name())
	}
	return spans[0]
}

func uploadSpanText(sp sdktrace.ReadOnlySpan) string {
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

func assertUploadSpanSafe(t *testing.T, text string) {
	t.Helper()
	for _, bad := range []string{traceSHA, tracePlaintext, "access_hash", "sha256"} {
		if strings.Contains(text, bad) {
			t.Fatalf("span contains %q: %s", bad, text)
		}
	}
}
