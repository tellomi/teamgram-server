package catalog

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	maxVideoStickerBytes   = 256 * 1024
	maxStaticStickerBytes  = 1 << 20
	maxVideoStickerSeconds = 3.0
	stickerSide            = 512
)

// validateStickerFile checks a prepared sticker. static is a 512×512 WebP.
// video is a 512×512 VP9 WebM, at most 3 seconds and 256KB.
func validateStickerFile(video bool, body []byte) error {
	if len(body) == 0 {
		return fmt.Errorf("empty file")
	}
	if video {
		if len(body) > maxVideoStickerBytes {
			return fmt.Errorf("video sticker is %d bytes, limit is %d", len(body), maxVideoStickerBytes)
		}
		return validateWebM(body)
	}
	if len(body) > maxStaticStickerBytes {
		return fmt.Errorf("static sticker is %d bytes, limit is %d", len(body), maxStaticStickerBytes)
	}
	w, h, err := webpSize(body)
	if err != nil {
		return err
	}
	if w != stickerSide || h != stickerSide {
		return fmt.Errorf("webp is %dx%d, want %dx%d", w, h, stickerSide, stickerSide)
	}
	return nil
}

func webpSize(b []byte) (int, int, error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return 0, 0, fmt.Errorf("not a webp")
	}
	off := 12
	for off+8 <= len(b) {
		tag := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		off += 8
		if size < 0 || off+size > len(b) {
			return 0, 0, fmt.Errorf("webp chunk %s overruns the file", tag)
		}
		payload := b[off : off+size]
		switch tag {
		case "VP8X":
			if len(payload) < 10 {
				return 0, 0, fmt.Errorf("webp VP8X chunk is short")
			}
			w := 1 + int(payload[4]) + int(payload[5])<<8 + int(payload[6])<<16
			h := 1 + int(payload[7]) + int(payload[8])<<8 + int(payload[9])<<16
			return w, h, nil
		case "VP8 ":
			w, h, err := vp8Size(payload)
			if err != nil {
				return 0, 0, err
			}
			return w, h, nil
		case "VP8L":
			w, h, err := vp8lSize(payload)
			if err != nil {
				return 0, 0, err
			}
			return w, h, nil
		}
		off += size
		if size%2 == 1 {
			off++
		}
	}
	return 0, 0, fmt.Errorf("webp has no dimension chunk")
}

func vp8Size(payload []byte) (int, int, error) {
	if len(payload) < 10 {
		return 0, 0, fmt.Errorf("webp VP8 frame is short")
	}
	// Keyframe bit is the least significant bit of the frame tag. 0 means keyframe.
	if payload[0]&1 != 0 {
		return 0, 0, fmt.Errorf("webp VP8 frame is not a keyframe")
	}
	if payload[3] != 0x9d || payload[4] != 0x01 || payload[5] != 0x2a {
		return 0, 0, fmt.Errorf("webp VP8 start code is missing")
	}
	w := int(binary.LittleEndian.Uint16(payload[6:8]) & 0x3fff)
	h := int(binary.LittleEndian.Uint16(payload[8:10]) & 0x3fff)
	return w, h, nil
}

func vp8lSize(payload []byte) (int, int, error) {
	if len(payload) < 5 || payload[0] != 0x2f {
		return 0, 0, fmt.Errorf("webp VP8L signature is missing")
	}
	bits := binary.LittleEndian.Uint32(payload[1:5])
	w := int(bits&0x3fff) + 1
	h := int((bits>>14)&0x3fff) + 1
	return w, h, nil
}

const (
	ebmlID          = 0x1A45DFA3
	ebmlDocType     = 0x4282
	ebmlSegment     = 0x18538067
	ebmlInfo        = 0x1549A966
	ebmlTimecode    = 0x2AD7B1
	ebmlDuration    = 0x4489
	ebmlTracks      = 0x1654AE6B
	ebmlTrackEntry  = 0xAE
	ebmlCodecID     = 0x86
	ebmlVideo       = 0xE0
	ebmlPixelWidth  = 0xB0
	ebmlPixelHeight = 0xBA
)

func validateWebM(b []byte) error {
	info := webmProbe{scale: 1000000}
	if err := walkEBML(b, &info, 0); err != nil {
		return err
	}
	if info.docType != "webm" {
		return fmt.Errorf("webm DocType is %q", info.docType)
	}
	if !info.durationSet {
		return fmt.Errorf("webm duration is missing")
	}
	seconds := info.duration * float64(info.scale) / 1e9
	if seconds < 0 || seconds > maxVideoStickerSeconds {
		return fmt.Errorf("webm duration is %.3fs, limit is %.0fs", seconds, maxVideoStickerSeconds)
	}
	if !info.video {
		return fmt.Errorf("webm has no video track")
	}
	if info.codec != "V_VP9" {
		return fmt.Errorf("webm codec is %q, want V_VP9", info.codec)
	}
	if info.width != stickerSide || info.height != stickerSide {
		return fmt.Errorf("webm is %dx%d, want %dx%d", info.width, info.height, stickerSide, stickerSide)
	}
	return nil
}

type webmProbe struct {
	docType     string
	scale       uint64
	duration    float64
	durationSet bool
	video       bool
	codec       string
	width       int
	height      int
}

func walkEBML(b []byte, info *webmProbe, depth int) error {
	if depth > 8 {
		return fmt.Errorf("webm nesting is too deep")
	}
	for len(b) > 0 {
		id, idn, ok := readEBMLID(b)
		if !ok {
			return fmt.Errorf("webm element id is truncated")
		}
		size, sn, unknown, ok := readEBMLSize(b[idn:])
		if !ok {
			return fmt.Errorf("webm element size is truncated")
		}
		rest := b[idn+sn:]
		var payload []byte
		if unknown {
			payload = rest
			b = nil
		} else {
			if uint64(len(rest)) < size {
				return fmt.Errorf("webm element 0x%x overruns the file", id)
			}
			payload = rest[:size]
			b = rest[size:]
		}
		switch id {
		case ebmlID:
			if err := walkEBML(payload, info, depth+1); err != nil {
				return err
			}
		case ebmlDocType:
			info.docType = string(payload)
		case ebmlSegment, ebmlInfo, ebmlTracks:
			if err := walkEBML(payload, info, depth+1); err != nil {
				return err
			}
		case ebmlTimecode:
			if v := readEBMLUint(payload); v > 0 {
				info.scale = v
			}
		case ebmlDuration:
			d, ok := readEBMLFloat(payload)
			if !ok {
				return fmt.Errorf("webm duration is not a float")
			}
			info.duration = d
			info.durationSet = true
		case ebmlTrackEntry:
			codec, w, h, video, err := readTrack(payload)
			if err != nil {
				return err
			}
			if !video {
				continue
			}
			if info.video {
				return fmt.Errorf("webm has more than one video track")
			}
			info.video = true
			info.codec = codec
			info.width = w
			info.height = h
		}
	}
	return nil
}

func readTrack(b []byte) (codec string, w, h int, video bool, err error) {
	for len(b) > 0 {
		id, idn, ok := readEBMLID(b)
		if !ok {
			return "", 0, 0, false, fmt.Errorf("webm track id is truncated")
		}
		size, sn, unknown, ok := readEBMLSize(b[idn:])
		if !ok {
			return "", 0, 0, false, fmt.Errorf("webm track size is truncated")
		}
		rest := b[idn+sn:]
		var payload []byte
		if unknown {
			payload = rest
			b = nil
		} else {
			if uint64(len(rest)) < size {
				return "", 0, 0, false, fmt.Errorf("webm track element overruns the file")
			}
			payload = rest[:size]
			b = rest[size:]
		}
		switch id {
		case ebmlCodecID:
			codec = string(payload)
		case ebmlVideo:
			video = true
			w, h, err = readVideoSize(payload)
			if err != nil {
				return "", 0, 0, false, err
			}
		}
	}
	return codec, w, h, video, nil
}

func readVideoSize(b []byte) (int, int, error) {
	var w, h int
	var sawW, sawH bool
	for len(b) > 0 {
		id, idn, ok := readEBMLID(b)
		if !ok {
			return 0, 0, fmt.Errorf("webm video id is truncated")
		}
		size, sn, unknown, ok := readEBMLSize(b[idn:])
		if !ok {
			return 0, 0, fmt.Errorf("webm video size is truncated")
		}
		rest := b[idn+sn:]
		var payload []byte
		if unknown {
			payload = rest
			b = nil
		} else {
			if uint64(len(rest)) < size {
				return 0, 0, fmt.Errorf("webm video element overruns the file")
			}
			payload = rest[:size]
			b = rest[size:]
		}
		switch id {
		case ebmlPixelWidth:
			w = int(readEBMLUint(payload))
			sawW = true
		case ebmlPixelHeight:
			h = int(readEBMLUint(payload))
			sawH = true
		}
	}
	if !sawW || !sawH {
		return 0, 0, fmt.Errorf("webm video dimensions are missing")
	}
	return w, h, nil
}

func readEBMLID(b []byte) (uint64, int, bool) {
	if len(b) == 0 || b[0] == 0 {
		return 0, 0, false
	}
	width := 1
	mask := byte(0x80)
	for b[0]&mask == 0 {
		mask >>= 1
		width++
		if width > 8 {
			return 0, 0, false
		}
	}
	if len(b) < width {
		return 0, 0, false
	}
	var id uint64
	for i := 0; i < width; i++ {
		id = id<<8 | uint64(b[i])
	}
	return id, width, true
}

func readEBMLSize(b []byte) (uint64, int, bool, bool) {
	if len(b) == 0 || b[0] == 0 {
		return 0, 0, false, false
	}
	width := 1
	mask := byte(0x80)
	for b[0]&mask == 0 {
		mask >>= 1
		width++
		if width > 8 {
			return 0, 0, false, false
		}
	}
	if len(b) < width {
		return 0, 0, false, false
	}
	var size uint64
	allOnes := true
	size = uint64(b[0] & (mask - 1))
	if b[0]&(mask-1) != mask-1 {
		allOnes = false
	}
	for i := 1; i < width; i++ {
		size = size<<8 | uint64(b[i])
		if b[i] != 0xff {
			allOnes = false
		}
	}
	return size, width, allOnes, true
}

func readEBMLUint(b []byte) uint64 {
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v
}

func readEBMLFloat(b []byte) (float64, bool) {
	switch len(b) {
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(b))), true
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(b)), true
	default:
		return 0, false
	}
}
