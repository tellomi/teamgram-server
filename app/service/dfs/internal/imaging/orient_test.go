package imaging

import (
	"image"
	"image/color"
	"testing"
)

func TestMatchDisplayAspectRotatesCodedLandscape(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	got := MatchDisplayAspect(img, -90, 4, 8)
	if got.Bounds().Dx() >= got.Bounds().Dy() {
		t.Fatalf("bounds %v", got.Bounds())
	}
	// -90° CCW is 90° clockwise. The marked pixel moves off the origin.
	if _, _, _, a := got.At(0, 0).RGBA(); a != 0 {
		t.Fatal("clockwise rotation left the marked pixel in place")
	}
}

func TestMatchDisplayAspectLeavesMatchingFrame(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 6))
	got := MatchDisplayAspect(img, -90, 9, 16)
	if got.Bounds().Dx() != 3 || got.Bounds().Dy() != 6 {
		t.Fatalf("rotated an already-tall frame: %v", got.Bounds())
	}
}

func TestMatchDisplayAspectLeavesSquare(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 5, 2))
	got := MatchDisplayAspect(img, -90, 10, 10)
	if got.Bounds().Dx() != 5 || got.Bounds().Dy() != 2 {
		t.Fatalf("square display rotated the frame: %v", got.Bounds())
	}
}
