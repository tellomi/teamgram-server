// Package mediablob is the content-addressed index for uploaded documents
// and photos. The object key is stored once. Each document or photo row
// keeps its own id and access_hash; a hash hit is not authorization.
package mediablob

import (
	"context"
	"database/sql"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	_ "github.com/go-sql-driver/mysql"
)

const (
	ClassDocuments = "documents"
	ClassPhotos    = "photos"

	KindDocument = "document"
	KindPhoto    = "photo"
)

// Store is the MySQL side of media_blobs. A nil Store means dedup is off
// and callers keep the historical <id>.dat key.
type Store struct {
	db *sql.DB
}

// Claim is the result of reserving a blob for one new document or photo.
type Claim struct {
	BlobID   int64
	Path     string
	SHA256   string
	Size     int64
	Class    string
	OriginID int64
	// Reused is true when Path already belongs to an earlier upload.
	// The caller must not write another object.
	Reused bool
}

// Info is the blob a document or photo row should point at.
type Info struct {
	BlobID int64
	SHA256 string
	Path   string
	Size   int64
}

// PhotoSize is one stored photo_sizes row, used to clone a reused photo.
type PhotoSize struct {
	Type        string
	W           int32
	H           int32
	FileSize    int32
	Path        string
	CachedType  int32
	CachedBytes string
}

// Key is one object a caller may delete after the last reference is gone.
type Key struct {
	Class string
	Path  string
}

// Except is the blob and photo a dry-run is pretending to release. Those
// rows are not a second live reference.
type Except struct {
	BlobID  int64
	PhotoID int64
}

// ReleaseResult is what remains after dropping one reference.
// Objects is set only when Remove is true: the blob row is already gone
// (or, for Peek, would be) and these keys are queued in media_blob_gc.
// Deleting them is the caller's job. A failed delete leaves the gc row and
// the object; it does not leave a media_blobs row or a dangling ref.
type ReleaseResult struct {
	Found   bool
	Remove  bool // this was the last reference; the object may be deleted
	Class   string
	Path    string
	BlobID  int64
	Objects []Key
}

// Open returns nil when dsn is empty. The pool connects on the first query.
func Open(dsn string) (*Store, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, nil
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	return New(db), nil
}

// New wraps an existing pool. The caller owns db.
func New(db *sql.DB) *Store {
	if db == nil {
		return nil
	}
	return &Store{db: db}
}

// Claim reserves the blob for refID. proposed is the object key used only
// when this call creates the row. On a hash hit the returned path is the
// existing key and Reused is true.
func (s *Store) Claim(ctx context.Context, class, kind string, refID int64, sha string, size int64, proposed string) (Claim, error) {
	out := Claim{Path: proposed, SHA256: sha, Size: size, Class: class, OriginID: refID}
	if s == nil || s.db == nil {
		return out, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Claim{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// LAST_INSERT_ID(id) makes LastInsertId return the existing row on a
	// duplicate hash, so two racing uploads share one blob.
	res, err := tx.ExecContext(ctx, `
INSERT INTO media_blobs (sha256, file_size, file_path, storage_class, ref_count, origin_ref_id)
VALUES (?, ?, ?, ?, 1, ?)
ON DUPLICATE KEY UPDATE ref_count = ref_count + 1, id = LAST_INSERT_ID(id)`,
		sha, size, proposed, class, refID)
	if err != nil {
		return Claim{}, err
	}
	blobID, err := res.LastInsertId()
	if err != nil {
		return Claim{}, err
	}
	var path string
	var origin int64
	if err = tx.QueryRowContext(ctx, `SELECT file_path, origin_ref_id FROM media_blobs WHERE id = ?`, blobID).Scan(&path, &origin); err != nil {
		return Claim{}, err
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO media_blob_refs (blob_id, ref_kind, ref_id) VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE blob_id = VALUES(blob_id)`, blobID, kind, refID); err != nil {
		return Claim{}, err
	}
	if err = tx.Commit(); err != nil {
		return Claim{}, err
	}
	out.BlobID = blobID
	out.Path = path
	out.OriginID = origin
	out.Reused = path != proposed
	return out, nil
}

// Ref returns the blob linked to a document or photo, if one was claimed.
func (s *Store) Ref(ctx context.Context, kind string, refID int64) (Info, bool, error) {
	if s == nil || s.db == nil || refID == 0 {
		return Info{}, false, nil
	}
	var info Info
	err := s.db.QueryRowContext(ctx, `
SELECT b.id, b.sha256, b.file_path, b.file_size
FROM media_blob_refs r
JOIN media_blobs b ON b.id = r.blob_id
WHERE r.ref_kind = ? AND r.ref_id = ?
LIMIT 1`, kind, refID).Scan(&info.BlobID, &info.SHA256, &info.Path, &info.Size)
	if err == sql.ErrNoRows {
		return Info{}, false, nil
	}
	if err != nil {
		return Info{}, false, err
	}
	return info, true, nil
}

// DocumentPath is the object key for a document, or empty when it was not
// claimed (historical rows stay at <id>.dat).
func (s *Store) DocumentPath(ctx context.Context, documentID int64) (string, error) {
	info, ok, err := s.Ref(ctx, KindDocument, documentID)
	if err != nil || !ok {
		return "", err
	}
	return info.Path, nil
}

// Release drops one reference. Remove is set when the object is no longer
// referenced. The blob row is deleted in this transaction when the count
// would hit zero, and the object keys are inserted into media_blob_gc.
// Missing refs are not an error. The row is never left with ref_count = 0.
func (s *Store) Release(ctx context.Context, kind string, refID int64) (ReleaseResult, error) {
	if s == nil || s.db == nil || refID == 0 {
		return ReleaseResult{}, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReleaseResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	rel, err := readRef(ctx, tx, kind, refID, true)
	if err != nil || !rel.Found {
		return rel, err
	}
	if rel.Remove {
		rel.Objects, err = objectKeys(ctx, tx, kind, refID, rel.Class, rel.Path)
		if err != nil {
			return ReleaseResult{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM media_blob_refs WHERE ref_kind = ? AND ref_id = ?`, kind, refID); err != nil {
		return ReleaseResult{}, err
	}
	if kind == KindPhoto {
		if _, err = tx.ExecContext(ctx, `DELETE FROM media_photo_paths WHERE photo_id = ?`, refID); err != nil {
			return ReleaseResult{}, err
		}
	}
	if rel.Remove {
		for _, key := range rel.Objects {
			if _, err = tx.ExecContext(ctx, `
INSERT INTO media_blob_gc (storage_class, file_path) VALUES (?, ?)
ON DUPLICATE KEY UPDATE storage_class = storage_class`, key.Class, key.Path); err != nil {
				return ReleaseResult{}, err
			}
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM media_blobs WHERE id = ?`, rel.BlobID); err != nil {
			return ReleaseResult{}, err
		}
	} else if _, err = tx.ExecContext(ctx, `UPDATE media_blobs SET ref_count = ref_count - 1 WHERE id = ? AND ref_count > 1`, rel.BlobID); err != nil {
		return ReleaseResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return ReleaseResult{}, err
	}
	return rel, nil
}

// Peek reports what Release would do without changing rows. Dry-runs use it
// so a shared object is not listed as something that would be deleted.
func (s *Store) Peek(ctx context.Context, kind string, refID int64) (ReleaseResult, error) {
	if s == nil || s.db == nil || refID == 0 {
		return ReleaseResult{}, nil
	}
	rel, err := readRef(ctx, s.db, kind, refID, false)
	if err != nil || !rel.Found || !rel.Remove {
		return rel, err
	}
	rel.Objects, err = objectKeys(ctx, s.db, kind, refID, rel.Class, rel.Path)
	return rel, err
}

type rowQuery interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func readRef(ctx context.Context, q rowQuery, kind string, refID int64, lock bool) (ReleaseResult, error) {
	stmt := `
SELECT b.id, b.storage_class, b.file_path, b.ref_count
FROM media_blob_refs r
JOIN media_blobs b ON b.id = r.blob_id
WHERE r.ref_kind = ? AND r.ref_id = ?`
	if lock {
		stmt += ` FOR UPDATE`
	}
	var blobID, count int64
	var class, path string
	err := q.QueryRowContext(ctx, stmt, kind, refID).Scan(&blobID, &class, &path, &count)
	if err == sql.ErrNoRows {
		return ReleaseResult{}, nil
	}
	if err != nil {
		return ReleaseResult{}, err
	}
	return ReleaseResult{
		Found:  true,
		Remove: count <= 1,
		Class:  class,
		Path:   path,
		BlobID: blobID,
	}, nil
}

// objectKeys are the full-size objects released with the last reference.
// Photo thumbnails s, i and j stay. Document thumbnails are not in this list.
func objectKeys(ctx context.Context, q rowQuery, kind string, refID int64, class, path string) ([]Key, error) {
	if kind != KindPhoto {
		if path == "" {
			return nil, nil
		}
		if class == "" {
			class = ClassDocuments
		}
		return []Key{{Class: class, Path: path}}, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT size_type, file_path FROM media_photo_paths WHERE photo_id = ?`, refID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var out []Key
	add := func(class, path string) {
		if path == "" {
			return
		}
		k := class + "\x00" + path
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, Key{Class: class, Path: path})
	}
	for rows.Next() {
		var sizeType, filePath string
		if err = rows.Scan(&sizeType, &filePath); err != nil {
			return nil, err
		}
		if keptThumb(sizeType) {
			continue
		}
		add(ClassPhotos, filePath)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if class == "" {
		class = ClassPhotos
	}
	add(class, path)
	return out, nil
}

func keptThumb(sizeType string) bool {
	switch sizeType {
	case "s", "i", "j":
		return true
	}
	return false
}

// PendingGC lists object keys whose blob row is already gone. The caller
// deletes the object, then AckGC. A delete failure leaves the row so the
// next pass can retry. The object stays an orphan rather than a dangling ref.
func (s *Store) PendingGC(ctx context.Context, limit int) ([]Key, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT storage_class, file_path FROM media_blob_gc ORDER BY id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Key
	for rows.Next() {
		var key Key
		if err = rows.Scan(&key.Class, &key.Path); err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

// AckGC drops a queued key after the object delete succeeded, or when a
// newer blob is using that key again.
func (s *Store) AckGC(ctx context.Context, class, path string) error {
	if s == nil || s.db == nil || path == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM media_blob_gc WHERE storage_class = ? AND file_path = ?`, class, path)
	return err
}

// PathInUse reports whether a live blob or photo size still points at
// class/path. The retention sweeper uses this so a retried expire cannot
// delete an object another document or photo still references.
func (s *Store) PathInUse(ctx context.Context, class, path string) (bool, error) {
	return s.InUse(ctx, class, path, Except{})
}

// InUse is PathInUse, ignoring the blob and photo named by except. A dry-run
// of the last reference still sees that row and must not treat it as a
// second holder.
func (s *Store) InUse(ctx context.Context, class, path string, except Except) (bool, error) {
	if s == nil || s.db == nil || path == "" {
		return false, nil
	}
	var n int
	err := s.db.QueryRowContext(ctx, `
SELECT 1 FROM media_blobs
WHERE storage_class = ? AND file_path = ? AND ref_count > 0 AND id <> ?
LIMIT 1`, class, path, except.BlobID).Scan(&n)
	if err == nil {
		return true, nil
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	err = s.db.QueryRowContext(ctx, `
SELECT 1 FROM media_photo_paths p
JOIN media_blob_refs r ON r.ref_kind = ? AND r.ref_id = p.photo_id
WHERE p.file_path = ? AND p.photo_id <> ?
LIMIT 1`, KindPhoto, path, except.PhotoID).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// FindDocument returns the oldest live document with this hash, size and mime.
// Zero means documentEmpty. The hash is lowercase hex.
func (s *Store) FindDocument(ctx context.Context, shaHex string, size int64, mime string) (int64, error) {
	if s == nil || s.db == nil || shaHex == "" || mime == "" {
		return 0, nil
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `
SELECT document_id FROM documents
WHERE sha256 = ? AND file_size = ? AND mime_type = ? AND expired_at = 0
ORDER BY id ASC
LIMIT 1`, shaHex, size, mime).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

// FindDocumentIDs returns live documents with this hash, oldest first.
// Callers pick one the user may see instead of always taking the oldest row.
func (s *Store) FindDocumentIDs(ctx context.Context, shaHex string, size int64, mime string) ([]int64, error) {
	if s == nil || s.db == nil || shaHex == "" || mime == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT document_id FROM documents
WHERE sha256 = ? AND file_size = ? AND mime_type = ? AND expired_at = 0
ORDER BY id ASC
LIMIT 32`, shaHex, size, mime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		if id != 0 {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

// SavePhotoPath records the object key for one size of a photo.
func (s *Store) SavePhotoPath(ctx context.Context, photoID int64, sizeType, path string) error {
	if s == nil || s.db == nil || photoID == 0 || sizeType == "" || path == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO media_photo_paths (photo_id, size_type, file_path) VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE file_path = VALUES(file_path)`, photoID, sizeType, path)
	return err
}

// PhotoPath is the shared key for a photo size, or empty when this photo
// owns its own <type>/<id>.dat object.
func (s *Store) PhotoPath(ctx context.Context, photoID int64, sizeType string) (string, error) {
	if s == nil || s.db == nil || photoID == 0 || sizeType == "" {
		return "", nil
	}
	var path string
	err := s.db.QueryRowContext(ctx, `
SELECT file_path FROM media_photo_paths WHERE photo_id = ? AND size_type = ?`, photoID, sizeType).Scan(&path)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return path, err
}

// PhotoPaths lists every recorded size key for a photo.
func (s *Store) PhotoPaths(ctx context.Context, photoID int64) (map[string]string, error) {
	out := map[string]string{}
	if s == nil || s.db == nil || photoID == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT size_type, file_path FROM media_photo_paths WHERE photo_id = ?`, photoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t, p string
		if err = rows.Scan(&t, &p); err != nil {
			return nil, err
		}
		out[t] = p
	}
	return out, rows.Err()
}

// ListPhotoSizes reads the origin photo's size rows so a reused photo can
// describe the same thumbs without writing them again.
func (s *Store) ListPhotoSizes(ctx context.Context, photoID int64) ([]PhotoSize, error) {
	if s == nil || s.db == nil || photoID == 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT size_type, width, height, file_size, file_path, cached_type, cached_bytes
FROM photo_sizes WHERE photo_size_id = ? ORDER BY id ASC`, photoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PhotoSize
	for rows.Next() {
		var sz PhotoSize
		if err = rows.Scan(&sz.Type, &sz.W, &sz.H, &sz.FileSize, &sz.Path, &sz.CachedType, &sz.CachedBytes); err != nil {
			return nil, err
		}
		out = append(out, sz)
	}
	return out, rows.Err()
}
