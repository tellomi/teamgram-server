package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/ffmpegutil"
)

// chooseVideoSize picks the dimensions stored on documentAttributeVideo.
//
// Clients lay a video bubble out from these numbers, or from the photo
// thumb that was generated to match them. A square value here center-crops
// a tall video. The probed display size wins when the client sent a square
// (a thumb box, not the picture) or when the file carries a display matrix.
// A round video stays on the client size: the circle crop is intentional.
// Without a display matrix, a non-square client size is kept, so a probe
// that only saw the coded landscape frame cannot overwrite a portrait the
// client already measured.
func chooseVideoSize(clientW, clientH, probeW, probeH int32, round, rotated bool) (int32, int32) {
	if round {
		if clientW > 0 && clientH > 0 {
			return clientW, clientH
		}
		return probeW, probeH
	}
	if probeW <= 0 || probeH <= 0 {
		return clientW, clientH
	}
	if clientW <= 0 || clientH <= 0 {
		return probeW, probeH
	}
	if clientW == clientH && probeW != probeH {
		return probeW, probeH
	}
	if rotated {
		return probeW, probeH
	}
	if clientW != clientH {
		return clientW, clientH
	}
	return probeW, probeH
}

func probeVideoDisplay(ff *ffmpegutil.FFmpegUtil, data []byte) (w, h, rot int) {
	if ff == nil || len(data) == 0 {
		return 0, 0, 0
	}
	md, err := ff.GetMetadataByPipe(data)
	if err != nil || md == nil {
		return 0, 0, 0
	}
	w, h = ffmpegutil.DisplaySize(md)
	rot = ffmpegutil.Rotation(md)
	return w, h, rot
}

func chosenAttributeSize(attrs []*mtproto.DocumentAttribute, dispW, dispH, rot int) (int32, int32) {
	for _, attr := range attrs {
		if attr.GetPredicateName() != mtproto.Predicate_documentAttributeVideo {
			continue
		}
		return chooseVideoSize(attr.GetW(), attr.GetH(), int32(dispW), int32(dispH), attr.GetRoundMessage(), rot != 0)
	}
	if dispW > 0 && dispH > 0 {
		return int32(dispW), int32(dispH)
	}
	return 0, 0
}

// fitAspect scales w x h so the long side is at most longSide, preserving
// the ratio. It never turns a non-square picture into a square.
func fitAspect(w, h, longSide int32) (int32, int32) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if longSide < 1 {
		longSide = 1
	}
	if w >= h {
		if w <= longSide {
			return w, h
		}
		nh := h * longSide / w
		if nh < 1 {
			nh = 1
		}
		return longSide, nh
	}
	if h <= longSide {
		return w, h
	}
	nw := w * longSide / h
	if nw < 1 {
		nw = 1
	}
	return nw, longSide
}
