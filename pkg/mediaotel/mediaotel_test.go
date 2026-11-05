package mediaotel

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/configload"
	gztrace "github.com/zeromicro/go-zero/core/trace"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestEmptyEndpointDoesNotInstallExporterOrFailUpload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "svc.yaml")
	body := []byte("Name: dfs\nTelemetry:\n  Name: service.dfs\n  Endpoint: ${TELLOMI_OTEL_ENDPOINT}\n  Sampler: 1.0\n  Batcher: otlpgrpc\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TELLOMI_OTEL_ENDPOINT", "   ")
	var cfg service.ServiceConf
	configload.MustLoad(path, &cfg)
	if cfg.Telemetry.Endpoint != "" || !cfg.Telemetry.Disabled {
		t.Fatalf("endpoint %q disabled %v, want empty and disabled", cfg.Telemetry.Endpoint, cfg.Telemetry.Disabled)
	}

	before := otel.GetTracerProvider()
	gztrace.StartAgent(cfg.Telemetry)
	if otel.GetTracerProvider() != before {
		t.Fatal("disabled telemetry installed a tracer provider")
	}

	logs := captureLogs(t)
	for i := 0; i < 20; i++ {
		ctx, finish := BeginUpload(context.Background(), ClassPhotos, i%2 == 0, int64(i+1))
		if ctx == nil || finish == nil {
			t.Fatal("nil upload trace")
		}
		finish(i%2 == 1, int64(i))
		_, done := BeginDocumentLookup(context.Background())
		done(LookupEmpty, int64(i))
	}
	if strings.Contains(logs.String(), "[otel] error") {
		t.Fatalf("empty endpoint logged otel errors:\n%s", logs.String())
	}
	if otel.GetTracerProvider() != before {
		t.Fatal("upload tracing installed a tracer provider")
	}
}

func TestUploadSpanHitMissAndWrite(t *testing.T) {
	rec := withRecorder(t)

	_, finishHit := BeginUpload(context.Background(), ClassDocuments, true, 32)
	finishHit(false, 0)
	finishHit(true, 99) // second call is ignored

	_, finishMiss := BeginUpload(context.Background(), ClassPhotos, false, 128)
	finishMiss(true, 128)

	spans := rec.Ended()
	if len(spans) != 2 {
		t.Fatalf("spans %d", len(spans))
	}
	hit := attrs(t, spans[0])
	if hit[AttrDedup] != DedupHit || hit[AttrReused] != "true" || hit[AttrObjectWritten] != "false" || hit[AttrBytesWritten] != "0" || hit[AttrBytes] != "32" || hit[AttrStorageClass] != ClassDocuments {
		t.Fatalf("hit attrs %#v", hit)
	}
	if !hasEvent(spans[0], EventClaim) || hasEvent(spans[0], EventWrite) {
		t.Fatalf("hit events %+v", spans[0].Events())
	}
	miss := attrs(t, spans[1])
	if miss[AttrDedup] != DedupMiss || miss[AttrReused] != "false" || miss[AttrObjectWritten] != "true" || miss[AttrBytesWritten] != "128" || miss[AttrBytes] != "128" || miss[AttrStorageClass] != ClassPhotos {
		t.Fatalf("miss attrs %#v", miss)
	}
	if !hasEvent(spans[1], EventClaim) || !hasEvent(spans[1], EventWrite) {
		t.Fatalf("miss events %+v", spans[1].Events())
	}
	for _, sp := range spans {
		if _, ok := attrs(t, sp)[AttrDurationMS]; !ok {
			t.Fatal("missing duration_ms")
		}
		assertSafeSpan(t, sp)
	}
}

func TestUploadDropsUnsafeClassAndNegativeCounts(t *testing.T) {
	rec := withRecorder(t)
	const sha = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	_, finish := BeginUpload(context.Background(), sha, false, -5)
	finish(true, -8)
	sp := onlySpan(t, rec)
	got := attrs(t, sp)
	if got[AttrStorageClass] != ClassUnknown || got[AttrBytes] != "0" || got[AttrBytesWritten] != "0" || got[AttrObjectWritten] != "true" {
		t.Fatalf("attrs %#v", got)
	}
	if strings.Contains(sp.Name(), sha) {
		t.Fatal("span name contains the hash")
	}
	assertSafeSpan(t, sp)
}

func TestDocumentLookupHitEmptyAndError(t *testing.T) {
	rec := withRecorder(t)
	for _, result := range []string{LookupHit, LookupEmpty, LookupError} {
		_, done := BeginDocumentLookup(context.Background())
		done(result, 64)
	}
	spans := rec.Ended()
	if len(spans) != 3 {
		t.Fatalf("spans %d", len(spans))
	}
	for i, want := range []string{LookupHit, LookupEmpty, LookupError} {
		if spans[i].Name() != SpanDocumentByHash {
			t.Fatalf("name %s", spans[i].Name())
		}
		got := attrs(t, spans[i])
		if got[AttrLookup] != want || got[AttrBytes] != "64" {
			t.Fatalf("%s attrs %#v", want, got)
		}
		if _, ok := got[AttrDurationMS]; !ok {
			t.Fatal("missing duration")
		}
		assertSafeSpan(t, spans[i])
	}
	if attrs(t, spans[0])[AttrLookup] == attrs(t, spans[1])[AttrLookup] {
		t.Fatal("hit and documentEmpty are not distinguishable")
	}
}

func TestDocumentLookupRejectsHashAsResult(t *testing.T) {
	rec := withRecorder(t)
	const sha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	_, done := BeginDocumentLookup(context.Background())
	done(sha, 4)
	sp := onlySpan(t, rec)
	if attrs(t, sp)[AttrLookup] != LookupError {
		t.Fatalf("lookup %q", attrs(t, sp)[AttrLookup])
	}
	assertSafeSpan(t, sp)
	if strings.Contains(sp.Name(), sha) {
		t.Fatal("span name contains the hash")
	}
}

func TestNilContextDoesNotFail(t *testing.T) {
	rec := withRecorder(t)
	_, finish := BeginUpload(nil, ClassDocuments, true, 1)
	finish(false, 0)
	_, done := BeginDocumentLookup(nil)
	done(LookupEmpty, 1)
	if len(rec.Ended()) != 2 {
		t.Fatalf("spans %d", len(rec.Ended()))
	}
}

func withRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	return installRecorder(t)
}

func installRecorder(t *testing.T) *tracetest.SpanRecorder {
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

func onlySpan(t *testing.T, rec *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()
	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans %d", len(spans))
	}
	return spans[0]
}

func attrs(t *testing.T, sp sdktrace.ReadOnlySpan) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, kv := range sp.Attributes() {
		out[string(kv.Key)] = kv.Value.Emit()
	}
	if _, ok := out[AttrDurationMS]; ok {
		d := out[AttrDurationMS]
		if d == "" || strings.HasPrefix(d, "-") {
			t.Fatalf("duration %q", d)
		}
	}
	return out
}

func hasEvent(sp sdktrace.ReadOnlySpan, name string) bool {
	for _, ev := range sp.Events() {
		if ev.Name == name {
			return true
		}
	}
	return false
}

func assertSafeSpan(t *testing.T, sp sdktrace.ReadOnlySpan) {
	t.Helper()
	check := func(kvs []attribute.KeyValue) {
		t.Helper()
		for _, kv := range kvs {
			key := strings.ToLower(string(kv.Key))
			for _, bad := range []string{"access_hash", "accesshash", "sha256", "sha", "plaintext", "content", "mime", "filename", "file_name", "body", "path"} {
				if key == bad || strings.Contains(key, bad) {
					t.Fatalf("forbidden attribute %s", kv.Key)
				}
			}
			switch kv.Value.Type() {
			case attribute.BOOL, attribute.INT64:
			case attribute.STRING:
				v := kv.Value.AsString()
				switch v {
				case DedupHit, DedupMiss, LookupEmpty, LookupError, ClassDocuments, ClassPhotos, ClassUnknown:
				default:
					t.Fatalf("unexpected string attribute %s=%q", kv.Key, v)
				}
			default:
				t.Fatalf("unexpected attribute type %s %v", kv.Key, kv.Value.Type())
			}
		}
	}
	check(sp.Attributes())
	for _, ev := range sp.Events() {
		if ev.Name != EventClaim && ev.Name != EventWrite {
			t.Fatalf("event %s", ev.Name)
		}
		check(ev.Attributes)
	}
	if sp.Name() != SpanUpload && sp.Name() != SpanDocumentByHash {
		t.Fatalf("span %s", sp.Name())
	}
}

type logBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *logBuf) record(v any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.b.WriteString(strings.TrimSpace(strings.ReplaceAll(toString(v), "\n", " ")))
	w.b.WriteByte('\n')
}

func (w *logBuf) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

func (w *logBuf) Alert(v any)                     { w.record(v) }
func (w *logBuf) Close() error                    { return nil }
func (w *logBuf) Debug(v any, _ ...logx.LogField) {}
func (w *logBuf) Error(v any, _ ...logx.LogField) { w.record(v) }
func (w *logBuf) Info(v any, _ ...logx.LogField)  {}
func (w *logBuf) Severe(v any)                    { w.record(v) }
func (w *logBuf) Slow(v any, _ ...logx.LogField)  {}
func (w *logBuf) Stack(v any)                     { w.record(v) }
func (w *logBuf) Stat(v any, _ ...logx.LogField)  {}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case error:
		return x.Error()
	default:
		return ""
	}
}

func captureLogs(t *testing.T) *logBuf {
	t.Helper()
	prevLevel := uint32(logx.DebugLevel)
	logx.SetLevel(logx.DebugLevel)
	buf := &logBuf{}
	old := logx.Reset()
	logx.SetWriter(buf)
	t.Cleanup(func() {
		logx.Reset()
		if old != nil {
			logx.SetWriter(old)
		}
		logx.SetLevel(prevLevel)
	})
	return buf
}
