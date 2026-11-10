package imaging

import (
	"fmt"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/zeromicro/go-zero/core/logx"
)

// staticJPEGQuality matches Telegram's photo export (about 87). q95
// re-encoded a 163KB original into a 329KB "x" size. Every stored size has
// EXIF, XMP and IPTC removed. Orientation is applied to the pixels first
// and the tag is then dropped, so a client does not rotate again. A
// full-size JPEG with no rotation is stripped by removing segments, which
// does not re-encode. Anything else is encoded at this quality.
const staticJPEGQuality = 87

// jpegShrinkQualities are tried, in order, when a downscale at
// staticJPEGQuality is still larger than the upload. The first result that
// fits is kept.
var jpegShrinkQualities = []int{75, 60, 45, 30}

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
	szType        string
	localId       int
	w, h          int32
	b             []byte
	reuseOriginal bool
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
	// Bake EXIF orientation into the pixels before any resize. The tag is
	// not kept on the bytes we store. libvips applies all eight values.
	oriented := false
	if format == staticJPEG {
		if o := img.Orientation(); o > 1 && o <= 8 {
			if err = img.AutoRotate(); err != nil {
				return nil, err
			}
			oriented = true
		}
	}
	if err = keepFirstPage(img); err != nil {
		return nil, err
	}
	if err = preparePixels(img); err != nil {
		return nil, err
	}
	// fileW/fileH are the decoded file. prepareABC may cover-crop, and those
	// pixels are not the original bytes. Reuse the upload only when nothing
	// has changed the geometry.
	fileW, fileH := img.Width(), img.Height()
	if isABC {
		if err = prepareABC(img); err != nil {
			return nil, err
		}
	}
	if img.Width() <= 0 || img.Height() <= 0 {
		return nil, fmt.Errorf("govips: empty image")
	}

	srcW, srcH := img.Width(), img.Height()
	widthLimited := srcW >= srcH
	longEdge := srcH
	if widthLimited {
		longEdge = srcW
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
		w, h := resized.Width(), resized.Height()
		// Same pixels as the file, and no EXIF rotation was applied: the
		// largest step is the upload with privacy segments removed. That
		// stays smaller than a q87 re-encode and does not keep GPS. A crop
		// or an orientation change has different pixels, so it is encoded.
		samePixels := !oriented && w == srcW && h == srcH && srcW == fileW && srcH == fileH && len(rb) > 0
		var encoded []byte
		reuse := false
		if samePixels && format == staticJPEG {
			if stripped, ok := stripJPEGMetadata(rb); ok && len(stripped) > 0 && len(stripped) <= len(rb) {
				encoded = stripped
				reuse = true
			}
		}
		if encoded == nil {
			encoded, err = staticEncode(resized, format)
			if err == nil && format == staticJPEG {
				encoded, err = finishJPEG(encoded)
			}
			if err == nil && format == staticJPEG && len(encoded) > len(rb) && len(rb) > 0 && !oriented {
				encoded, err = shrinkJPEG(resized, len(rb))
			}
		}
		resized.Close()
		if err != nil {
			return nil, err
		}
		if len(encoded) == 0 || w <= 0 || h <= 0 {
			return nil, fmt.Errorf("govips: empty %s derivative", sz.Type)
		}
		// JPEG derivatives must not exceed the upload, except when EXIF
		// orientation forced a re-encode of pixels that were stored sideways.
		// PNG and WebP thumbs of a tiny source can be larger than that source.
		if !reuse && !oriented && format == staticJPEG && len(rb) > 0 && len(encoded) > len(rb) {
			return nil, fmt.Errorf("govips: %s derivative is %d bytes, larger than the %d byte original", sz.Type, len(encoded), len(rb))
		}
		out = append(out, derivedSize{
			szType:        sz.Type,
			localId:       sz.LocalId,
			w:             int32(w),
			h:             int32(h),
			b:             encoded,
			reuseOriginal: reuse,
		})
		if stop {
			break
		}
	}
	return out, nil
}

// finishJPEG removes any EXIF libvips wrote back (it keeps a minimal
// orientation tag unless the segments are deleted).
func finishJPEG(b []byte) ([]byte, error) {
	stripped, ok := stripJPEGMetadata(b)
	if !ok || len(stripped) == 0 || jpegHasPrivacy(stripped) {
		return nil, fmt.Errorf("govips: jpeg still contains metadata")
	}
	return stripped, nil
}

// shrinkJPEG re-encodes until the buffer is no larger than limit.
// The caller already tried staticJPEGQuality. Each attempt is stripped.
func shrinkJPEG(img *vips.ImageRef, limit int) ([]byte, error) {
	var best []byte
	for _, q := range jpegShrinkQualities {
		out, err := exportJPEG(img, q)
		if err != nil {
			return nil, err
		}
		out, err = finishJPEG(out)
		if err != nil {
			return nil, err
		}
		if best == nil || len(out) < len(best) {
			best = out
		}
		if len(out) <= limit {
			return out, nil
		}
	}
	if best == nil {
		return nil, fmt.Errorf("govips: empty jpeg")
	}
	if len(best) > limit {
		return nil, fmt.Errorf("govips: jpeg %d bytes exceeds original %d", len(best), limit)
	}
	return best, nil
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

func exportJPEG(img *vips.ImageRef, quality int) ([]byte, error) {
	if img.HasAlpha() {
		if err := img.Flatten(&vips.Color{R: 255, G: 255, B: 255}); err != nil {
			return nil, err
		}
	}
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}
	params := vips.NewJpegExportParams()
	params.Quality = quality
	params.StripMetadata = true
	params.Interlace = false
	out, _, err := img.ExportJpeg(params)
	return out, err
}

func encodeStaticImage(img *vips.ImageRef, format staticFormat) ([]byte, error) {
	switch format {
	case staticJPEG:
		return exportJPEG(img, staticJPEGQuality)
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
