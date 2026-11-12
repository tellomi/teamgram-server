package core

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestDocumentThumbsMatchEXIFImageSize(t *testing.T) {
	// A wide file. Orientations 6 and 8 turn it portrait; 3 turns it 180°
	// and leaves the aspect. Clients send documentAttributeImageSize as the
	// display size, and iOS swaps the bubble when the thumb is the inverse.
	const rawW, rawH = 160, 48
	for _, orientation := range []int{6, 8, 3} {
		raw := orientedJPEG(t, rawW, rawH, orientation)
		stored := append([]byte(nil), raw...)
		dispW, dispH := exifDisplaySize(rawW, rawH, orientation)
		attr := mtproto.MakeTLDocumentAttributeImageSize(&mtproto.DocumentAttribute{
			W: int32(dispW),
			H: int32(dispH),
		}).To_DocumentAttribute()

		stripped, medium, decErr, buildErr := documentFileThumbs(raw)
		if decErr != nil || buildErr != nil {
			t.Fatalf("orientation %d: decode %v build %v", orientation, decErr, buildErr)
		}
		if !bytes.Equal(raw, stored) {
			t.Fatalf("orientation %d: thumb build rewrote the file bytes", orientation)
		}
		if len(stripped) < 3 || stripped[0] != 0x01 {
			t.Fatalf("orientation %d: stripped thumb len %d", orientation, len(stripped))
		}
		// EncodeStripped stores height then width in the inline thumb header.
		strippedW, strippedH := int32(stripped[2]), int32(stripped[1])
		if !aspectAgrees(attr.GetW(), attr.GetH(), strippedW, strippedH) {
			t.Fatalf("orientation %d: stripped %dx%d disagrees with image size %dx%d",
				orientation, strippedW, strippedH, attr.GetW(), attr.GetH())
		}
		if !aspectAgrees(attr.GetW(), attr.GetH(), int32(medium.Bounds().Dx()), int32(medium.Bounds().Dy())) {
			t.Fatalf("orientation %d: medium %dx%d disagrees with image size %dx%d",
				orientation, medium.Bounds().Dx(), medium.Bounds().Dy(), attr.GetW(), attr.GetH())
		}
	}
}

func aspectAgrees(attrW, attrH, thumbW, thumbH int32) bool {
	if attrW < 1 || attrH < 1 || thumbW < 1 || thumbH < 1 || attrW == attrH || thumbW == thumbH {
		return false
	}
	return (attrW > attrH) == (thumbW > thumbH)
}

func exifDisplaySize(w, h, orientation int) (int, int) {
	switch orientation {
	case 5, 6, 7, 8:
		return h, w
	default:
		return w, h
	}
}

func orientedJPEG(t *testing.T, w, h, orientation int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBA{R: 210, A: 255}
			if x >= w/2 {
				c = color.NRGBA{B: 210, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	if len(raw) < 2 || raw[0] != 0xFF || raw[1] != 0xD8 {
		t.Fatal("encoder did not write a JPEG")
	}
	out := append([]byte{}, raw[:2]...)
	out = append(out, exifOrientationAPP1(orientation)...)
	out = append(out, raw[2:]...)
	return out
}

func exifOrientationAPP1(orientation int) []byte {
	tiff := []byte{'I', 'I', 0x2a, 0x00, 8, 0, 0, 0}
	tiff = append(tiff, 1, 0)
	tiff = append(tiff,
		0x12, 0x01, 0x03, 0x00,
		0x01, 0x00, 0x00, 0x00,
		byte(orientation), 0x00, 0x00, 0x00,
	)
	tiff = append(tiff, 0, 0, 0, 0)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := make([]byte, 4+len(payload))
	seg[0] = 0xFF
	seg[1] = 0xE1
	binary.BigEndian.PutUint16(seg[2:4], uint16(len(payload)+2))
	copy(seg[4:], payload)
	return seg
}
