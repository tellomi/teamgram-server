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
// object keys for the new photo. ok is false when the origin has no sizes
// yet; the caller then writes its own derivatives.
func (c *DfsCore) reusedPhotoSizes(ctx context.Context, photoID int64, claim mediablob.Claim) ([]*mtproto.PhotoSize, bool) {
	if c.svcCtx.Dao.Blobs == nil || claim.OriginID == 0 {
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
		if err = c.svcCtx.Dao.Blobs.SavePhotoPath(ctx, photoID, row.Type, row.Path); err != nil {
			c.Logger.Errorf("dfs.savePhotoPath - %v", err)
		}
		if sz := photoSizeFromRow(row); sz != nil {
			out = append(out, sz)
		}
	}
	if err = c.svcCtx.Dao.Blobs.SavePhotoPath(ctx, photoID, "0", claim.Path); err != nil {
		c.Logger.Errorf("dfs.savePhotoPath 0 - %v", err)
	}
	return out, len(out) > 0
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
