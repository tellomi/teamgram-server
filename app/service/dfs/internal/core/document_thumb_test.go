package core

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
)

func TestReadFirstKeepsStorageError(t *testing.T) {
	timeout := errors.New("r2 timeout")
	got, err := readFirst(
		func() ([]byte, error) { return nil, timeout },
		func() ([]byte, error) { return nil, minio_util.ErrNotFound },
	)
	if err != timeout || got != nil {
		t.Fatalf("masked storage error: %q %v", got, err)
	}

	got, err = readFirst(
		func() ([]byte, error) { return nil, minio_util.ErrNotFound },
		func() ([]byte, error) { return []byte("jpeg"), nil },
	)
	if err != nil || string(got) != "jpeg" {
		t.Fatalf("fallback %q %v", got, err)
	}
}

func TestDocumentThumbPathsIncludeOrigin(t *testing.T) {
	paths := thumbPathList("m", 200, "m/200.dat", []string{"photos/m/200.dat"}, 100)
	want := []string{"m/200.dat", "photos/m/200.dat", "m/100.dat"}
	if len(paths) != len(want) {
		t.Fatalf("paths %v", paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths %v", paths)
		}
	}
	if id := originIDFromDocumentPath("100.dat", 200); id != 100 {
		t.Fatalf("origin %d", id)
	}
	if id := originIDFromDocumentPath("200.dat", 200); id != 0 {
		t.Fatalf("self origin %d", id)
	}
	if id := originIDFromDocumentPath("ab/not-an-id.dat", 200); id != 0 {
		t.Fatalf("non-numeric origin %d", id)
	}
}

func TestDocumentThumbReadsOriginInsteadOfFileIDInvalid(t *testing.T) {
	jpeg := tinyJPEG(t)
	store := map[string][]byte{
		"m/100.dat": jpeg,
	}
	paths := thumbPathList(mtproto.PhotoSZMediumType, 200, "m/200.dat", nil, 100)
	got, err := readThumbCandidates(0, func() ([]byte, error) {
		if body, ok := store[paths[0]]; ok {
			return body, nil
		}
		return nil, minio_util.ErrNotFound
	}, func() ([]byte, error) {
		if body, ok := store[paths[1]]; ok {
			return body, nil
		}
		return nil, minio_util.ErrNotFound
	})
	if err != nil || !bytes.Equal(got, jpeg) {
		t.Fatalf("origin thumb %d bytes, err %v", len(got), err)
	}
	out, err := resolveThumbDownload(0, 1024, got, err, func() ([]byte, error) {
		t.Fatal("rebuilt a thumb that was already stored")
		return nil, nil
	})
	if err != nil || !bytes.Equal(out, jpeg) {
		t.Fatalf("download %d %v", len(out), err)
	}
}

func TestMissingStillThumbRebuildsJPEG(t *testing.T) {
	jpeg := tinyJPEG(t)
	got, err := resolveThumbDownload(0, 1024, nil, minio_util.ErrNotFound, func() ([]byte, error) {
		body, ok := thumbFromDocumentBytes(jpeg)
		if !ok {
			return nil, errThumbPermanent
		}
		return body, nil
	})
	if err != nil || stillKind(got) != "jpeg" || len(got) == 0 {
		t.Fatalf("rebuilt %d %v", len(got), err)
	}
}

func TestMissingNonImageStaysFileIDInvalid(t *testing.T) {
	_, err := resolveThumbDownload(0, 32, nil, minio_util.ErrNotFound, func() ([]byte, error) {
		if _, ok := thumbFromDocumentBytes([]byte("%PDF-1.7 not a photo")); ok {
			t.Fatal("pdf became a thumb")
		}
		return nil, errThumbPermanent
	})
	if err != mtproto.ErrFileIdInvalid {
		t.Fatalf("err %v, want FILE_ID_INVALID", err)
	}
}

func TestThumbStorageErrorIsNotFileIDInvalid(t *testing.T) {
	timeout := errors.New("dial tcp 172.19.44.222:443: i/o timeout")
	got, err := readThumbCandidates(0,
		func() ([]byte, error) { return nil, timeout },
		func() ([]byte, error) { return nil, minio_util.ErrNotFound },
	)
	if err != timeout || got != nil {
		t.Fatalf("candidates %q %v", got, err)
	}
	out, err := resolveThumbDownload(0, 128*1024, got, err, func() ([]byte, error) {
		t.Fatal("rebuilt after a storage error")
		return nil, nil
	})
	if err != timeout || out != nil {
		t.Fatalf("got %q %v, want the storage error", out, err)
	}
	if err == mtproto.ErrFileIdInvalid {
		t.Fatal("storage error became FILE_ID_INVALID")
	}
}

func TestThumbFromDocumentBytes(t *testing.T) {
	jpeg := tinyJPEG(t)
	got, ok := thumbFromDocumentBytes(jpeg)
	if !ok || stillKind(got) != "jpeg" {
		t.Fatal("jpeg document did not become a thumb")
	}
	if _, ok = thumbFromDocumentBytes([]byte("voice-note")); ok {
		t.Fatal("voice note became a thumb")
	}
}

func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	img.Set(0, 0, color.RGBA{R: 200, G: 10, B: 10, A: 255})
	var buf bytes.Buffer
	if err := imaging.EncodeJpeg(&buf, img); err != nil {
		t.Fatal(err)
	}
	if stillKind(buf.Bytes()) != "jpeg" {
		t.Fatal("fixture is not a jpeg")
	}
	return buf.Bytes()
}
