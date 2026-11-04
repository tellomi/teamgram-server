package catalog

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
)

// varyTGS builds one reaction slot from a Noto animation. Each slot keeps the
// 512×512 canvas and changes the in/out points so appear, select, activate,
// effect, around, and center are not the same idle clip.
func varyTGS(raw []byte, slot string) ([]byte, bool) {
	r, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, false
	}
	defer r.Close()
	var doc map[string]any
	if err = json.NewDecoder(io.LimitReader(r, 8<<20)).Decode(&doc); err != nil {
		return nil, false
	}
	op, ok := asFloat(doc["op"])
	if !ok || op < 2 {
		op = 30
	}
	switch slot {
	case "appear":
		doc["ip"] = 0.0
		doc["op"] = maxFloat(2, op/3)
	case "select":
		doc["ip"] = op / 4
		doc["op"] = op
	case "activate":
		doc["ip"] = 0.0
		doc["op"] = op
	case "effect":
		if fr, ok := asFloat(doc["fr"]); ok && fr > 1 {
			doc["fr"] = fr + 8
		}
	case "around":
		start := op / 5
		doc["ip"] = start
		doc["op"] = maxFloat(start+2, op*3/4)
	case "center":
		doc["ip"] = maxFloat(0, op-6)
		doc["op"] = op
	default:
		return nil, false
	}
	doc["nm"] = slot
	doc["w"] = 512.0
	doc["h"] = 512.0
	rawJSON, err := json.Marshal(doc)
	if err != nil {
		return nil, false
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err = w.Write(rawJSON); err != nil {
		return nil, false
	}
	if err = w.Close(); err != nil {
		return nil, false
	}
	out := buf.Bytes()
	if !tgsCanvas512(out) {
		return nil, false
	}
	return out, true
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// tgsCanvas512 reports whether a gzipped Lottie file is a 512×512 sticker.
// Telegram clients reject other sizes for reaction animations.
func tgsCanvas512(raw []byte) bool {
	r, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return false
	}
	defer r.Close()
	var meta struct {
		W int `json:"w"`
		H int `json:"h"`
	}
	if err = json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&meta); err != nil && err != io.EOF {
		return false
	}
	return meta.W == 512 && meta.H == 512
}
