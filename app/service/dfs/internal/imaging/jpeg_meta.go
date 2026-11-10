package imaging

import "bytes"

// jpeg privacy markers. Official clients receive photos with EXIF, XMP and
// IPTC removed. GPS, camera make and timestamps live in those segments.
const (
	jpegSOI = 0xD8
	jpegEOI = 0xD9
	jpegSOS = 0xDA
	jpegCOM = 0xFE
)

// stripJPEGMetadata drops EXIF, XMP, IPTC and comments without re-encoding.
// A harmless ICC profile (APP2) and the Adobe color segment (APP14) stay.
// ok is false when the file is not a JPEG this walker understands; the
// caller then re-encodes at the photo quality instead.
func stripJPEGMetadata(src []byte) (out []byte, ok bool) {
	if len(src) < 4 || src[0] != 0xFF || src[1] != jpegSOI {
		return nil, false
	}
	dst := make([]byte, 0, len(src))
	dst = append(dst, 0xFF, jpegSOI)
	rest, done, ok := copyJPEG(src[2:], &dst)
	if !ok || !done || len(rest) != 0 {
		return nil, false
	}
	if jpegHasPrivacy(dst) {
		return nil, false
	}
	return dst, true
}

// jpegOrientation is the EXIF orientation (1–8), or 0 when the file has none.
func jpegOrientation(src []byte) int {
	if len(src) < 4 || src[0] != 0xFF || src[1] != jpegSOI {
		return 0
	}
	orient := 0
	_ = walkJPEGSegments(src[2:], func(marker byte, payload []byte) bool {
		if marker == 0xE1 {
			if n := exifOrientation(payload); n >= 1 && n <= 8 {
				orient = n
			}
		}
		return marker != jpegSOS
	})
	return orient
}

// jpegHasPrivacy reports EXIF, XMP, IPTC, comments, or a GPS-bearing ICC.
func jpegHasPrivacy(src []byte) bool {
	if len(src) < 4 || src[0] != 0xFF || src[1] != jpegSOI {
		return true
	}
	if bytes.Contains(src, []byte("Exif\x00\x00")) ||
		bytes.Contains(src, []byte("http://ns.adobe.com/xmp/")) ||
		bytes.Contains(src, []byte("Photoshop 3.0")) ||
		bytes.Contains(src, []byte("GPSLatitude")) ||
		bytes.Contains(src, []byte("GPSLongitude")) {
		return true
	}
	found := false
	_ = walkJPEGSegments(src[2:], func(marker byte, payload []byte) bool {
		switch marker {
		case 0xE1, jpegCOM, 0xED: // APP1, COM, APP13
			found = true
			return false
		case 0xE2:
			if len(payload) > 0 && !harmlessICC(payload) {
				found = true
				return false
			}
		}
		return marker != jpegSOS
	})
	return found
}

func walkJPEGSegments(src []byte, fn func(marker byte, payload []byte) bool) bool {
	_, _, ok := copyJPEG(src, nil)
	_ = fn
	return ok && scanSegments(src, fn)
}

func scanSegments(src []byte, fn func(marker byte, payload []byte) bool) bool {
	i := 0
	for i < len(src) {
		if src[i] != 0xFF {
			return false
		}
		j := i + 1
		for j < len(src) && src[j] == 0xFF {
			j++
		}
		if j >= len(src) {
			return false
		}
		marker := src[j]
		if marker == jpegEOI || marker == 0x00 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 {
			return true
		}
		if j+3 > len(src) {
			return false
		}
		seglen := int(src[j+1])<<8 | int(src[j+2])
		if seglen < 2 || j+1+seglen > len(src) {
			return false
		}
		payload := src[j+3 : j+1+seglen]
		if !fn(marker, payload) {
			return true
		}
		if marker == jpegSOS {
			return true
		}
		i = j + 1 + seglen
	}
	return false
}

// copyJPEG copies src onto dst, dropping privacy segments, through EOI.
// A nil dst only validates. done is true when EOI was consumed.
func copyJPEG(src []byte, dst *[]byte) (rest []byte, done, ok bool) {
	i := 0
	for i < len(src) {
		if src[i] != 0xFF {
			return nil, false, false
		}
		j := i + 1
		for j < len(src) && src[j] == 0xFF {
			j++
		}
		if j >= len(src) {
			return nil, false, false
		}
		marker := src[j]
		if marker == 0x00 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			// Standalone markers are only legal inside a scan. Seeing one
			// before SOS means this walker does not understand the file.
			return nil, false, false
		}
		if marker == jpegEOI {
			if dst != nil {
				*dst = append(*dst, 0xFF, jpegEOI)
			}
			return src[j+1:], true, true
		}
		if j+3 > len(src) {
			return nil, false, false
		}
		seglen := int(src[j+1])<<8 | int(src[j+2])
		if seglen < 2 || j+1+seglen > len(src) {
			return nil, false, false
		}
		payload := src[j+3 : j+1+seglen]
		segEnd := j + 1 + seglen
		keep := marker == jpegSOS || keepJPEGMarker(marker, payload)
		if keep && dst != nil {
			*dst = append(*dst, 0xFF, marker)
			*dst = append(*dst, src[j+1:segEnd]...)
		}
		if marker == jpegSOS {
			return copyJPEGScan(src[segEnd:], dst)
		}
		i = segEnd
	}
	return nil, false, false
}

func copyJPEGScan(src []byte, dst *[]byte) (rest []byte, done, ok bool) {
	i := 0
	for i < len(src) {
		if src[i] != 0xFF {
			if dst != nil {
				*dst = append(*dst, src[i])
			}
			i++
			continue
		}
		j := i + 1
		for j < len(src) && src[j] == 0xFF {
			j++
		}
		if j >= len(src) {
			return nil, false, false
		}
		marker := src[j]
		if marker == 0x00 {
			if dst != nil {
				*dst = append(*dst, 0xFF, 0x00)
			}
			i = j + 1
			continue
		}
		if marker >= 0xD0 && marker <= 0xD7 {
			if dst != nil {
				*dst = append(*dst, 0xFF, marker)
			}
			i = j + 1
			continue
		}
		if marker == jpegEOI {
			if dst != nil {
				*dst = append(*dst, 0xFF, jpegEOI)
			}
			return src[j+1:], true, true
		}
		if j+3 > len(src) {
			return nil, false, false
		}
		seglen := int(src[j+1])<<8 | int(src[j+2])
		if seglen < 2 || j+1+seglen > len(src) {
			return nil, false, false
		}
		payload := src[j+3 : j+1+seglen]
		segEnd := j + 1 + seglen
		keep := marker == jpegSOS || keepJPEGMarker(marker, payload)
		if keep && dst != nil {
			*dst = append(*dst, 0xFF, marker)
			*dst = append(*dst, src[j+1:segEnd]...)
		}
		if marker == jpegSOS {
			i = segEnd
			continue
		}
		i = segEnd
	}
	return nil, false, false
}

func keepJPEGMarker(marker byte, payload []byte) bool {
	switch marker {
	case 0xE0: // APP0. A JFIF header has no location. A JFXX thumbnail can.
		return bytes.HasPrefix(payload, []byte("JFIF\x00"))
	case 0xE2: // APP2
		return harmlessICC(payload)
	case 0xEE: // APP14 Adobe color transform. Not a privacy segment.
		return bytes.HasPrefix(payload, []byte("Adobe"))
	case 0xE1, 0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xEB, 0xEC, 0xED, 0xEF, jpegCOM:
		return false
	default:
		return true
	}
}

// harmlessICC keeps a normal color profile and drops one that carries
// EXIF, XMP or GPS text. The profile is not re-encoded.
func harmlessICC(payload []byte) bool {
	const magic = "ICC_PROFILE\x00"
	if !bytes.HasPrefix(payload, []byte(magic)) || len(payload) < len(magic)+2+128 {
		return false
	}
	data := payload[len(magic)+2:]
	if len(data) < 128 || !bytes.Equal(data[36:40], []byte("acsp")) {
		return false
	}
	for _, needle := range [][]byte{
		[]byte("GPSLatitude"),
		[]byte("GPSLongitude"),
		[]byte("GPSAltitude"),
		[]byte("GPSInfo"),
		[]byte("Exif"),
		[]byte("http://ns.adobe.com/xmp/"),
		[]byte("Photoshop 3.0"),
		[]byte("<rdf"),
	} {
		if bytes.Contains(data, needle) {
			return false
		}
	}
	return true
}

func exifOrientation(payload []byte) int {
	const prefix = "Exif\x00\x00"
	if !bytes.HasPrefix(payload, []byte(prefix)) || len(payload) < len(prefix)+8 {
		return 0
	}
	tiff := payload[len(prefix):]
	if len(tiff) < 8 {
		return 0
	}
	le := false
	switch {
	case tiff[0] == 'I' && tiff[1] == 'I':
		le = true
	case tiff[0] == 'M' && tiff[1] == 'M':
		le = false
	default:
		return 0
	}
	u16 := func(b []byte) uint16 {
		if le {
			return uint16(b[0]) | uint16(b[1])<<8
		}
		return uint16(b[0])<<8 | uint16(b[1])
	}
	u32 := func(b []byte) uint32 {
		if le {
			return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
		}
		return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	}
	if u16(tiff[2:4]) != 42 {
		return 0
	}
	ifd := int(u32(tiff[4:8]))
	if ifd < 0 || ifd+2 > len(tiff) {
		return 0
	}
	n := int(u16(tiff[ifd : ifd+2]))
	if n < 0 || ifd+2+n*12 > len(tiff) {
		return 0
	}
	for e := 0; e < n; e++ {
		ent := tiff[ifd+2+e*12 : ifd+2+(e+1)*12]
		if u16(ent[0:2]) != 0x0112 || u16(ent[2:4]) != 3 || u32(ent[4:8]) != 1 {
			continue
		}
		return int(u16(ent[8:10]))
	}
	return 0
}
