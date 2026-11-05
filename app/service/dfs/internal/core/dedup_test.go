package core

import (
	"bytes"
	"testing"

	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
	"github.com/teamgram/teamgram-server/app/service/mediablob"
)

func TestStoredPhotoPath(t *testing.T) {
	if got := storedPhotoPath("x", "x/11.dat", 11, cachedTypeSize); got != "x/11.dat" {
		t.Fatalf("stored path %q", got)
	}
	if got := storedPhotoPath("x", "", 11, cachedTypeSize); got != "x/11.dat" {
		t.Fatalf("legacy path %q", got)
	}
	if got := storedPhotoPath("i", "i/11.dat", 11, cachedTypeStrippedSize); got != "" {
		t.Fatalf("stripped path %q", got)
	}
	if got := storedPhotoPath("m", "", 0, cachedTypeSize); got != "" {
		t.Fatalf("missing origin %q", got)
	}
	if got := storedPhotoPath("j", "", 11, cachedTypePathSize); got != "" {
		t.Fatalf("path size %q", got)
	}
}

func TestRecordOriginalPhotoPath(t *testing.T) {
	if !recordOriginalPhotoPath(mediablob.ClassPhotos) {
		t.Fatal("photo original must be recorded on reuse")
	}
	if recordOriginalPhotoPath(mediablob.ClassDocuments) {
		t.Fatal("document object must not be stored as photo size 0")
	}
	if recordOriginalPhotoPath("") {
		t.Fatal("empty class")
	}
}

// The animated GIF upload decodes the received bytes before it stores them.
// image.Decode only accepts GIF when the decoder is registered.
func TestTinyGIFDecodes(t *testing.T) {
	img, err := imaging.Decode(bytes.NewReader(tinyGIF()))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() < 1 || img.Bounds().Dy() < 1 {
		t.Fatalf("bounds %v", img.Bounds())
	}
}

func tinyGIF() []byte {
	return []byte{
		0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
		0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02,
		0x44, 0x01, 0x00, 0x3b,
	}
}
