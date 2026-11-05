package imaging

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/chai2010/webp"
	"github.com/davidbyttow/govips/v2/vips"
)

func TestStaticFormatRouting(t *testing.T) {
	cases := []struct {
		ext  string
		ok   bool
		want staticFormat
	}{
		{".jpg", true, staticJPEG},
		{".JPEG", true, staticJPEG},
		{".jpeg", true, staticJPEG},
		{".png", true, staticPNG},
		{".webp", true, staticWEBP},
		{".WEBP", true, staticWEBP},
		{".gif", false, 0},
		{".tif", false, 0},
		{".tiff", false, 0},
		{".bmp", false, 0},
		{".mp4", false, 0},
		{"", false, 0},
	}
	for _, c := range cases {
		got, ok := staticFormatOf(c.ext)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%s: got %v %v", c.ext, got, ok)
		}
	}
}

func TestPhotoLadderUsesKnownSizeTypes(t *testing.T) {
	got := resizeAll(t, solid(t, 1000, 400, staticJPEG), ".jpg", false)
	want := []derivedSize{
		{szType: "s", w: 90, h: 36},
		{szType: "m", w: 320, h: 128},
		{szType: "x", w: 800, h: 320},
		{szType: "y", w: 1000, h: 400},
	}
	assertLadder(t, got, want, staticJPEG, clientPhotoTypes)
}

func TestPNGPortraitLadder(t *testing.T) {
	got := resizeAll(t, solid(t, 200, 400, staticPNG), ".png", false)
	want := []derivedSize{
		{szType: "s", w: 45, h: 90},
		{szType: "m", w: 160, h: 320},
		{szType: "x", w: 200, h: 400},
	}
	assertLadder(t, got, want, staticPNG, clientPhotoTypes)
}

func TestWebPSquareLadder(t *testing.T) {
	got := resizeAll(t, solid(t, 500, 500, staticWEBP), ".webp", false)
	want := []derivedSize{
		{szType: "s", w: 90, h: 90},
		{szType: "m", w: 320, h: 320},
		{szType: "x", w: 500, h: 500},
	}
	assertLadder(t, got, want, staticWEBP, clientPhotoTypes)
}

func TestProfileLadderStaysSquare(t *testing.T) {
	// Both sides at least 1280 and not square: cover to 1280, then a/b/c/d.
	got := resizeAll(t, solid(t, 2000, 1500, staticJPEG), ".jpg", true)
	want := []derivedSize{
		{szType: "a", w: 160, h: 160},
		{szType: "b", w: 320, h: 320},
		{szType: "c", w: 640, h: 640},
		{szType: "d", w: 1280, h: 1280},
	}
	assertLadder(t, got, want, staticJPEG, clientABCTypes)

	// One side below 1280: the historical ladder covers to 640 and stops at c.
	wide := resizeAll(t, solid(t, 2000, 1000, staticJPEG), ".jpg", true)
	wantWide := []derivedSize{
		{szType: "a", w: 160, h: 160},
		{szType: "b", w: 320, h: 320},
		{szType: "c", w: 640, h: 640},
	}
	assertLadder(t, wide, wantWide, staticJPEG, clientABCTypes)

	small := resizeAll(t, solid(t, 100, 80, staticPNG), ".png", true)
	assertLadder(t, small, wantWide, staticPNG, clientABCTypes)
}

func TestGovipsDecodeFailureWritesNothing(t *testing.T) {
	calls := 0
	err := ReSizeImage([]byte("not a jpeg"), ".jpg", false, func(string, int, int32, int32, []byte) error {
		calls++
		return nil
	})
	if err == nil || calls != 0 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestGovipsExportFailureWritesNothing(t *testing.T) {
	orig := staticEncode
	t.Cleanup(func() { staticEncode = orig })
	n := 0
	staticEncode = func(img *vips.ImageRef, format staticFormat) ([]byte, error) {
		n++
		if n >= 2 {
			return nil, errors.New("export failed")
		}
		return orig(img, format)
	}
	calls := 0
	err := ReSizeImage(solid(t, 1000, 400, staticJPEG), ".jpg", false, func(string, int, int32, int32, []byte) error {
		calls++
		return nil
	})
	if err == nil || calls != 0 {
		t.Fatalf("partial publish err=%v calls=%d encodes=%d", err, calls, n)
	}
}

func TestCallbackErrorStopsTheLadder(t *testing.T) {
	calls := 0
	err := ReSizeImage(solid(t, 1000, 400, staticJPEG), ".jpg", false, func(string, int, int32, int32, []byte) error {
		calls++
		return errors.New("store failed")
	})
	if err == nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestGIFStaysOffGovips(t *testing.T) {
	orig := staticEncode
	t.Cleanup(func() { staticEncode = orig })
	staticEncode = func(*vips.ImageRef, staticFormat) ([]byte, error) {
		return nil, errors.New("govips should not encode gif or tiff")
	}

	var gifTypes []string
	err := ReSizeImage(tinyGIF(), ".gif", false, func(sz string, _ int, w, h int32, b []byte) error {
		gifTypes = append(gifTypes, sz)
		if w != 1 || h != 1 || !bytes.HasPrefix(b, []byte("GIF8")) {
			t.Fatalf("gif derivative %s %dx%d", sz, w, h)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(gifTypes) != 1 || gifTypes[0] != "s" {
		t.Fatalf("gif types %v", gifTypes)
	}
}

var (
	clientPhotoTypes = map[string]bool{"s": true, "m": true, "x": true, "y": true, "w": true}
	clientABCTypes   = map[string]bool{"a": true, "b": true, "c": true, "d": true}
)

func resizeAll(t *testing.T, raw []byte, ext string, abc bool) []derivedSize {
	t.Helper()
	var out []derivedSize
	err := ReSizeImage(raw, ext, abc, func(sz string, localID int, w, h int32, b []byte) error {
		out = append(out, derivedSize{szType: sz, localId: localID, w: w, h: h, b: b})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertLadder(t *testing.T, got, want []derivedSize, format staticFormat, allowed map[string]bool) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("sizes %d want %d (%v)", len(got), len(want), typesOf(got))
	}
	for i := range want {
		g := got[i]
		if g.szType != want[i].szType || g.w != want[i].w || g.h != want[i].h {
			t.Fatalf("size %d got %s %dx%d want %s %dx%d", i, g.szType, g.w, g.h, want[i].szType, want[i].w, want[i].h)
		}
		if !allowed[g.szType] || g.szType == "i" || g.szType == "j" {
			t.Fatalf("client does not know size type %q", g.szType)
		}
		assertDecodable(t, format, g.b, g.w, g.h)
	}
}

func typesOf(s []derivedSize) []string {
	out := make([]string, len(s))
	for i := range s {
		out[i] = s[i].szType
	}
	return out
}

func assertDecodable(t *testing.T, format staticFormat, b []byte, w, h int32) {
	t.Helper()
	var (
		img image.Image
		err error
	)
	switch format {
	case staticJPEG:
		if len(b) < 2 || b[0] != 0xff || b[1] != 0xd8 {
			t.Fatalf("not a jpeg (%d bytes)", len(b))
		}
		img, err = jpeg.Decode(bytes.NewReader(b))
	case staticPNG:
		img, err = png.Decode(bytes.NewReader(b))
	case staticWEBP:
		if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
			t.Fatalf("not a webp")
		}
		img, err = webp.Decode(bytes.NewReader(b))
	default:
		t.Fatalf("format %d", format)
	}
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != int(w) || img.Bounds().Dy() != int(h) {
		t.Fatalf("decoded %v, reported %dx%d", img.Bounds(), w, h)
	}
}

func solid(t *testing.T, w, h int, format staticFormat) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if format == staticPNG && x < 2 {
				a = 0
			}
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 17), G: uint8(y * 13), B: 80, A: a})
		}
	}
	var buf bytes.Buffer
	var err error
	switch format {
	case staticJPEG:
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	case staticPNG:
		err = png.Encode(&buf, img)
	case staticWEBP:
		err = webp.Encode(&buf, img, &webp.Options{Lossless: true})
	default:
		t.Fatalf("format %d", format)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tinyGIF() []byte {
	return []byte{
		0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
		0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02,
		0x44, 0x01, 0x00, 0x3b,
	}
}
