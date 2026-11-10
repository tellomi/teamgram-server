package mediapurge

import (
	"context"
	"database/sql"
	"time"

	"github.com/teamgram/teamgram-server/app/service/mediablob"
)

const classVideos = "videos"

// Object is one stored file. Class is the dfs storage class (documents,
// photos, videos). Path is the key inside that class.
type Object struct {
	Class string
	Path  string
	Bytes int64
}

// Objects lists every key for a photo or document, including photo size
// variants and video thumbs. Retention keeps small thumbs. A user delete
// does not: once nothing else references the media, every variant goes.
func (s *Store) Objects(ctx context.Context, m Media) ([]Object, error) {
	if s == nil || s.db == nil || m.ID == 0 {
		return nil, nil
	}
	seen := map[Object]int64{}
	add := func(class, path string, size int64) {
		if path == "" || class == "" {
			return
		}
		key := Object{Class: class, Path: path}
		if prev, ok := seen[key]; !ok || prev == 0 {
			seen[key] = size
		}
	}
	if m.Kind == KindPhoto {
		if err := s.photoObjects(ctx, m.ID, add); err != nil {
			return nil, err
		}
	} else {
		if err := s.documentObjects(ctx, m.ID, add); err != nil {
			return nil, err
		}
	}
	if err := s.blobObjects(ctx, m, add); err != nil {
		return nil, err
	}
	out := make([]Object, 0, len(seen))
	for key, size := range seen {
		key.Bytes = size
		out = append(out, key)
	}
	return out, nil
}

func (s *Store) photoObjects(ctx context.Context, photoID int64, add func(string, string, int64)) error {
	rows, err := s.db.QueryContext(ctx, `
SELECT file_path FROM media_photo_paths WHERE photo_id=?`, photoID)
	if err != nil && !missing(err) {
		return err
	}
	if err == nil {
		for rows.Next() {
			var path string
			if err = rows.Scan(&path); err != nil {
				rows.Close()
				return err
			}
			add(mediablob.ClassPhotos, path, 0)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	var sizeID, videoID int64
	err = s.db.QueryRowContext(ctx, `
SELECT IF(size_id<>0, size_id, photo_id), video_size_id FROM photos WHERE photo_id=?`, photoID).Scan(&sizeID, &videoID)
	if err == sql.ErrNoRows || missing(err) {
		add(mediablob.ClassPhotos, "0/"+itoa(photoID)+".dat", 0)
		return nil
	}
	if err != nil {
		return err
	}
	if err = s.sizePaths(ctx, `SELECT file_path, file_size FROM photo_sizes WHERE photo_size_id=? AND file_path<>''`, sizeID, mediablob.ClassPhotos, add); err != nil {
		return err
	}
	if videoID != 0 {
		if err = s.sizePaths(ctx, `SELECT file_path, file_size FROM video_sizes WHERE video_size_id=? AND file_path<>''`, videoID, classVideos, add); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) documentObjects(ctx context.Context, documentID int64, add func(string, string, int64)) error {
	var path string
	var size, thumbID, videoID int64
	err := s.db.QueryRowContext(ctx, `
SELECT file_path, file_size, thumb_id, video_thumb_id FROM documents WHERE document_id=?`, documentID).Scan(&path, &size, &thumbID, &videoID)
	if err == sql.ErrNoRows || missing(err) {
		add(mediablob.ClassDocuments, itoa(documentID)+".dat", 0)
		return nil
	}
	if err != nil {
		return err
	}
	if path == "" {
		path = itoa(documentID) + ".dat"
	}
	add(mediablob.ClassDocuments, path, size)
	if thumbID != 0 {
		if err = s.sizePaths(ctx, `SELECT file_path, file_size FROM photo_sizes WHERE photo_size_id=? AND file_path<>''`, thumbID, mediablob.ClassPhotos, add); err != nil {
			return err
		}
	}
	if videoID != 0 {
		if err = s.sizePaths(ctx, `SELECT file_path, file_size FROM video_sizes WHERE video_size_id=? AND file_path<>''`, videoID, classVideos, add); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) sizePaths(ctx context.Context, q string, id int64, class string, add func(string, string, int64)) error {
	rows, err := s.db.QueryContext(ctx, q, id)
	if missing(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		var size int64
		if err = rows.Scan(&path, &size); err != nil {
			return err
		}
		add(class, path, size)
	}
	return rows.Err()
}

func (s *Store) blobObjects(ctx context.Context, m Media, add func(string, string, int64)) error {
	kind := mediablob.KindDocument
	if m.Kind == KindPhoto {
		kind = mediablob.KindPhoto
	}
	var class, path string
	var size int64
	err := s.db.QueryRowContext(ctx, `
SELECT b.storage_class, b.file_path, b.file_size
FROM media_blob_refs r JOIN media_blobs b ON b.id = r.blob_id
WHERE r.ref_kind=? AND r.ref_id=? LIMIT 1`, kind, m.ID).Scan(&class, &path, &size)
	if err == sql.ErrNoRows || missing(err) {
		return nil
	}
	if err != nil {
		return err
	}
	add(class, path, size)
	return nil
}

// pathHeld reports whether some other live blob, photo size, or document
// still points at this key. except is the media id being purged.
func (s *Store) pathHeld(ctx context.Context, class, path string, except Media) (bool, error) {
	if path == "" {
		return false, nil
	}
	blobs := mediablob.New(s.db)
	exceptBlob := mediablob.Except{}
	if info, ok, err := blobs.Ref(ctx, blobKind(except.Kind), except.ID); err != nil {
		return false, err
	} else if ok {
		exceptBlob.BlobID = info.BlobID
		if except.Kind == KindPhoto {
			exceptBlob.PhotoID = except.ID
		}
	}
	inUse, err := blobs.InUse(ctx, class, path, exceptBlob)
	if err != nil && !missing(err) {
		return false, err
	}
	if inUse {
		return true, nil
	}
	var docID int64
	if except.Kind == KindDocument {
		docID = except.ID
	}
	// A document row keeps file_path after its blob ref is released. That
	// string is not a live holder. Only a document that still has a blob
	// ref, or a video thumb owned by one, blocks deletion.
	ok, err := s.exists(ctx, `
SELECT 1 FROM documents d
JOIN media_blob_refs r ON r.ref_kind=? AND r.ref_id=d.document_id
WHERE d.file_path=? AND d.document_id<>? AND d.expired_at=0
LIMIT 1`, mediablob.KindDocument, path, docID)
	if err != nil || ok {
		return ok, err
	}
	ok, err = s.exists(ctx, `
SELECT 1 FROM video_sizes v
JOIN documents d ON d.video_thumb_id = v.video_size_id
JOIN media_blob_refs r ON r.ref_kind=? AND r.ref_id=d.document_id
WHERE v.file_path=? AND d.document_id<>? AND d.expired_at=0
LIMIT 1`, mediablob.KindDocument, path, docID)
	if err != nil || ok {
		return ok, err
	}
	return false, nil
}

// expirePath marks every document or photo that used this key and no longer
// has a blob ref. A re-upload sibling is expired together with the last copy
// so upload.getFile fails for both ids.
func (s *Store) expirePath(ctx context.Context, class, path string, now time.Time) error {
	if path == "" {
		return nil
	}
	at := now.Unix()
	if class == "" || class == mediablob.ClassDocuments {
		_, err := s.db.ExecContext(ctx, `
UPDATE documents d
LEFT JOIN media_blob_refs r ON r.ref_kind=? AND r.ref_id=d.document_id
SET d.expired_at=?
WHERE d.file_path=? AND d.expired_at=0 AND r.id IS NULL`, mediablob.KindDocument, at, path)
		if err != nil && !missing(err) && !missingColumn(err) {
			return err
		}
	}
	if class == "" || class == mediablob.ClassPhotos || class == classVideos {
		_, err := s.db.ExecContext(ctx, `
UPDATE photos p
JOIN photo_sizes s ON s.photo_size_id = IF(p.size_id<>0, p.size_id, p.photo_id)
LEFT JOIN media_blob_refs r ON r.ref_kind=? AND r.ref_id=p.photo_id
SET p.expired_at=?
WHERE s.file_path=? AND p.expired_at=0 AND r.id IS NULL`, mediablob.KindPhoto, at, path)
		if err != nil && !missing(err) && !missingColumn(err) {
			return err
		}
	}
	return nil
}

func blobKind(kind string) string {
	if kind == KindPhoto {
		return mediablob.KindPhoto
	}
	return mediablob.KindDocument
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
