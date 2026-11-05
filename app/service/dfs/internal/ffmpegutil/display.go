package ffmpegutil

import (
	"strconv"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/goffmpeg/models"
)

// DisplaySize is the size a player shows, after sample aspect ratio and
// the display matrix. Coded width/height stay landscape for many phone
// portrait videos; the rotation is what makes them tall.
func DisplaySize(md *models.Metadata) (w, h int) {
	s := videoStream(md)
	if s == nil || s.Width <= 0 || s.Height <= 0 {
		return 0, 0
	}
	w, h = applySAR(s.Width, s.Height, s.SampleAspectRatio)
	if swapsAxes(Rotation(md)) {
		w, h = h, w
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// Rotation is the ffprobe display angle in degrees, counter-clockwise.
// Zero means the coded frame is already the display orientation.
func Rotation(md *models.Metadata) int {
	s := videoStream(md)
	if s == nil {
		return 0
	}
	for _, side := range s.SideDataList {
		if side.Rotation != 0 {
			return normalizeAngle(int(side.Rotation))
		}
	}
	raw := strings.TrimSpace(string(s.Tags.Rotate))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return normalizeAngle(n)
}

func videoStream(md *models.Metadata) *models.Streams {
	if md == nil {
		return nil
	}
	for i := range md.Streams {
		if md.Streams[i].CodecType == "video" && md.Streams[i].Width > 0 && md.Streams[i].Height > 0 {
			return &md.Streams[i]
		}
	}
	return nil
}

func applySAR(w, h int, sar string) (int, int) {
	sar = strings.TrimSpace(sar)
	switch sar {
	case "", "1:1", "0:1", "N/A":
		return w, h
	}
	parts := strings.Split(sar, ":")
	if len(parts) != 2 {
		return w, h
	}
	a, errA := strconv.Atoi(parts[0])
	b, errB := strconv.Atoi(parts[1])
	if errA != nil || errB != nil || a <= 0 || b <= 0 {
		return w, h
	}
	dw := w * a / b
	if dw < 1 {
		dw = 1
	}
	return dw, h
}

func normalizeAngle(d int) int {
	d %= 360
	if d > 180 {
		d -= 360
	}
	if d < -180 {
		d += 360
	}
	return d
}

func swapsAxes(deg int) bool {
	d := deg % 360
	if d < 0 {
		d += 360
	}
	return d == 90 || d == 270
}
