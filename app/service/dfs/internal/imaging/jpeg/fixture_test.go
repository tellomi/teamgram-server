package jpeg

import (
	"os"
	"testing"
)

func skipWithoutJPEGFixtures(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("../testdata/video-001.jpeg"); err != nil {
		t.Skip("jpeg testdata is not in the tree")
	}
}
