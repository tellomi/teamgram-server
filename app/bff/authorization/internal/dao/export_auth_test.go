package dao

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestExportedAuthIsOneShot(t *testing.T) {
	memoryExports.Lock()
	memoryExports.rows = map[string]exportRecord{}
	memoryExports.Unlock()

	raw := []byte("not-a-dc-number")
	now := time.Unix(1_700_000_000, 0)
	ctx := context.Background()
	if err := putExportedAuth(ctx, nil, raw, 42, now); err != nil {
		t.Fatal(err)
	}
	got, err := takeExportedAuth(ctx, nil, raw, now)
	if err != nil || got != 42 {
		t.Fatalf("take: %d %v", got, err)
	}
	if _, err = takeExportedAuth(ctx, nil, raw, now); status.Convert(err).Message() != "AUTH_BYTES_INVALID" {
		t.Fatalf("second take: %v", err)
	}
	if _, err = takeExportedAuth(ctx, nil, []byte("other"), now); status.Convert(err).Message() != "AUTH_BYTES_INVALID" {
		t.Fatalf("unknown: %v", err)
	}
	if _, err = takeExportedAuth(ctx, nil, nil, now); status.Convert(err).Message() != "AUTH_BYTES_INVALID" {
		t.Fatalf("empty: %v", err)
	}
}

func TestExportedAuthExpires(t *testing.T) {
	memoryExports.Lock()
	memoryExports.rows = map[string]exportRecord{}
	memoryExports.Unlock()

	raw := []byte("expiring")
	now := time.Unix(1_700_000_000, 0)
	if err := putExportedAuth(context.Background(), nil, raw, 7, now); err != nil {
		t.Fatal(err)
	}
	later := now.Add(exportedAuthTTL*time.Second + time.Second)
	if _, err := takeExportedAuth(context.Background(), nil, raw, later); status.Convert(err).Message() != mtproto.ErrAuthBytesInvalid.Error() && status.Convert(err).Message() != "AUTH_BYTES_INVALID" {
		t.Fatalf("expired: %v", err)
	}
}
