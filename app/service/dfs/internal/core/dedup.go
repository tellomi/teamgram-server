package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/teamgram/marmota/pkg/threading2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/mediablob"

	"github.com/zeromicro/go-zero/core/jsonx"
)

// cached photo_sizes.cached_type values. They match media/internal/dao.
const (
	cachedTypeSize            = 0
	cachedTypePathSize        = 1
	cachedTypeStrippedSize    = 2
	cachedTypeCachedSize      = 3
	cachedTypeSizeProgressive = 4
)

// storeDocumentBytes claims the documents blob for these bytes and writes
// them only when this call owns the object. A hash hit returns the existing
// key and writes nothing. async matches the historical upload path, which
// returns the document before the object is durable.
func (c *DfsCore) claimDocumentBytes(ctx context.Context, documentID int64, data []byte) mediablob.Claim {
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	claim, err := c.claimDocument(ctx, documentID, sha, int64(len(data)))
	if err != nil {
		c.Logger.Errorf("dfs.claimDocument - %v", err)
		return mediablob.Claim{Path: fmt.Sprintf("%d.dat", documentID), SHA256: sha, Size: int64(len(data))}
	}
	return claim
}

func (c *DfsCore) storeDocumentBytes(ctx context.Context, documentID int64, data []byte, async bool) (mediablob.Claim, error) {
	claim := c.claimDocumentBytes(ctx, documentID, data)
	if claim.Reused {
		return claim, nil
	}
	write := func(ctx context.Context) {
		if _, err2 := c.svcCtx.Dao.PutDocumentFile(ctx, claim.Path, bytes.NewReader(data)); err2 != nil {
			c.Logger.Errorf("dfs.PutDocumentFile %s - %v", claim.Path, err2)
		}
	}
	if async {
		threading2.GoSafeContext(ctx, write)
		return claim, nil
	}
	if _, err2 := c.svcCtx.Dao.PutDocumentFile(ctx, claim.Path, bytes.NewReader(data)); err2 != nil {
		return claim, err2
	}
	return claim, nil
}

// storeDocumentReader hashes a seekable upload and writes it when the blob
// is new. The reader is left at the start when the object still needs to be
// written by the caller; on a hit the caller must not write.
func (c *DfsCore) claimDocumentReader(ctx context.Context, documentID int64, r io.ReadSeeker) (mediablob.Claim, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return mediablob.Claim{}, err
	}
	if _, err = r.Seek(0, io.SeekStart); err != nil {
		return mediablob.Claim{}, err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	claim, err := c.claimDocument(ctx, documentID, sum, n)
	if err != nil {
		c.Logger.Errorf("dfs.claimDocument - %v", err)
		return mediablob.Claim{Path: fmt.Sprintf("%d.dat", documentID), SHA256: sum, Size: n}, nil
	}
	return claim, nil
}

func (c *DfsCore) claimDocument(ctx context.Context, documentID int64, sha string, size int64) (mediablob.Claim, error) {
	proposed := fmt.Sprintf("%d.dat", documentID)
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Blobs == nil {
		return mediablob.Claim{Path: proposed, SHA256: sha, Size: size, OriginID: documentID}, nil
	}
	return c.svcCtx.Dao.Blobs.Claim(ctx, mediablob.ClassDocuments, mediablob.KindDocument, documentID, sha, size, proposed)
}

func (c *DfsCore) claimPhoto(ctx context.Context, photoID int64, data []byte) (mediablob.Claim, error) {
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	proposed := fmt.Sprintf("0/%d.dat", photoID)
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Blobs == nil {
		return mediablob.Claim{Path: proposed, SHA256: sha, Size: int64(len(data)), OriginID: photoID}, nil
	}
	claim, err := c.svcCtx.Dao.Blobs.Claim(ctx, mediablob.ClassPhotos, mediablob.KindPhoto, photoID, sha, int64(len(data)), proposed)
	if err != nil {
		c.Logger.Errorf("dfs.claimPhoto - %v", err)
		return mediablob.Claim{Path: proposed, SHA256: sha, Size: int64(len(data)), OriginID: photoID}, nil
	}
	return claim, nil
}

// reusedPhotoSizes clones the origin photo's size list and records those
// object keys for the new photo or document. ok is false when this upload
// owns the blob, or the origin has no sizes yet; the caller then writes
// its own derivatives.
func (c *DfsCore) reusedPhotoSizes(ctx context.Context, photoID int64, claim mediablob.Claim) ([]*mtproto.PhotoSize, bool) {
	if !claim.Reused || c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Blobs == nil {
		return nil, false
	}
	if claim.OriginID == 0 || claim.OriginID == photoID {
		return nil, false
	}
	rows, err := c.svcCtx.Dao.Blobs.ListPhotoSizes(ctx, claim.OriginID)
	if err != nil {
		c.Logger.Errorf("dfs.reusedPhotoSizes - %v", err)
		return nil, false
	}
	if len(rows) == 0 {
		return nil, false
	}
	out := make([]*mtproto.PhotoSize, 0, len(rows))
	for _, row := range rows {
		c.rememberPhotoPath(ctx, photoID, row.Type, storedPhotoPath(row.Type, row.Path, claim.OriginID, row.CachedType))
		if sz := photoSizeFromRow(row); sz != nil {
			out = append(out, sz)
		}
	}
	c.rememberPhotoPath(ctx, photoID, "0", storedPhotoPath("0", claim.Path, claim.OriginID, cachedTypeSize))
	return out, len(out) > 0
}

// originSizePaths is the first upload's recorded keys. A second upload that
// arrives before photo_sizes is inserted still points at those keys instead
// of writing another object.
func (c *DfsCore) originSizePaths(ctx context.Context, claim mediablob.Claim) map[string]string {
	out := map[string]string{}
	if !claim.Reused || claim.OriginID == 0 || c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Blobs == nil {
		return out
	}
	paths, err := c.svcCtx.Dao.Blobs.PhotoPaths(ctx, claim.OriginID)
	if err != nil {
		c.Logger.Errorf("dfs.originSizePaths - %v", err)
		return out
	}
	if paths == nil {
		return out
	}
	return paths
}

func (c *DfsCore) rememberPhotoPath(ctx context.Context, photoID int64, sizeType, path string) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Blobs == nil {
		return
	}
	if err := c.svcCtx.Dao.Blobs.SavePhotoPath(ctx, photoID, sizeType, path); err != nil {
		c.Logger.Errorf("dfs.savePhotoPath %s - %v", sizeType, err)
	}
}

// abandonClaim drops a reference when the upload returns an error before
// the document or photo row is saved. A shared object stays.
func (c *DfsCore) abandonClaim(ctx context.Context, kind string, id int64) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Blobs == nil || id == 0 {
		return
	}
	if _, err := c.svcCtx.Dao.Blobs.Release(ctx, kind, id); err != nil {
		c.Logger.Errorf("dfs.release %s %d - %v", kind, id, err)
	}
}

// storedPhotoPath is the object key for a photo size. Inline sizes (stripped,
// path, cached bytes) have no object. An empty stored path uses the first
// upload's conventional <type>/<id>.dat key.
func storedPhotoPath(sizeType, stored string, originID int64, cachedType int32) string {
	switch cachedType {
	case cachedTypePathSize, cachedTypeStrippedSize, cachedTypeCachedSize:
		return ""
	}
	if stored != "" {
		return stored
	}
	if originID == 0 || sizeType == "" {
		return ""
	}
	return fmt.Sprintf("%s/%d.dat", sizeType, originID)
}

func photoSizeFromRow(row mediablob.PhotoSize) *mtproto.PhotoSize {
	switch row.CachedType {
	case cachedTypePathSize:
		b, _ := base64.RawStdEncoding.DecodeString(row.CachedBytes)
		if len(b) == 0 {
			return nil
		}
		return mtproto.MakeTLPhotoPathSize(&mtproto.PhotoSize{Type: row.Type, Bytes: b}).To_PhotoSize()
	case cachedTypeStrippedSize:
		b, _ := base64.RawStdEncoding.DecodeString(row.CachedBytes)
		if len(b) == 0 {
			return nil
		}
		return mtproto.MakeTLPhotoStrippedSize(&mtproto.PhotoSize{Type: row.Type, Bytes: b}).To_PhotoSize()
	case cachedTypeCachedSize:
		b, _ := base64.RawStdEncoding.DecodeString(row.CachedBytes)
		if len(b) == 0 {
			return nil
		}
		return mtproto.MakeTLPhotoCachedSize(&mtproto.PhotoSize{Type: row.Type, W: row.W, H: row.H, Bytes: b}).To_PhotoSize()
	case cachedTypeSizeProgressive:
		if row.CachedBytes == "" {
			return nil
		}
		var sizes []int32
		if err := jsonx.UnmarshalFromString(row.CachedBytes, &sizes); err != nil {
			return nil
		}
		return mtproto.MakeTLPhotoSizeProgressive(&mtproto.PhotoSize{Type: row.Type, W: row.W, H: row.H, Sizes: sizes}).To_PhotoSize()
	default:
		return mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{Type: row.Type, W: row.W, H: row.H, Size2: row.FileSize}).To_PhotoSize()
	}
}

func applyDocumentSize(doc *mtproto.Document, size int64) {
	if doc == nil || size <= 0 {
		return
	}
	doc.Size2 = size
	doc.Size2_INT32 = int32(size)
	doc.Size2_INT64 = size
}
