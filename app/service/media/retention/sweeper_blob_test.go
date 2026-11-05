package retention

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/teamgram/teamgram-server/app/service/mediablob"
)

func TestDryRunNeverDeletesReferencedBlob(t *testing.T) {
	db := openBlobDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS media_blob_gc, media_blob_refs, media_photo_paths, media_blobs, documents`); err != nil {
		t.Fatal(err)
	}
	if err := createRetentionBlobTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	blobs := mediablob.New(db)
	const sha = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	if _, err := blobs.Claim(ctx, mediablob.ClassDocuments, mediablob.KindDocument, 11, sha, 40, "11.dat"); err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Claim(ctx, mediablob.ClassDocuments, mediablob.KindDocument, 22, sha, 40, "22.dat"); err != nil {
		t.Fatal(err)
	}
	del := &fakeDeleter{}
	sw := &Sweeper{Store: &SQLStore{DB: db}, Objects: del}
	if err := sw.expire(ctx, Media{Kind: KindDocument, ID: 22, Size: 40}, time.Now(), true, &Report{Expired: map[Kind]int{}}); err != nil {
		t.Fatal(err)
	}
	if len(del.keys) != 0 {
		t.Fatalf("dry-run deleted a shared blob: %v", del.keys)
	}
	var refs int
	if err := db.QueryRowContext(ctx, `SELECT ref_count FROM media_blobs WHERE file_path = '11.dat'`).Scan(&refs); err != nil || refs != 2 {
		t.Fatalf("dry-run changed ref_count to %d (%v)", refs, err)
	}

	if _, err := blobs.Release(ctx, mediablob.KindDocument, 11); err != nil {
		t.Fatal(err)
	}
	del.keys = nil
	if err := sw.expire(ctx, Media{Kind: KindDocument, ID: 22, Size: 40}, time.Now(), true, &Report{Expired: map[Kind]int{}}); err != nil {
		t.Fatal(err)
	}
	if len(del.keys) != 0 {
		t.Fatalf("dry-run deleted the last live ref: %v", del.keys)
	}
	if err := db.QueryRowContext(ctx, `SELECT ref_count FROM media_blobs WHERE file_path = '11.dat'`).Scan(&refs); err != nil || refs != 1 {
		t.Fatalf("last ref ref_count %d err %v", refs, err)
	}
	var queued int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_blob_gc`).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("dry-run queued gc rows %d err %v", queued, err)
	}
}

func TestSweeperKeepsSharedBlobUntilLastRef(t *testing.T) {
	db := openBlobDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS media_blob_gc, media_blob_refs, media_photo_paths, media_blobs, documents`); err != nil {
		t.Fatal(err)
	}
	if err := createRetentionBlobTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE documents (document_id bigint PRIMARY KEY, expired_at bigint NOT NULL DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO documents (document_id) VALUES (11), (22)`); err != nil {
		t.Fatal(err)
	}
	blobs := mediablob.New(db)
	const sha = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	if _, err := blobs.Claim(ctx, mediablob.ClassDocuments, mediablob.KindDocument, 11, sha, 40, "11.dat"); err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Claim(ctx, mediablob.ClassDocuments, mediablob.KindDocument, 22, sha, 40, "22.dat"); err != nil {
		t.Fatal(err)
	}
	del := &fakeDeleter{}
	sw := &Sweeper{Store: &SQLStore{DB: db}, Objects: del}
	dry, err := sw.retentionObjects(ctx, Media{Kind: KindDocument, ID: 22}, true)
	if err != nil || len(dry) != 0 {
		t.Fatalf("dry run of a shared document would delete %+v (%v)", dry, err)
	}
	var refs int
	if err = db.QueryRowContext(ctx, `SELECT ref_count FROM media_blobs WHERE file_path = '11.dat'`).Scan(&refs); err != nil || refs != 2 {
		t.Fatalf("dry run changed ref_count to %d (%v)", refs, err)
	}

	if err = sw.expire(ctx, Media{Kind: KindDocument, ID: 11, Size: 40}, time.Now(), false, &Report{Expired: map[Kind]int{}}); err != nil {
		t.Fatal(err)
	}
	if len(del.keys) != 0 {
		t.Fatalf("deleted while another ref exists: %v", del.keys)
	}
	if err = db.QueryRowContext(ctx, `SELECT ref_count FROM media_blobs WHERE file_path = '11.dat'`).Scan(&refs); err != nil || refs != 1 {
		t.Fatalf("ref_count %d err %v", refs, err)
	}
	inUse, err := blobs.PathInUse(ctx, mediablob.ClassDocuments, "11.dat")
	if err != nil || !inUse {
		t.Fatalf("path in use %v %v", inUse, err)
	}

	sw.Objects = errDeleter{}
	if err = sw.expire(ctx, Media{Kind: KindDocument, ID: 22, Size: 40}, time.Now(), false, &Report{Expired: map[Kind]int{}}); err == nil {
		t.Fatal("delete failure was ignored")
	}
	var blobsLeft, zeros, queued, expired int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_blobs`).Scan(&blobsLeft); err != nil || blobsLeft != 0 {
		t.Fatalf("blob row left %d err %v", blobsLeft, err)
	}
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_blobs WHERE ref_count = 0`).Scan(&zeros); err != nil || zeros != 0 {
		t.Fatalf("ref_count 0 rows %d err %v", zeros, err)
	}
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_blob_gc WHERE file_path = '11.dat'`).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("gc rows %d err %v", queued, err)
	}
	if err = db.QueryRowContext(ctx, `SELECT expired_at FROM documents WHERE document_id = 22`).Scan(&expired); err != nil || expired != 0 {
		t.Fatalf("marked expired after delete failure: %d %v", expired, err)
	}

	again := &fakeDeleter{}
	sw.Objects = again
	if err = sw.drainBlobGC(ctx, &Report{Expired: map[Kind]int{}}); err != nil {
		t.Fatal(err)
	}
	if len(again.keys) != 1 || again.keys[0] != "documents/11.dat" {
		t.Fatalf("retry deleted %v", again.keys)
	}
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_blob_gc`).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("gc left %d err %v", queued, err)
	}
}

type errDeleter struct{}

func (errDeleter) Delete(context.Context, string, string) error { return errors.New("disk") }

func openBlobDB(t *testing.T) *sql.DB {
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

func createRetentionBlobTables(ctx context.Context, db *sql.DB) error {
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
