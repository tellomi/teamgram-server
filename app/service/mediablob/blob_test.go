package mediablob

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"
)

func TestClaimReusesPathAndReleaseKeepsSharedObject(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS media_blob_gc, media_blob_refs, media_photo_paths, media_blobs`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE media_blobs (
		  id bigint NOT NULL AUTO_INCREMENT,
		  sha256 char(64) NOT NULL,
		  file_size bigint NOT NULL,
		  file_path varchar(255) NOT NULL,
		  storage_class varchar(32) NOT NULL,
		  ref_count int NOT NULL DEFAULT 1,
		  origin_ref_id bigint NOT NULL DEFAULT 0,
		  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		  PRIMARY KEY (id),
		  UNIQUE KEY uk_media_blobs_hash (storage_class, sha256, file_size)
		)`,
		`CREATE TABLE media_blob_refs (
		  id bigint NOT NULL AUTO_INCREMENT,
		  blob_id bigint NOT NULL,
		  ref_kind varchar(16) NOT NULL,
		  ref_id bigint NOT NULL,
		  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  PRIMARY KEY (id),
		  UNIQUE KEY uk_media_blob_refs_ref (ref_kind, ref_id)
		)`,
		`CREATE TABLE media_photo_paths (
		  photo_id bigint NOT NULL,
		  size_type varchar(8) NOT NULL,
		  file_path varchar(255) NOT NULL,
		  PRIMARY KEY (photo_id, size_type)
		)`,
		`CREATE TABLE media_blob_gc (
		  id bigint NOT NULL AUTO_INCREMENT,
		  storage_class varchar(32) NOT NULL,
		  file_path varchar(255) NOT NULL,
		  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  PRIMARY KEY (id),
		  UNIQUE KEY uk_media_blob_gc_path (storage_class, file_path)
		)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}

	s := New(db)
	const sha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	first, err := s.Claim(ctx, ClassDocuments, KindDocument, 11, sha, 40, "11.dat")
	if err != nil {
		t.Fatal(err)
	}
	if first.Reused || first.Path != "11.dat" || first.OriginID != 11 {
		t.Fatalf("first claim: %+v", first)
	}
	second, err := s.Claim(ctx, ClassDocuments, KindDocument, 22, sha, 40, "22.dat")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Reused || second.Path != "11.dat" || second.BlobID != first.BlobID || second.OriginID != 11 {
		t.Fatalf("second claim: %+v", second)
	}
	path, err := s.DocumentPath(ctx, 22)
	if err != nil || path != "11.dat" {
		t.Fatalf("document path %q err %v", path, err)
	}
	var blobs, refs int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_blobs`).Scan(&blobs); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT ref_count FROM media_blobs WHERE id = ?`, first.BlobID).Scan(&refs); err != nil {
		t.Fatal(err)
	}
	if blobs != 1 || refs != 2 {
		t.Fatalf("blobs=%d ref_count=%d", blobs, refs)
	}

	rel, err := s.Release(ctx, KindDocument, 11)
	if err != nil {
		t.Fatal(err)
	}
	if !rel.Found || rel.Remove || rel.Path != "11.dat" {
		t.Fatalf("release first: %+v", rel)
	}
	inUse, err := s.PathInUse(ctx, ClassDocuments, "11.dat")
	if err != nil || !inUse {
		t.Fatalf("path in use after one release: %v %v", inUse, err)
	}
	rel, err = s.Release(ctx, KindDocument, 22)
	if err != nil {
		t.Fatal(err)
	}
	if !rel.Found || !rel.Remove {
		t.Fatalf("release last: %+v", rel)
	}
	inUse, err = s.PathInUse(ctx, ClassDocuments, "11.dat")
	if err != nil || inUse {
		t.Fatalf("path still in use: %v %v", inUse, err)
	}
	var zeros, blobsLeft, queued int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_blobs WHERE ref_count = 0`).Scan(&zeros); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_blobs`).Scan(&blobsLeft); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_blob_gc WHERE storage_class = 'documents' AND file_path = '11.dat'`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if zeros != 0 || blobsLeft != 0 || queued != 1 || len(rel.Objects) != 1 || rel.Objects[0].Path != "11.dat" {
		t.Fatalf("zeros=%d blobs=%d queued=%d objects=%+v", zeros, blobsLeft, queued, rel.Objects)
	}
	pending, err := s.PendingGC(ctx, 10)
	if err != nil || len(pending) != 1 || pending[0].Path != "11.dat" {
		t.Fatalf("pending %+v %v", pending, err)
	}
	if err = s.AckGC(ctx, ClassDocuments, "11.dat"); err != nil {
		t.Fatal(err)
	}
	pending, err = s.PendingGC(ctx, 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after ack %+v %v", pending, err)
	}
}

func TestReleaseKeepsPhotoThumbsAndQueuesFullSizes(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS media_blob_gc, media_blob_refs, media_photo_paths, media_blobs`); err != nil {
		t.Fatal(err)
	}
	s := New(db)
	if err := createBlobTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	const sha = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := s.Claim(ctx, ClassPhotos, KindPhoto, 7, sha, 100, "0/7.dat"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(ctx, ClassPhotos, KindPhoto, 8, sha, 100, "0/8.dat"); err != nil {
		t.Fatal(err)
	}
	for _, photoID := range []int64{7, 8} {
		for _, pair := range [][2]string{{"0", "0/7.dat"}, {"m", "m/7.dat"}, {"s", "s/7.dat"}, {"i", "i/7.dat"}} {
			if err := s.SavePhotoPath(ctx, photoID, pair[0], pair[1]); err != nil {
				t.Fatal(err)
			}
		}
	}
	rel, err := s.Release(ctx, KindPhoto, 7)
	if err != nil || !rel.Found || rel.Remove {
		t.Fatalf("first photo: %+v %v", rel, err)
	}
	inUse, err := s.PathInUse(ctx, ClassPhotos, "m/7.dat")
	if err != nil || !inUse {
		t.Fatalf("shared size still referenced: %v %v", inUse, err)
	}
	var queued int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_blob_gc`).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("queued %d err %v", queued, err)
	}
	rel, err = s.Release(ctx, KindPhoto, 8)
	if err != nil || !rel.Remove {
		t.Fatalf("last photo: %+v %v", rel, err)
	}
	got := map[string]bool{}
	for _, key := range rel.Objects {
		got[key.Class+"/"+key.Path] = true
	}
	for _, want := range []string{"photos/0/7.dat", "photos/m/7.dat"} {
		if !got[want] {
			t.Fatalf("missing %s in %+v", want, rel.Objects)
		}
	}
	if got["photos/s/7.dat"] || got["photos/i/7.dat"] {
		t.Fatalf("thumbnail queued: %+v", rel.Objects)
	}
	var zeros int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_blobs WHERE ref_count = 0`).Scan(&zeros); err != nil || zeros != 0 {
		t.Fatalf("zeros %d err %v", zeros, err)
	}
	inUse, err = s.PathInUse(ctx, ClassPhotos, "m/7.dat")
	if err != nil || inUse {
		t.Fatalf("size still in use after last ref: %v %v", inUse, err)
	}
}

func createBlobTables(ctx context.Context, db *sql.DB) error {
	for _, q := range []string{
		`CREATE TABLE media_blobs (
		  id bigint NOT NULL AUTO_INCREMENT,
		  sha256 char(64) NOT NULL,
		  file_size bigint NOT NULL,
		  file_path varchar(255) NOT NULL,
		  storage_class varchar(32) NOT NULL,
		  ref_count int NOT NULL DEFAULT 1,
		  origin_ref_id bigint NOT NULL DEFAULT 0,
		  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		  PRIMARY KEY (id),
		  UNIQUE KEY uk_media_blobs_hash (storage_class, sha256, file_size)
		)`,
		`CREATE TABLE media_blob_refs (
		  id bigint NOT NULL AUTO_INCREMENT,
		  blob_id bigint NOT NULL,
		  ref_kind varchar(16) NOT NULL,
		  ref_id bigint NOT NULL,
		  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  PRIMARY KEY (id),
		  UNIQUE KEY uk_media_blob_refs_ref (ref_kind, ref_id)
		)`,
		`CREATE TABLE media_photo_paths (
		  photo_id bigint NOT NULL,
		  size_type varchar(8) NOT NULL,
		  file_path varchar(255) NOT NULL,
		  PRIMARY KEY (photo_id, size_type)
		)`,
		`CREATE TABLE media_blob_gc (
		  id bigint NOT NULL AUTO_INCREMENT,
		  storage_class varchar(32) NOT NULL,
		  file_path varchar(255) NOT NULL,
		  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  PRIMARY KEY (id),
		  UNIQUE KEY uk_media_blob_gc_path (storage_class, file_path)
		)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TELLOMI_TEST_MYSQL")
	if dsn == "" {
		dsn = "root:@tcp(127.0.0.1:3306)/mysql?parseTime=true"
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	return db
}
