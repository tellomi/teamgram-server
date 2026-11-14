package dao

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// exportedAuthTTL is how long auth.exportAuthorization bytes stay valid.
// Telegram treats the blob as a one-shot cross-DC credential.
const exportedAuthTTL = 300

type exportKV interface {
	GetCtx(ctx context.Context, key string) (string, error)
	SetexCtx(ctx context.Context, key, value string, seconds int) error
	DelCtx(ctx context.Context, keys ...string) (int, error)
}

type exportRecord struct {
	userID  int64
	expires time.Time
}

var memoryExports = struct {
	sync.Mutex
	rows map[string]exportRecord
}{rows: map[string]exportRecord{}}

func exportAuthKey(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "auth_export_" + hex.EncodeToString(sum[:])
}

// PutExportedAuth stores a one-shot blob for userID. The bytes themselves
// are the secret; the key is their hash.
func (d *Dao) PutExportedAuth(ctx context.Context, raw []byte, userID int64) error {
	if len(raw) == 0 || userID == 0 {
		return mtproto.ErrAuthBytesInvalid
	}
	return putExportedAuth(ctx, exportKVOf(d), raw, userID, time.Now())
}

// TakeExportedAuth returns the user id bound to raw and forgets the blob.
// A second take, a mismatch, or an expired blob is AUTH_BYTES_INVALID.
func (d *Dao) TakeExportedAuth(ctx context.Context, raw []byte) (int64, error) {
	return takeExportedAuth(ctx, exportKVOf(d), raw, time.Now())
}

func exportKVOf(d *Dao) exportKV {
	if d == nil || d.kv == nil {
		return nil
	}
	return d.kv
}

func putExportedAuth(ctx context.Context, kv exportKV, raw []byte, userID int64, now time.Time) error {
	key := exportAuthKey(raw)
	value := strconv.FormatInt(userID, 10)
	if kv == nil {
		memoryExports.Lock()
		memoryExports.rows[key] = exportRecord{userID: userID, expires: now.Add(exportedAuthTTL * time.Second)}
		memoryExports.Unlock()
		return nil
	}
	if err := kv.SetexCtx(ctx, key, value, exportedAuthTTL); err != nil {
		return fmt.Errorf("export authorization: %w", err)
	}
	return nil
}

func takeExportedAuth(ctx context.Context, kv exportKV, raw []byte, now time.Time) (int64, error) {
	if len(raw) == 0 {
		return 0, mtproto.ErrAuthBytesInvalid
	}
	key := exportAuthKey(raw)
	if kv == nil {
		memoryExports.Lock()
		defer memoryExports.Unlock()
		row, ok := memoryExports.rows[key]
		if !ok || !row.expires.After(now) || row.userID == 0 {
			delete(memoryExports.rows, key)
			return 0, mtproto.ErrAuthBytesInvalid
		}
		delete(memoryExports.rows, key)
		return row.userID, nil
	}
	value, err := kv.GetCtx(ctx, key)
	if err != nil || value == "" {
		return 0, mtproto.ErrAuthBytesInvalid
	}
	_, _ = kv.DelCtx(ctx, key)
	userID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || userID == 0 {
		return 0, mtproto.ErrAuthBytesInvalid
	}
	return userID, nil
}

// NewExportBytes is the secret half of auth.exportedAuthorization.
func NewExportBytes() ([]byte, error) {
	raw := make([]byte, 128)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	return raw, nil
}
