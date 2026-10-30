package accountassets

import (
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Theme formats from https://core.telegram.org/method/account.uploadTheme.
// mime_type must be application/x-tgtheme-{format}.
var themeFormatMIME = map[string]string{
	"android":  "application/x-tgtheme-android",
	"ios":      "application/x-tgtheme-ios",
	"macos":    "application/x-tgtheme-macos",
	"tdesktop": "application/x-tgtheme-tdesktop",
}

var themeMIMEFormat = map[string]string{
	"application/x-tgtheme-android":  "android",
	"application/x-tgtheme-ios":      "ios",
	"application/x-tgtheme-macos":    "macos",
	"application/x-tgtheme-tdesktop": "tdesktop",
}

var wallMIME = map[string]bool{
	"image/jpeg":                  true,
	"image/jpg":                   true,
	"image/png":                   true,
	"application/x-tgwallpattern": true,
}

// MP3 types are stored as-is. Other accepted types are converted to audio/mpeg.
// https://core.telegram.org/method/account.saveRingtone
var ringtoneMIME = map[string]bool{
	"audio/mpeg":       true,
	"audio/mp3":        true,
	"audio/mpeg3":      true,
	"audio/mp4":        true,
	"audio/m4a":        true,
	"audio/x-m4a":      true,
	"audio/ogg":        true,
	"audio/opus":       true,
	"audio/x-opus+ogg": true,
}

func mp3MIME(mime string) bool {
	switch strings.ToLower(mime) {
	case "audio/mpeg", "audio/mp3", "audio/mpeg3":
		return true
	default:
		return false
	}
}

func formatOK(format string) bool {
	_, ok := themeFormatMIME[format]
	return ok
}

type docRec struct {
	doc   *mtproto.Document
	owner int64
	kind  string // theme, wall, tone
	mime  string
	name  string
	mp3   bool
	// converted is the mp3 document id minted by account.saveRingtone.
	converted int64
}

func fileError(file *mtproto.InputFile) error {
	if file == nil {
		return mtproto.ErrFileIdInvalid
	}
	switch file.GetPredicateName() {
	case mtproto.Predicate_inputFile, mtproto.Predicate_inputFileBig:
	default:
		return mtproto.ErrFileIdInvalid
	}
	if file.GetId_INT64() == 0 {
		return mtproto.ErrFileIdInvalid
	}
	if file.GetParts() <= 0 {
		return mtproto.ErrFilePartsInvalid
	}
	return nil
}

func (s *store) putDoc(owner int64, kind, mime, name string, parts int32, audio bool) *docRec {
	id := s.nextID()
	size := int64(parts) * 1024
	if size <= 0 {
		size = 1024
	}
	attrs := []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{
			FileName: name,
		}).To_DocumentAttribute(),
	}
	if audio {
		attrs = append(attrs, mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{
			Duration_INT32: 1,
			Title:          wrapperspb.String(name),
		}).To_DocumentAttribute())
	}
	doc := mtproto.MakeTLDocument(&mtproto.Document{
		Id:            id,
		AccessHash:    accessOf(id),
		FileReference: []byte{1},
		Date:          int32(time.Now().Unix()),
		MimeType:      mime,
		Size2_INT64:   size,
		DcId:          1,
		Attributes:    attrs,
	}).To_Document()
	rec := &docRec{doc: doc, owner: owner, kind: kind, mime: mime, name: name, mp3: mp3MIME(mime)}
	s.docs[id] = rec
	return rec
}

func cloneDoc(d *mtproto.Document) *mtproto.Document {
	if d == nil {
		return nil
	}
	return proto.Clone(d).(*mtproto.Document)
}

func (s *store) docFor(self int64, in *mtproto.InputDocument, kind string) (*docRec, error) {
	if in == nil || in.GetPredicateName() != mtproto.Predicate_inputDocument || in.GetId() == 0 {
		return nil, mtproto.ErrDocumentInvalid
	}
	rec := s.docs[in.GetId()]
	if rec == nil || rec.kind != kind || rec.owner != self || rec.doc.GetAccessHash() != in.GetAccessHash() {
		return nil, mtproto.ErrDocumentInvalid
	}
	return rec, nil
}

func settingsOK(s *mtproto.WallPaperSettings) bool {
	if s == nil {
		return true
	}
	if s.GetIntensity() != nil {
		v := s.GetIntensity().GetValue()
		if v < -100 || v > 100 {
			return false
		}
	}
	if s.GetRotation() != nil {
		v := s.GetRotation().GetValue()
		if v < 0 || v > 359 || v%45 != 0 {
			return false
		}
	}
	return true
}

func copyWallSettings(in *mtproto.WallPaperSettings) *mtproto.WallPaperSettings {
	if in == nil {
		return nil
	}
	c := proto.Clone(in).(*mtproto.WallPaperSettings)
	return mtproto.MakeTLWallPaperSettings(c).To_WallPaperSettings()
}

func baseOK(b *mtproto.BaseTheme) bool {
	if b == nil {
		return false
	}
	switch b.GetPredicateName() {
	case mtproto.Predicate_baseThemeClassic,
		mtproto.Predicate_baseThemeDay,
		mtproto.Predicate_baseThemeNight,
		mtproto.Predicate_baseThemeTinted,
		mtproto.Predicate_baseThemeArctic:
		return true
	default:
		return false
	}
}
