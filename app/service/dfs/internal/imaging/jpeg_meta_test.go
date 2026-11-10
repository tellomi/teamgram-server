package imaging

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestStripDropsGPSAndKeepsHarmlessICC(t *testing.T) {
	base := solid(t, 40, 20, staticJPEG)
	raw := insertAfterSOI(base,
		exifApp1(1),
		xmpApp1(),
		iptcApp13(),
		jpegSegment(0xFE, []byte("GPS timestamp 2020-01-01 camera")),
		iccApp2(),
	)
	if !jpegHasPrivacy(raw) || jpegOrientation(raw) != 1 {
		t.Fatalf("fixture privacy=%v orientation=%d", jpegHasPrivacy(raw), jpegOrientation(raw))
	}
	stripped, ok := stripJPEGMetadata(raw)
	if !ok {
		t.Fatal("strip failed")
	}
	if jpegHasPrivacy(stripped) {
		t.Fatal("stripped jpeg still has privacy metadata")
	}
	if !bytes.Contains(stripped, []byte("ICC_PROFILE")) {
		t.Fatal("harmless ICC profile was removed")
	}
	img, err := jpeg.Decode(bytes.NewReader(stripped))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 40 || img.Bounds().Dy() != 20 {
		t.Fatalf("stripped size %v", img.Bounds())
	}
	if len(stripped) > len(raw) {
		t.Fatalf("strip grew the file %d -> %d", len(raw), len(stripped))
	}
}

func TestStoredSizesHaveNoGPSAndOrientationIsApplied(t *testing.T) {
	// Wide image, orientation 6 is 90° clockwise, so the stored photo is tall.
	// Left half is red and right half is blue; after the rotation the top is
	// red and the bottom is blue. GPS, XMP, IPTC and the orientation tag must
	// not survive in any size.
	const w, h = 200, 80
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBA{R: 220, A: 255}
			if x >= w/2 {
				c = color.NRGBA{B: 220, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	raw := insertAfterSOI(buf.Bytes(), exifApp1(6), xmpApp1(), iptcApp13(), iccApp2())
	if jpegOrientation(raw) != 6 || !bytes.Contains(raw, []byte("GPSLatitude")) {
		t.Fatal("fixture did not carry orientation and GPS")
	}

	var sizes []derivedSize
	err := ReSizeImage(raw, ".jpg", false, func(sz string, _ int, dw, dh int32, b []byte) error {
		sizes = append(sizes, derivedSize{szType: sz, w: dw, h: dh, b: b})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sizes) < 2 {
		t.Fatalf("sizes %v", typesOf(sizes))
	}
	for _, sz := range sizes {
		if jpegHasPrivacy(sz.b) || jpegOrientation(sz.b) != 0 {
			t.Fatalf("size %s still has metadata orientation=%d", sz.szType, jpegOrientation(sz.b))
		}
		decoded, err := jpeg.Decode(bytes.NewReader(sz.b))
		if err != nil {
			t.Fatalf("size %s: %v", sz.szType, err)
		}
		if decoded.Bounds().Dx() != int(sz.w) || decoded.Bounds().Dy() != int(sz.h) {
			t.Fatalf("size %s decoded %v reported %dx%d", sz.szType, decoded.Bounds(), sz.w, sz.h)
		}
		// Oriented frame is taller than it is wide. The unrotated file is wider.
		if sz.w >= sz.h {
			t.Fatalf("size %s is %dx%d, orientation was not applied", sz.szType, sz.w, sz.h)
		}
	}
	full := sizes[len(sizes)-1]
	if full.w != h || full.h != w {
		t.Fatalf("full size %s is %dx%d, want %dx%d", full.szType, full.w, full.h, h, w)
	}
	top := sample(full.b, full.w/2, full.h/4)
	bottom := sample(full.b, full.w/2, full.h*3/4)
	if top.R < 150 || top.B > 80 {
		t.Fatalf("top pixel %+v, want the rotated red half", top)
	}
	if bottom.B < 150 || bottom.R > 80 {
		t.Fatalf("bottom pixel %+v, want the rotated blue half", bottom)
	}
	// The full-size step is a segment strip only when orientation is identity.
	// Orientation 6 re-encodes, so the ICC profile is not required there.
	// A separate identity upload keeps a harmless ICC on that step.
	plain := insertAfterSOI(buf.Bytes(), exifApp1(1), xmpApp1(), iccApp2())
	got := resizeAll(t, plain, ".jpg", false)
	largest := got[len(got)-1]
	if jpegHasPrivacy(largest.b) {
		t.Fatal("identity full size still has privacy metadata")
	}
	if !bytes.Contains(largest.b, []byte("ICC_PROFILE")) {
		t.Fatal("identity full size dropped a harmless ICC profile")
	}
	if largest.w != w || largest.h != h {
		t.Fatalf("identity full size %dx%d", largest.w, largest.h)
	}
}

func sample(jpegBytes []byte, x, y int32) color.NRGBA {
	img, err := jpeg.Decode(bytes.NewReader(jpegBytes))
	if err != nil {
		return color.NRGBA{}
	}
	r, g, b, a := img.At(int(x), int(y)).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func insertAfterSOI(src []byte, segs ...[]byte) []byte {
	out := append([]byte{}, src[:2]...)
	for _, seg := range segs {
		out = append(out, seg...)
	}
	return append(out, src[2:]...)
}

func jpegSegment(marker byte, payload []byte) []byte {
	n := len(payload) + 2
	seg := []byte{0xFF, marker, byte(n >> 8), byte(n)}
	return append(seg, payload...)
}

func exifApp1(orientation int) []byte {
	tiff := []byte{'I', 'I', 0x2a, 0x00, 8, 0, 0, 0}
	tiff = append(tiff, 2, 0)
	tiff = append(tiff,
		0x12, 0x01, 0x03, 0x00,
		0x01, 0x00, 0x00, 0x00,
		byte(orientation), 0x00, 0x00, 0x00,
	)
	const gpsAt = 38
	tiff = append(tiff,
		0x25, 0x88, 0x04, 0x00,
		0x01, 0x00, 0x00, 0x00,
		gpsAt, 0, 0, 0,
	)
	tiff = append(tiff, 0, 0, 0, 0)
	tiff = append(tiff, 1, 0)
	const dataAt = 56
	tiff = append(tiff,
		0x02, 0x00, 0x05, 0x00,
		0x03, 0x00, 0x00, 0x00,
		dataAt, 0, 0, 0,
	)
	tiff = append(tiff, 0, 0, 0, 0)
	var rats [24]byte
	for i, n := range []uint32{37, 30, 0} {
		binary.LittleEndian.PutUint32(rats[i*8:i*8+4], n)
		binary.LittleEndian.PutUint32(rats[i*8+4:i*8+8], 1)
	}
	tiff = append(tiff, rats[:]...)
	tiff = append(tiff, []byte("GPSLatitude")...)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	return jpegSegment(0xE1, payload)
}

func xmpApp1() []byte {
	xml := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description exif:GPSLatitude="37,30.0N"/></rdf:RDF></x:xmpmeta>`
	payload := append([]byte("http://ns.adobe.com/xmp/1.0/\x00"), xml...)
	return jpegSegment(0xE1, payload)
}

func iptcApp13() []byte {
	body := append([]byte("Photoshop 3.0\x00"), []byte("8BIM GPSLatitude")...)
	return jpegSegment(0xED, body)
}

func iccApp2() []byte {
	profile := make([]byte, 128)
	binary.BigEndian.PutUint32(profile[0:4], 128)
	copy(profile[36:40], "acsp")
	payload := append([]byte("ICC_PROFILE\x00"), 1, 1)
	payload = append(payload, profile...)
	return jpegSegment(0xE2, payload)
}
