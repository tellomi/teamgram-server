package imaging

import (
	"fmt"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/zeromicro/go-zero/core/logx"
)

// staticJPEGQuality matches disintegration/imaging's default JPEG encode
// quality. Derivatives are baseline and carry no EXIF: the historical
// encoder decoded to an image.Image, which drops orientation, then encoded
// again. Do not autorotate here or the size ladder changes shape.
const staticJPEGQuality = 95

type staticFormat int

const (
	staticJPEG staticFormat = iota
	staticPNG
	staticWEBP
)

// staticEncode is the govips export. Tests replace it to fail one size
// after earlier sizes have already been encoded in memory.
var staticEncode = encodeStaticImage

var vipsOnce sync.Once

func startVips() {
	vipsOnce.Do(func() {
		// GLib levels are bit flags. LogLevelError is the least-verbose
		// value govips will deliver when it is the verbosity ceiling.
		vips.LoggingSettings(func(domain string, _ vips.LogLevel, message string) {
			logx.Errorf("govips %s: %s", domain, message)
		}, vips.LogLevelError)
		// Zero concurrency lets libvips pick a thread count. Zero caches
		// keep one upload from pinning decoded tiles in the process.
		vips.Startup(&vips.Config{
			ConcurrencyLevel: 0,
			MaxCacheFiles:    0,
			MaxCacheMem:      0,
			MaxCacheSize:     0,
		})
	})
}

func staticFormatOf(extName string) (staticFormat, bool) {
	switch strings.ToLower(extName) {
	case ".jpg", ".jpeg":
		return staticJPEG, true
	case ".png":
		return staticPNG, true
	case ".webp":
		return staticWEBP, true
	default:
		return 0, false
	}
}

type derivedSize struct {
	szType  string
	localId int
	w, h    int32
	b       []byte
}

// reSizeStatic encodes every derivative before cb runs. A govips error
// returns with cb uncalled, so the upload cannot store a partial ladder
// or treat it as success.
func reSizeStatic(rb []byte, extName string, isABC bool, cb func(szType string, localId int, w, h int32, b []byte) error) error {
	format, ok := staticFormatOf(extName)
	if !ok {
		return fmt.Errorf("govips: %s is not a static photo", extName)
	}
	sizes, err := buildStaticSizes(rb, format, isABC)
	if err != nil {
		logx.Errorf("govips resize r(%d) %s: %v", len(rb), extName, err)
		return err
	}
	if len(sizes) == 0 {
		err = fmt.Errorf("govips: no sizes")
		logx.Error(err.Error())
		return err
	}
	for _, sz := range sizes {
		if err = cb(sz.szType, sz.localId, sz.w, sz.h, sz.b); err != nil {
			return err
		}
	}
	return nil
}

func buildStaticSizes(rb []byte, format staticFormat, isABC bool) ([]derivedSize, error) {
	startVips()
	img, err := vips.NewImageFromBuffer(rb)
	if err != nil {
		return nil, err
	}
	defer img.Close()
	if err = keepFirstPage(img); err != nil {
		return nil, err
	}
	if err = preparePixels(img); err != nil {
		return nil, err
	}
	if isABC {
		if err = prepareABC(img); err != nil {
			return nil, err
		}
	}
	if img.Width() <= 0 || img.Height() <= 0 {
		return nil, fmt.Errorf("govips: empty image")
	}

	widthLimited := img.Width() >= img.Height()
	longEdge := img.Height()
	if widthLimited {
		longEdge = img.Width()
	}
	szList := mtproto.ReSizeInfoPhotoList
	if isABC {
		szList = mtproto.ReSizeInfoABCList
	}

	var out []derivedSize
	for _, sz := range szList {
		edge := sz.Size
		stop := false
		if edge >= longEdge {
			edge = longEdge
			stop = true
		}
		resized, err := resizeLongEdge(img, widthLimited, edge)
		if err != nil {
			return nil, err
		}
		encoded, err := staticEncode(resized, format)
		w, h := resized.Width(), resized.Height()
		resized.Close()
		if err != nil {
			return nil, err
		}
		if len(encoded) == 0 || w <= 0 || h <= 0 {
			return nil, fmt.Errorf("govips: empty %s derivative", sz.Type)
		}
		out = append(out, derivedSize{
			szType:  sz.Type,
			localId: sz.LocalId,
			w:       int32(w),
			h:       int32(h),
			b:       encoded,
		})
		if stop {
			break
		}
	}
	return out, nil
}

func keepFirstPage(img *vips.ImageRef) error {
	if img.Pages() <= 1 {
		return nil
	}
	h := img.PageHeight()
	if h <= 0 || h >= img.Height() {
		return nil
	}
	if err := img.ExtractArea(0, 0, img.Width(), h); err != nil {
		return err
	}
	return img.SetPages(1)
}

func preparePixels(img *vips.ImageRef) error {
	if img.Interpretation() != vips.InterpretationSRGB {
		if err := img.ToColorSpace(vips.InterpretationSRGB); err != nil {
			return err
		}
	}
	if img.BandFormat() != vips.BandFormatUchar {
		if err := img.Cast(vips.BandFormatUchar); err != nil {
			return err
		}
	}
	return nil
}

// prepareABC matches the historical square cover used for profile photos.
func prepareABC(img *vips.ImageRef) error {
	w, h := img.Width(), img.Height()
	d := mtproto.PhotoSZDSize
	c := mtproto.PhotoSZCSize
	if w >= d && h >= d {
		if w != h {
			return coverSquare(img, d)
		}
		return nil
	}
	if w <= c && h <= c {
		return coverSquare(img, c)
	}
	if w != h {
		return coverSquare(img, c)
	}
	return nil
}

func coverSquare(img *vips.ImageRef, size int) error {
	w, h := img.Width(), img.Height()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("govips: empty image")
	}
	if w == size && h == size {
		return nil
	}
	short := w
	if h < short {
		short = h
	}
	scale := float64(size) / float64(short)
	if scale != 1 {
		if err := img.Resize(scale, vips.KernelLanczos3); err != nil {
			return err
		}
	}
	w, h = img.Width(), img.Height()
	left := (w - size) / 2
	top := (h - size) / 2
	if left < 0 {
		left = 0
	}
	if top < 0 {
		top = 0
	}
	cw, ch := size, size
	if left+cw > w {
		cw = w - left
	}
	if top+ch > h {
		ch = h - top
	}
	if cw <= 0 || ch <= 0 {
		return fmt.Errorf("govips: crop %dx%d", cw, ch)
	}
	return img.ExtractArea(left, top, cw, ch)
}

func resizeLongEdge(src *vips.ImageRef, widthLimited bool, edge int) (*vips.ImageRef, error) {
	ref, err := src.Copy()
	if err != nil {
		return nil, err
	}
	base := ref.Height()
	if widthLimited {
		base = ref.Width()
	}
	if base <= 0 {
		ref.Close()
		return nil, fmt.Errorf("govips: empty image")
	}
	if base != edge {
		if err = ref.Resize(float64(edge)/float64(base), vips.KernelLanczos3); err != nil {
			ref.Close()
			return nil, err
		}
	}
	return ref, nil
}

func encodeStaticImage(img *vips.ImageRef, format staticFormat) ([]byte, error) {
	switch format {
	case staticJPEG:
		if img.HasAlpha() {
			if err := img.Flatten(&vips.Color{R: 255, G: 255, B: 255}); err != nil {
				return nil, err
			}
		}
		params := vips.NewJpegExportParams()
		params.Quality = staticJPEGQuality
		params.StripMetadata = true
		params.Interlace = false
		out, _, err := img.ExportJpeg(params)
		return out, err
	case staticPNG:
		params := vips.NewPngExportParams()
		params.StripMetadata = true
		out, _, err := img.ExportPng(params)
		return out, err
	case staticWEBP:
		params := vips.NewWebpExportParams()
		params.StripMetadata = true
		out, _, err := img.ExportWebp(params)
		return out, err
	default:
		return nil, fmt.Errorf("govips: unsupported format")
	}
}
