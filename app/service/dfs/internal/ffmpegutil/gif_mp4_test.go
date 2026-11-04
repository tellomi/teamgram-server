package ffmpegutil

import (
	"os"
	"os/exec"
	"testing"
)

func TestConvertBytesToMp4(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Fatal("ffmpeg is required to transcode an animated GIF")
	}
	gif, err := os.ReadFile("safe_image.gif")
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := NewFFmpegUtil().ConvertBytesToMp4(gif, 160, 120)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 12 || string(out[4:8]) != "ftyp" {
		t.Fatalf("not an mp4 (%d bytes)", len(out))
	}
	frame, err := NewFFmpegUtil().GetFirstFrameByPipe(out)
	if err != nil || len(frame) < 3 || frame[0] != 0xff || frame[1] != 0xd8 {
		t.Fatalf("first frame %d %v", len(frame), err)
	}
}
