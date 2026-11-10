package thumbbackfill

import (
	"context"
	"errors"
	"testing"
)

type memDB struct {
	rows []Row
	size map[int64]int32
}

func (m *memDB) Next(_ context.Context, after int64, limit int) ([]Row, error) {
	var out []Row
	for _, row := range m.rows {
		if row.ID <= after {
			continue
		}
		out = append(out, row)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (m *memDB) UpdateFileSize(_ context.Context, id int64, size int32) error {
	if m.size == nil {
		m.size = map[int64]int32{}
	}
	m.size[id] = size
	return nil
}

type memBlobs struct {
	present map[string]bool
	docs    map[string][]byte
	photos  map[string][]byte
	existsN int
}

func (b *memBlobs) Exists(_ context.Context, path string) (bool, error) {
	b.existsN++
	return b.present[path], nil
}

func (b *memBlobs) ReadDocument(_ context.Context, path string) ([]byte, error) {
	body, ok := b.docs[path]
	if !ok {
		return nil, errors.New("missing document")
	}
	return body, nil
}

func (b *memBlobs) PutPhoto(_ context.Context, path string, body []byte) error {
	if b.photos == nil {
		b.photos = map[string][]byte{}
	}
	b.photos[path] = append([]byte(nil), body...)
	return nil
}

func TestDryRunFlagsAndPlansRegen(t *testing.T) {
	db := &memDB{rows: []Row{
		{ID: 1, PhotoSizeID: 10, SizeType: "m", Path: "m/10.dat", Mime: "video/mp4", DocumentPath: "10.dat"},
		{ID: 2, PhotoSizeID: 11, SizeType: "m", Path: "m/11.dat", Mime: "video/mp4"},
		{ID: 3, PhotoSizeID: 12, SizeType: "m", Path: "m/12.dat"},
	}}
	blobs := &memBlobs{present: map[string]bool{"m/12.dat": true}}
	rep, err := Run(context.Background(), db, blobs, Options{
		DryRun: true,
		Batch:  2,
		Frame:  func([]byte) ([]byte, error) { return []byte("jpeg"), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 3 || rep.Present != 1 || rep.Regen != 1 || rep.Flagged != 1 {
		t.Fatalf("%s", rep)
	}
	if len(blobs.photos) != 0 || len(db.size) != 0 {
		t.Fatal("dry run wrote")
	}
	if blobs.existsN != 3 {
		t.Fatalf("exists calls %d", blobs.existsN)
	}
}

func TestApplyWritesMissingThumb(t *testing.T) {
	db := &memDB{rows: []Row{
		{ID: 1, PhotoSizeID: 10, SizeType: "m", Path: "m/10.dat", Mime: "video/mp4", DocumentPath: "10.dat"},
	}}
	blobs := &memBlobs{docs: map[string][]byte{"10.dat": []byte("video")}}
	rep, err := Run(context.Background(), db, blobs, Options{
		DryRun: false,
		Frame:  func(src []byte) ([]byte, error) { return append([]byte("jpeg:"), src...), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Regen != 1 || rep.Flagged != 0 {
		t.Fatalf("%s %v", rep, rep.Flags)
	}
	if string(blobs.photos["m/10.dat"]) != "jpeg:video" {
		t.Fatalf("stored %q", blobs.photos["m/10.dat"])
	}
	if db.size[1] != int32(len("jpeg:video")) {
		t.Fatalf("file_size %d", db.size[1])
	}
}

func TestNoFFmpegFlags(t *testing.T) {
	db := &memDB{rows: []Row{
		{ID: 1, PhotoSizeID: 10, SizeType: "m", Path: "m/10.dat", Mime: "video/mp4", DocumentPath: "10.dat"},
	}}
	rep, err := Run(context.Background(), db, &memBlobs{docs: map[string][]byte{"10.dat": []byte("v")}}, Options{DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Flagged != 1 || rep.Regen != 0 {
		t.Fatalf("%s %v", rep, rep.Flags)
	}
}
