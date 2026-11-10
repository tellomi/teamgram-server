// Package objectstore opens the dfs S3 client from TELLOMI_S3_*.
// Sticker import and upload.getFile must share this client. A second
// minio.New call drifts on TLS, region, and path style, and then GetObject
// reports that stickers/<id>.dat does not exist.
package objectstore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
)

// ErrNotFound is a missing object. Import uses it to decide whether to upload.
var ErrNotFound = errors.New("object does not exist")

// testRoundTripper is set by tests that talk to a local TLS server.
var testRoundTripper http.RoundTripper

// SetRoundTripperForTest installs the transport Open uses. Tests call it
// with a client that trusts httptest's certificate. Pass nil to clear it.
func SetRoundTripperForTest(rt http.RoundTripper) {
	testRoundTripper = rt
}

// Client is the dfs object client. Puts and gets go through resolveLocation,
// so a sticker written here is the object GetStickerFile reads.
type Client struct {
	u *minio_util.MinioUtil
}

// Open builds the client dfs uses after MinioConfig.ApplyEnv.
// It returns nil, nil when the S3 variables are unset so the catalog can
// still be served without object storage.
func Open() (*Client, error) {
	endpoint := strings.TrimSpace(os.Getenv(minio_util.EnvEndpoint))
	keyID := os.Getenv(minio_util.EnvAccessKeyID)
	secret := os.Getenv(minio_util.EnvSecretAccessKey)
	bucket := strings.TrimSpace(os.Getenv(minio_util.EnvBucketPublicMedia))
	attachments := strings.TrimSpace(os.Getenv(minio_util.EnvBucketAttachments))
	if endpoint == "" && keyID == "" && secret == "" && bucket == "" && attachments == "" {
		return nil, nil
	}
	if endpoint == "" || keyID == "" || secret == "" || bucket == "" || attachments == "" {
		return nil, fmt.Errorf("object storage needs %s, %s, %s, %s, and %s",
			minio_util.EnvEndpoint, minio_util.EnvAccessKeyID, minio_util.EnvSecretAccessKey,
			minio_util.EnvBucketAttachments, minio_util.EnvBucketPublicMedia)
	}
	cfg := minio_util.MinioConfig{}
	if err := cfg.ApplyEnv(); err != nil {
		return nil, err
	}
	if testRoundTripper != nil {
		cfg.SetTransportForTest(testRoundTripper)
	}
	u, err := minio_util.NewMinioClient(&cfg)
	if err != nil {
		return nil, err
	}
	return &Client{u: u}, nil
}

// Put writes body to the public-media key for class (stickers or emoji).
// path is the object name inside that class, such as "<document id>.dat".
func (c *Client) Put(ctx context.Context, class, path, contentType string, body []byte) error {
	if c == nil || c.u == nil {
		return fmt.Errorf("object storage client is nil")
	}
	switch class {
	case minio_util.ClassStickers, minio_util.ClassEmoji:
	default:
		return fmt.Errorf("object storage class %q is not %s or %s", class, minio_util.ClassStickers, minio_util.ClassEmoji)
	}
	if _, err := c.u.PutClassBytes(ctx, class, path, contentType, body); err != nil {
		return fmt.Errorf("put %s/%s: %w", class, path, err)
	}
	return nil
}

// Head is the size and ETag of one object. ETag is the S3 value, quotes included.
type Head struct {
	Size int64
	ETag string
}

// List returns objects already stored for a class, keyed by the path Put uses.
// The sticker seed compares these ETags to the embedded bytes and skips
// unchanged objects.
func (c *Client) List(ctx context.Context, class string) (map[string]Head, error) {
	if c == nil || c.u == nil {
		return nil, fmt.Errorf("object storage client is nil")
	}
	raw, err := c.u.ListClass(ctx, class)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Head, len(raw))
	for path, h := range raw {
		out[path] = Head{Size: h.Size, ETag: h.ETag}
	}
	return out, nil
}

// Size is the stored object size. A missing object returns ErrNotFound.
func (c *Client) Size(ctx context.Context, class, path string) (int64, error) {
	if c == nil || c.u == nil {
		return 0, fmt.Errorf("object storage client is nil")
	}
	n, err := c.u.StatClass(ctx, class, path)
	if errors.Is(err, minio_util.ErrNotFound) {
		return 0, ErrNotFound
	}
	return n, err
}

// Get reads the object upload.getFile reads for this class and path.
func (c *Client) Get(ctx context.Context, class, path string, limit int32) ([]byte, error) {
	if c == nil || c.u == nil {
		return nil, fmt.Errorf("object storage client is nil")
	}
	switch class {
	case minio_util.ClassStickers:
		return c.u.GetStickerFile(ctx, path, 0, limit)
	case minio_util.ClassEmoji:
		return c.u.GetEmojiFile(ctx, path, 0, limit)
	default:
		return nil, fmt.Errorf("object storage class %q is not %s or %s", class, minio_util.ClassStickers, minio_util.ClassEmoji)
	}
}

// Chat media classes Delete accepts. They mirror minio_util so callers
// outside dfs do not import an internal package.
const (
	ClassDocuments      = minio_util.ClassDocuments
	ClassPhotos         = minio_util.ClassPhotos
	ClassVideos         = minio_util.ClassVideos
	ClassEncryptedFiles = minio_util.ClassEncryptedFiles
)

// Delete removes one chat-media object (documents, photos, videos or
// encryptedfiles). Public media classes are refused. A missing object is
// not an error, so a retried sweep is safe.
func (c *Client) Delete(ctx context.Context, class, path string) error {
	if c == nil || c.u == nil {
		return fmt.Errorf("object storage client is nil")
	}
	if err := c.u.RemoveClassObject(ctx, class, path); err != nil {
		return fmt.Errorf("delete %s/%s: %w", class, path, err)
	}
	return nil
}

// Endpoint is the S3 API host this client writes to. Account deletion logs
// it so an operator can see whether the process is talking to R2 or MinIO.
func (c *Client) Endpoint() string {
	if c == nil || c.u == nil {
		return ""
	}
	return c.u.Endpoint()
}

// MoveToTrash copies one object to the trash/ prefix in the same bucket and
// deletes the live key. Avatars are allowed. Stickers, emoji and wallpapers
// are refused. A missing object is success.
func (c *Client) MoveToTrash(ctx context.Context, class, path string) error {
	if c == nil || c.u == nil {
		return fmt.Errorf("object storage client is nil")
	}
	if err := c.u.MoveToTrash(ctx, class, path); err != nil {
		return fmt.Errorf("trash %s/%s: %w", class, path, err)
	}
	return nil
}

// DeleteTrash removes the trash/ copy written by MoveToTrash.
func (c *Client) DeleteTrash(ctx context.Context, class, path string) error {
	if c == nil || c.u == nil {
		return fmt.Errorf("object storage client is nil")
	}
	if err := c.u.DeleteTrash(ctx, class, path); err != nil {
		return fmt.Errorf("delete trash %s/%s: %w", class, path, err)
	}
	return nil
}
