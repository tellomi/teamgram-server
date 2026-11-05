package imaging

import (
	"image"

	"github.com/disintegration/imaging"
)

// MatchDisplayAspect rotates img so its aspect matches dispW x dispH.
// ffmpegRotationCCW is the ffprobe angle (counter-clockwise). A frame that
// already has the display aspect is left as it is, so an autorotated JPEG
// is not rotated a second time. A square display is left unchanged: round
// videos are square on purpose.
func MatchDisplayAspect(img image.Image, ffmpegRotationCCW, dispW, dispH int) image.Image {
	if img == nil || dispW <= 0 || dispH <= 0 || dispW == dispH {
		return img
	}
	fw, fh := img.Bounds().Dx(), img.Bounds().Dy()
	if fw <= 0 || fh <= 0 || fw == fh {
		return img
	}
	if (fh > fw) == (dispH > dispW) {
		return img
	}
	switch clockwiseFromCCW(ffmpegRotationCCW) {
	case 180:
		return imaging.Rotate180(img)
	case 270:
		return imaging.Rotate270(img)
	default:
		return imaging.Rotate90(img)
	}
}

func clockwiseFromCCW(ccw int) int {
	c := ((-ccw % 360) + 360) % 360
	switch c {
	case 90, 180, 270:
		return c
	default:
		return 90
	}
}
