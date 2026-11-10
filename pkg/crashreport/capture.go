package crashreport

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Incoming is one private message delivered to the crash-report bot.
type Incoming struct {
	SenderID   int64
	MsgID      int32
	When       time.Time
	Text       string
	Filename   string
	Mime       string
	DocumentID int64
}

// Capture copies a document, or the text body, into object storage and writes
// a crash_reports row. A nil service does nothing so a process that has not
// opened MySQL still accepts the chat message.
func Capture(ctx context.Context, in Incoming) error {
	s := current()
	if s == nil {
		return nil
	}
	return s.capture(ctx, in)
}

func (s *Service) capture(ctx context.Context, in Incoming) error {
	if s == nil || in.SenderID == 0 || in.MsgID == 0 || in.SenderID == AccountID {
		return nil
	}
	when := in.When
	if when.IsZero() {
		when = time.Now()
	}
	kind := "text"
	name := in.Filename
	mime := in.Mime
	status := StatusStored
	var size int64
	var path string
	if in.DocumentID != 0 {
		kind = "document"
		src, err := s.document(ctx, in.DocumentID)
		if err != nil {
			return err
		}
		if name == "" {
			name = src.Name
		}
		if name == "" {
			name = "file.bin"
		}
		if mime == "" {
			mime = src.Mime
		}
		size = src.Size
		if size > MaxBytes() {
			status = StatusSkipped
		} else {
			if s.objects == nil {
				return fmt.Errorf("object storage is not configured")
			}
			path = ObjectPath(when, in.SenderID, in.MsgID, name)
			if err = s.objects.Copy(ctx, src.Class, src.Path, path); err != nil {
				return err
			}
		}
	} else {
		body := []byte(in.Text)
		size = int64(len(body))
		if name == "" {
			name = "message.txt"
		}
		if mime == "" {
			mime = "text/plain"
		}
		if size > MaxBytes() {
			status = StatusSkipped
		} else {
			if s.objects == nil {
				return fmt.Errorf("object storage is not configured")
			}
			path = ObjectPath(when, in.SenderID, in.MsgID, name)
			if err := s.objects.Put(ctx, path, mime, body); err != nil {
				return err
			}
		}
	}
	caption := ""
	message := in.Text
	if kind == "document" {
		caption = in.Text
	}
	device, app := Hints(name, caption, message)
	return s.save(ctx, Row{
		SenderUserID: in.SenderID,
		MsgID:        in.MsgID,
		Kind:         kind,
		Filename:     name,
		Caption:      caption,
		Message:      message,
		Device:       device,
		App:          app,
		FileSize:     size,
		ContentType:  mime,
		StorageClass: Class,
		StorageKey:   FullKey(path),
		Status:       status,
	})
}

// List returns reports and fills a presigned download URL when object storage can sign one.
func (s *Service) List(ctx context.Context, q Query) ([]Row, error) {
	if s == nil {
		return nil, fmt.Errorf("crash report store is not configured")
	}
	rows, err := s.list(ctx, q)
	if err != nil {
		return nil, err
	}
	if s == nil || s.objects == nil {
		return rows, nil
	}
	for i := range rows {
		path := PathOf(rows[i].StorageKey)
		if path == "" || strings.TrimSpace(rows[i].StorageKey) == "" {
			continue
		}
		url, err := s.objects.Presign(ctx, path, 15*time.Minute)
		if err != nil {
			continue
		}
		rows[i].DownloadURL = url
	}
	return rows, nil
}
