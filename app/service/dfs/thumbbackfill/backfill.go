// Package thumbbackfill finds photo_sizes rows whose object is missing and
// rebuilds a JPEG thumb from the source document, or flags the row when
// that is not possible. It does not delete rows.
package thumbbackfill

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"

	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
)

// FitJPEG re-encodes a video frame as the medium JPEG clients download.
// The long side is at most 320, matching dfs.uploadDocumentFileV2.
func FitJPEG(frame []byte) ([]byte, error) {
	if len(frame) == 0 {
		return nil, errors.New("empty frame")
	}
	img, err := imaging.Decode(bytes.NewReader(frame))
	if err != nil {
		if isJPEG(frame) {
			return append([]byte(nil), frame...), nil
		}
		return nil, err
	}
	img = fitSide(img, 320)
	var buf bytes.Buffer
	if err = imaging.EncodeJpeg(&buf, img); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, errors.New("empty jpeg")
	}
	return buf.Bytes(), nil
}

func fitSide(img image.Image, max int) image.Image {
	b := img.Bounds()
	if b.Dx() >= b.Dy() {
		if b.Dx() > max {
			return imaging.Resize(img, max, 0)
		}
		return img
	}
	if b.Dy() > max {
		return imaging.Resize(img, 0, max)
	}
	return img
}

func isJPEG(b []byte) bool {
	return len(b) > 2 && b[0] == 0xff && b[1] == 0xd8
}

// Row is one stored photo size that claims an object.
type Row struct {
	ID           int64
	PhotoSizeID  int64
	SizeType     string
	Path         string
	Width        int32
	Height       int32
	FileSize     int32
	Mime         string
	DocumentPath string
}

// DB pages photo_sizes rows that point at an object.
type DB interface {
	Next(ctx context.Context, after int64, limit int) ([]Row, error)
	// UpdateFileSize records the rebuilt object length. A dry run does not call it.
	UpdateFileSize(ctx context.Context, id int64, size int32) error
}

// Blobs is the photos bucket plus the source document.
type Blobs interface {
	// Exists reports whether the photo object is present and non-empty.
	Exists(ctx context.Context, path string) (bool, error)
	ReadDocument(ctx context.Context, path string) ([]byte, error)
	PutPhoto(ctx context.Context, path string, body []byte) error
}

// Frame turns source-document bytes into a JPEG. Nil means ffmpeg is absent.
type Frame func(src []byte) ([]byte, error)

// Options controls one run. DryRun is the default the command uses.
type Options struct {
	After  int64
	Batch  int
	Max    int
	DryRun bool
	Frame  Frame
}

// Report is the outcome of one run.
type Report struct {
	Scanned int
	Present int
	Regen   int
	Flagged int
	Failed  int
	Last    int64
	Flags   []string
}

func (r Report) String() string {
	return fmt.Sprintf("scanned=%d present=%d regen=%d flagged=%d failed=%d last=%d",
		r.Scanned, r.Present, r.Regen, r.Flagged, r.Failed, r.Last)
}

// Run walks photo size rows after Options.After. DryRun does not write.
func Run(ctx context.Context, db DB, blobs Blobs, opt Options) (Report, error) {
	if db == nil || blobs == nil {
		return Report{}, fmt.Errorf("thumb backfill: db and blobs are required")
	}
	if opt.Batch <= 0 {
		opt.Batch = 100
	}
	var rep Report
	after := opt.After
	for {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		n := opt.Batch
		if opt.Max > 0 {
			left := opt.Max - rep.Scanned
			if left <= 0 {
				return rep, nil
			}
			if left < n {
				n = left
			}
		}
		rows, err := db.Next(ctx, after, n)
		if err != nil {
			return rep, err
		}
		if len(rows) == 0 {
			return rep, nil
		}
		for _, row := range rows {
			rep.Scanned++
			rep.Last = row.ID
			after = row.ID
			if err := ctx.Err(); err != nil {
				return rep, err
			}
			ok, err := blobs.Exists(ctx, row.Path)
			if err != nil {
				rep.Failed++
				rep.Flags = append(rep.Flags, fmt.Sprintf("FLAG %d %s %s exists: %v", row.PhotoSizeID, row.SizeType, row.Path, err))
				continue
			}
			if ok {
				rep.Present++
				continue
			}
			reason := flagReason(row, opt.Frame)
			if reason != "" {
				rep.Flagged++
				rep.Flags = append(rep.Flags, fmt.Sprintf("FLAG %d %s %s %s", row.PhotoSizeID, row.SizeType, row.Path, reason))
				continue
			}
			if opt.DryRun {
				rep.Regen++
				rep.Flags = append(rep.Flags, fmt.Sprintf("WOULD_REGEN %d %s %s", row.PhotoSizeID, row.SizeType, row.Path))
				continue
			}
			src, err := blobs.ReadDocument(ctx, row.DocumentPath)
			if err != nil || len(src) == 0 {
				rep.Flagged++
				if err == nil {
					err = errors.New("empty document")
				}
				rep.Flags = append(rep.Flags, fmt.Sprintf("FLAG %d %s %s read: %v", row.PhotoSizeID, row.SizeType, row.Path, err))
				continue
			}
			jpeg, err := opt.Frame(src)
			if err != nil || len(jpeg) == 0 {
				rep.Flagged++
				if err == nil {
					err = errors.New("empty frame")
				}
				rep.Flags = append(rep.Flags, fmt.Sprintf("FLAG %d %s %s frame: %v", row.PhotoSizeID, row.SizeType, row.Path, err))
				continue
			}
			if err = blobs.PutPhoto(ctx, row.Path, jpeg); err != nil {
				rep.Failed++
				rep.Flags = append(rep.Flags, fmt.Sprintf("FLAG %d %s %s put: %v", row.PhotoSizeID, row.SizeType, row.Path, err))
				continue
			}
			if err = db.UpdateFileSize(ctx, row.ID, int32(len(jpeg))); err != nil {
				rep.Failed++
				rep.Flags = append(rep.Flags, fmt.Sprintf("FLAG %d %s %s size: %v", row.PhotoSizeID, row.SizeType, row.Path, err))
				continue
			}
			rep.Regen++
		}
		if len(rows) < n {
			return rep, nil
		}
	}
}

func flagReason(row Row, frame Frame) string {
	if row.Path == "" {
		return "no object path"
	}
	if row.DocumentPath == "" {
		return "no source document"
	}
	switch row.Mime {
	case "video/mp4", "video/webm", "video/quicktime", "image/gif", "image/webp", "image/jpeg", "image/png":
	default:
		if row.Mime == "" {
			return "document mime unknown"
		}
		return "mime " + row.Mime + " is not a video or image"
	}
	if frame == nil {
		return "ffmpeg unavailable"
	}
	return ""
}
