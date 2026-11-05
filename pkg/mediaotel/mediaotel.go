// Package mediaotel records upload dedup and messages.getDocumentByHash
// spans on the process tracer. It does not install an exporter. Export
// stays with go-zero, which starts one only when TELLOMI_OTEL_ENDPOINT
// is non-empty (pkg/configload). An empty endpoint leaves the default
// no-op tracer: these calls do not fail the upload and do not log
// [otel] error.
//
// Attributes are an allow-list. access_hash, object bytes, user
// plaintext, and the content sha256 are not accepted and cannot be set
// through this API.
package mediaotel

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const TracerName = "tellomi.media"

const (
	SpanUpload         = "media.upload"
	SpanDocumentByHash = "messages.getDocumentByHash"
)

const (
	AttrStorageClass  = "storage_class"
	AttrReused        = "reused"
	AttrDedup         = "dedup"
	AttrBytes         = "bytes"
	AttrBytesWritten  = "bytes_written"
	AttrObjectWritten = "object_written"
	AttrDurationMS    = "duration_ms"
	AttrLookup        = "lookup"
)

const (
	DedupHit  = "hit"
	DedupMiss = "miss"

	LookupHit   = "hit"
	LookupEmpty = "documentEmpty"
	LookupError = "error"

	EventClaim = "claim"
	EventWrite = "object.write"

	ClassDocuments = "documents"
	ClassPhotos    = "photos"
	ClassUnknown   = "unknown"
)

// BeginUpload starts one span for a content-addressed upload.
// storageClass is documents or photos. reused is the claim result:
// true means the object already existed (hit). size is the byte count
// of the upload, not the bytes themselves.
//
// The returned function ends the span. Call it once with wrote=true
// only after the object bytes have been stored. Further calls do
// nothing. A nil context is treated as Background so a missing parent
// cannot fail the upload.
func BeginUpload(ctx context.Context, storageClass string, reused bool, size int64) (context.Context, func(wrote bool, written int64)) {
	if ctx == nil {
		ctx = context.Background()
	}
	class := safeClass(storageClass)
	dedup := DedupMiss
	if reused {
		dedup = DedupHit
	}
	size = nonNegative(size)
	start := time.Now()
	ctx, span := otel.Tracer(TracerName).Start(ctx, SpanUpload)
	span.SetAttributes(
		attribute.String(AttrStorageClass, class),
		attribute.Bool(AttrReused, reused),
		attribute.String(AttrDedup, dedup),
		attribute.Int64(AttrBytes, size),
		attribute.Int64(AttrBytesWritten, 0),
		attribute.Bool(AttrObjectWritten, false),
	)
	span.AddEvent(EventClaim, trace.WithAttributes(
		attribute.String(AttrStorageClass, class),
		attribute.Bool(AttrReused, reused),
		attribute.String(AttrDedup, dedup),
		attribute.Int64(AttrBytes, size),
	))
	var once sync.Once
	return ctx, func(wrote bool, written int64) {
		once.Do(func() {
			if wrote {
				written = nonNegative(written)
				span.SetAttributes(
					attribute.Bool(AttrObjectWritten, true),
					attribute.Int64(AttrBytesWritten, written),
				)
				span.AddEvent(EventWrite, trace.WithAttributes(
					attribute.Bool(AttrObjectWritten, true),
					attribute.Int64(AttrBytesWritten, written),
				))
			}
			span.SetAttributes(attribute.Int64(AttrDurationMS, time.Since(start).Milliseconds()))
			span.End()
		})
	}
}

// BeginDocumentLookup starts the messages.getDocumentByHash span.
// The returned function records lookup=hit, documentEmpty, or error,
// plus the requested size as a byte count, then ends the span.
// Any other result string is recorded as error so a hash cannot be
// stored by passing it as the result.
func BeginDocumentLookup(ctx context.Context) (context.Context, func(result string, size int64)) {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	ctx, span := otel.Tracer(TracerName).Start(ctx, SpanDocumentByHash)
	var once sync.Once
	return ctx, func(result string, size int64) {
		once.Do(func() {
			span.SetAttributes(
				attribute.String(AttrLookup, safeLookup(result)),
				attribute.Int64(AttrBytes, nonNegative(size)),
				attribute.Int64(AttrDurationMS, time.Since(start).Milliseconds()),
			)
			span.End()
		})
	}
}

func safeClass(class string) string {
	switch class {
	case ClassDocuments, ClassPhotos:
		return class
	default:
		return ClassUnknown
	}
}

func safeLookup(result string) string {
	switch result {
	case LookupHit, LookupEmpty, LookupError:
		return result
	default:
		return LookupError
	}
}

func nonNegative(n int64) int64 {
	if n < 0 {
		return 0
	}
	return n
}
