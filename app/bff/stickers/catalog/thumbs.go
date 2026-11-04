package catalog

import (
	"bytes"
	"image"
	"image/jpeg"
	"strings"
	"sync"

	"github.com/chai2010/webp"
)

const thumbSide = 72

var thumbCache sync.Map // asset path -> []byte

// thumbJPEG is a 72×72 JPEG the sticker panel can draw without decoding
// the sticker file itself. Animated documents use the matching Twemoji still.
func (item *Item) thumbJPEG() []byte {
	if item == nil {
		return nil
	}
	return jpegFromAsset(item.thumbAsset())
}

func (item *Item) thumbAsset() string {
	if item == nil {
		return ""
	}
	if strings.HasSuffix(item.AssetPath, ".webp") {
		return item.AssetPath
	}
	code := strings.TrimSuffix(item.FileName, ".tgs")
	if code == "" || code == item.FileName {
		return ""
	}
	return "assets/twemoji/" + code + ".webp"
}

func jpegFromAsset(path string) []byte {
	if path == "" {
		return nil
	}
	if v, ok := thumbCache.Load(path); ok {
		return v.([]byte)
	}
	raw, ok := readAsset(path)
	if !ok {
		thumbCache.Store(path, []byte(nil))
		return nil
	}
	img, err := webp.Decode(bytes.NewReader(raw))
	if err != nil {
		thumbCache.Store(path, []byte(nil))
		return nil
	}
	var buf bytes.Buffer
	if err = jpeg.Encode(&buf, scaleImage(img, thumbSide), &jpeg.Options{Quality: 70}); err != nil || buf.Len() == 0 {
		thumbCache.Store(path, []byte(nil))
		return nil
	}
	out := buf.Bytes()
	thumbCache.Store(path, out)
	return out
}

func scaleImage(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	if w <= max && h <= max {
		return src
	}
	nw, nh := max, max
	if w >= h {
		nh = h * max / w
	} else {
		nw = w * max / h
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + y*h/nh
		for x := 0; x < nw; x++ {
			sx := b.Min.X + x*w/nw
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// ThumbFile is the JPEG for upload.getFile when the client asks for a
// sticker thumb. size is the photoSize type ("m").
func (s *Service) ThumbFile(id int64, size string) (body []byte, mime string, ok bool) {
	if s == nil || id == 0 || size == "" {
		return nil, "", false
	}
	s.mu.Lock()
	link := s.docs[id]
	s.mu.Unlock()
	if link == nil || link.item == nil {
		return nil, "", false
	}
	jpg := link.item.thumbJPEG()
	if len(jpg) == 0 {
		return nil, "", false
	}
	return jpg, "image/jpeg", true
}
