package catalog

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
)

// centerIconScale is how much of the 512 canvas the center_icon glyph fills.
// Official iOS draws a builtin reaction's center_icon at 2× the 20pt pill
// slot and does not clip it (ReactionIconView, case .builtin). Desktop
// draws a full-bleed select animation at half size when center_icon is
// missing (Reactions::resolveImageFor). Both expect the glyph in the
// center half of that canvas. A full-bleed sticker at the 2× scale spills
// out of the pill.
const centerIconScale = 50.0

// varyTGS builds one reaction slot from a Noto animation. Each slot keeps the
// 512×512 canvas and changes the in/out points so appear, select, activate,
// effect, around, and center are not the same idle clip. center is also
// scaled to centerIconScale so the pill glyph matches official clients.
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
		if !insetCenterIcon(doc) {
			return nil, false
		}
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

// insetCenterIcon wraps the current composition in a precomp scaled to
// centerIconScale and centered on the 512 canvas. The other slots stay
// full-bleed; only the pill icon is inset.
func insetCenterIcon(doc map[string]any) bool {
	layers, ok := doc["layers"].([]any)
	if !ok || len(layers) == 0 {
		return false
	}
	const id = "pill-inset"
	assets, _ := doc["assets"].([]any)
	for _, raw := range assets {
		asset, _ := raw.(map[string]any)
		if asset != nil && asset["id"] == id {
			return false
		}
	}
	doc["assets"] = append(assets, map[string]any{
		"id":     id,
		"w":      512.0,
		"h":      512.0,
		"layers": layers,
	})
	ip, _ := asFloat(doc["ip"])
	op, okOp := asFloat(doc["op"])
	if !okOp {
		op = 30
	}
	doc["layers"] = []any{map[string]any{
		"ddd":   0.0,
		"ind":   1.0,
		"ty":    0.0,
		"nm":    "pill-inset",
		"refId": id,
		"sr":    1.0,
		"ks": map[string]any{
			"o": map[string]any{"a": 0.0, "k": 100.0},
			"r": map[string]any{"a": 0.0, "k": 0.0},
			"p": map[string]any{"a": 0.0, "k": []any{256.0, 256.0, 0.0}},
			"a": map[string]any{"a": 0.0, "k": []any{256.0, 256.0, 0.0}},
			"s": map[string]any{"a": 0.0, "k": []any{centerIconScale, centerIconScale, 100.0}},
		},
		"ao": 0.0,
		"w":  512.0,
		"h":  512.0,
		"ip": ip,
		"op": op,
		"st": 0.0,
		"bm": 0.0,
	}}
	return true
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
