package ffmpegutil

import (
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/goffmpeg/models"
)

func TestDisplaySizeAppliesRotationAndSAR(t *testing.T) {
	portrait := &models.Metadata{Streams: []models.Streams{{
		CodecType: "video", Width: 1080, Height: 1920, SampleAspectRatio: "1:1",
	}}}
	w, h := DisplaySize(portrait)
	if w != 1080 || h != 1920 {
		t.Fatalf("portrait coded %dx%d", w, h)
	}

	phone := &models.Metadata{Streams: []models.Streams{{
		CodecType: "video", Width: 1920, Height: 1080, SampleAspectRatio: "1:1",
		SideDataList: []models.SideData{{SideDataType: "Display Matrix", Rotation: -90}},
	}}}
	w, h = DisplaySize(phone)
	if w != 1080 || h != 1920 {
		t.Fatalf("rotated phone video %dx%d", w, h)
	}
	if Rotation(phone) != -90 {
		t.Fatalf("rotation %d", Rotation(phone))
	}

	tagged := &models.Metadata{Streams: []models.Streams{{
		CodecType: "video", Width: 1920, Height: 1080,
		Tags: models.StreamTags{Rotate: "90"},
	}}}
	w, h = DisplaySize(tagged)
	if w != 1080 || h != 1920 {
		t.Fatalf("rotate tag %dx%d", w, h)
	}

	anamorphic := &models.Metadata{Streams: []models.Streams{{
		CodecType: "video", Width: 720, Height: 720, SampleAspectRatio: "3:4",
	}}}
	w, h = DisplaySize(anamorphic)
	if w != 540 || h != 720 {
		t.Fatalf("sar %dx%d", w, h)
	}

	square := &models.Metadata{Streams: []models.Streams{{
		CodecType: "video", Width: 640, Height: 640, SampleAspectRatio: "1:1",
	}}}
	w, h = DisplaySize(square)
	if w != 640 || h != 640 {
		t.Fatalf("square %dx%d", w, h)
	}
}

func TestDisplaySizeJSONRotateNumber(t *testing.T) {
	raw := []byte(`{"streams":[{"codec_type":"video","width":1920,"height":1080,"sample_aspect_ratio":"1:1","tags":{"rotate":-90}}]}`)
	var md models.Metadata
	if err := json.Unmarshal(raw, &md); err != nil {
		t.Fatal(err)
	}
	w, h := DisplaySize(&md)
	if w != 1080 || h != 1920 {
		t.Fatalf("numeric rotate %dx%d", w, h)
	}
}

func TestDisplaySizeOfPortraitFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}
	path := t.TempDir() + "/tall.mp4"
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "color=c=blue:s=360x640:d=1,format=yuv420p", "-c:v", "libx264", "-pix_fmt", "yuv420p", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	md, err := NewFFmpegUtil().GetMetadata(path)
	if err != nil {
		t.Fatal(err)
	}
	w, h := DisplaySize(md)
	if w <= 0 || h <= w {
		t.Fatalf("display %dx%d", w, h)
	}
	frame, err := NewFFmpegUtil().GetFirstFrame(path)
	if err != nil {
		t.Fatal(err)
	}
	fw, fh := jpegSize(t, frame)
	if fw <= 0 || fh <= fw {
		t.Fatalf("frame %dx%d", fw, fh)
	}
}

func jpegSize(t *testing.T, data []byte) (int, int) {
	t.Helper()
	i := 0
	for i+8 < len(data) {
		if data[i] != 0xff {
			i++
			continue
		}
		marker := data[i+1]
		if marker == 0xc0 || marker == 0xc2 {
			h := int(data[i+5])<<8 | int(data[i+6])
			w := int(data[i+7])<<8 | int(data[i+8])
			return w, h
		}
		if marker == 0xd8 || marker == 0xd9 {
			i += 2
			continue
		}
		if i+3 >= len(data) {
			break
		}
		n := int(data[i+2])<<8 | int(data[i+3])
		if n < 2 {
			break
		}
		i += 2 + n
	}
	t.Fatal("no SOF marker")
	return 0, 0
}
