package catalog

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
)

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
