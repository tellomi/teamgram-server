package catalog

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestStickerFileRules(t *testing.T) {
	if err := validateStickerFile(false, webpVP8X(512, 512)); err != nil {
		t.Fatal(err)
	}
	if err := validateStickerFile(false, webpVP8X(256, 512)); err == nil {
		t.Fatal("accepted a 256px webp")
	}
	if err := validateStickerFile(false, []byte("not a webp")); err == nil {
		t.Fatal("accepted a non-webp")
	}
	if err := validateStickerFile(true, sampleWebM(2.5, "V_VP9", 512, 512)); err != nil {
		t.Fatal(err)
	}
	if err := validateStickerFile(true, sampleWebM(3, "V_VP9", 512, 512)); err != nil {
		t.Fatal(err)
	}
	if err := validateStickerFile(true, sampleWebM(3.1, "V_VP9", 512, 512)); err == nil {
		t.Fatal("accepted a webm longer than 3s")
	}
	if err := validateStickerFile(true, sampleWebM(1, "V_VP8", 512, 512)); err == nil {
		t.Fatal("accepted a VP8 webm")
	}
	if err := validateStickerFile(true, sampleWebM(1, "V_VP9", 256, 256)); err == nil {
		t.Fatal("accepted a 256px webm")
	}
	big := append(sampleWebM(1, "V_VP9", 512, 512), make([]byte, maxVideoStickerBytes)...)
	if err := validateStickerFile(true, big); err == nil {
		t.Fatal("accepted a webm over 256KB")
	}
}

func webpVP8X(w, h int) []byte {
	payload := make([]byte, 10)
	ww, hh := w-1, h-1
	payload[4] = byte(ww)
	payload[5] = byte(ww >> 8)
	payload[6] = byte(ww >> 16)
	payload[7] = byte(hh)
	payload[8] = byte(hh >> 8)
	payload[9] = byte(hh >> 16)
	chunk := append([]byte("VP8X"), byte(len(payload)), 0, 0, 0)
	chunk = append(chunk, payload...)
	data := append([]byte("WEBP"), chunk...)
	size := uint32(len(data))
	out := []byte{'R', 'I', 'F', 'F', byte(size), byte(size >> 8), byte(size >> 16), byte(size >> 24)}
	return append(out, data...)
}

func sampleWebM(seconds float64, codec string, w, h uint64) []byte {
	const scale = uint64(1000000)
	info := ebmlElement(idInfo, append(ebmlUint(idTimecode, scale), ebmlFloat(idDuration, seconds*1e9/float64(scale))...))
	video := ebmlElement(idVideo, append(ebmlUint(idPW, w), ebmlUint(idPH, h)...))
	track := ebmlElement(idTrack, append(ebmlString(idCodec, codec), video...))
	segment := ebmlElement(idSegment, append(info, ebmlElement(idTracks, track)...))
	return append(ebmlElement(idEBML, ebmlString(idDocType, "webm")), segment...)
}

var (
	idEBML     = []byte{0x1A, 0x45, 0xDF, 0xA3}
	idDocType  = []byte{0x42, 0x82}
	idSegment  = []byte{0x18, 0x53, 0x80, 0x67}
	idInfo     = []byte{0x15, 0x49, 0xA9, 0x66}
	idTimecode = []byte{0x2A, 0xD7, 0xB1}
	idDuration = []byte{0x44, 0x89}
	idTracks   = []byte{0x16, 0x54, 0xAE, 0x6B}
	idTrack    = []byte{0xAE}
	idCodec    = []byte{0x86}
	idVideo    = []byte{0xE0}
	idPW       = []byte{0xB0}
	idPH       = []byte{0xBA}
)

func ebmlElement(id, data []byte) []byte {
	out := append([]byte{}, id...)
	out = append(out, ebmlSize(len(data))...)
	return append(out, data...)
}

func ebmlSize(n int) []byte {
	if n < 127 {
		return []byte{0x80 | byte(n)}
	}
	return []byte{0x40 | byte(n>>8), byte(n)}
}

func ebmlUint(id []byte, v uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	i := 0
	for i < 7 && buf[i] == 0 {
		i++
	}
	return ebmlElement(id, buf[i:])
}

func ebmlFloat(id []byte, v float64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], math.Float64bits(v))
	return ebmlElement(id, buf[:])
}

func ebmlString(id []byte, s string) []byte {
	return ebmlElement(id, []byte(s))
}
