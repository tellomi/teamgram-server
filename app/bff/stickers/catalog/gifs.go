package catalog

import (
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/dcoptions"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// gifClip is one animation in the GIF panel. Built-in clips keep their
// bytes in the binary. Imported clips point at stickers/<id>.dat in the
// same DFS bucket as sticker documents.
type gifClip struct {
	code       string
	title      string
	w, h       int32
	keywords   []string
	mime       string
	size       int64
	duration   float64
	fileName   string
	objectPath string
	thumbPath  string
	poster     []byte
	imported   bool
	id         int64
	access     int64
}

var gifClips = []gifClip{
	{code: "clip0", title: "Red", w: 160, h: 120},
	{code: "clip1", title: "Blue", w: 160, h: 120},
	{code: "clip2", title: "Green", w: 160, h: 120},
	{code: "clip3", title: "Orange", w: 160, h: 120},
	{code: "clip4", title: "Purple", w: 160, h: 120},
	{code: "clip5", title: "Teal", w: 160, h: 120},
}

func gifID(code string) int64     { return stableID("gif", code) }
func gifAccess(code string) int64 { return stableID("gif-access", code) }

func gifClipByID(id int64) (gifClip, bool) {
	for _, clip := range gifClips {
		if gifID(clip.code) == id {
			return clip, true
		}
	}
	return gifClip{}, false
}

// GifFile is the mp4 for a bundled GIF document.
func GifFile(id int64) (body []byte, mime string, ok bool) {
	clip, found := gifClipByID(id)
	if !found {
		return nil, "", false
	}
	body, ok = readAsset("assets/gifs/" + clip.code + ".mp4")
	if !ok {
		return nil, "", false
	}
	return body, "video/mp4", true
}

// GifThumb is the JPEG poster (thumb type "m") or the mp4 itself for the
// video thumb (type "v") upload.getFile asks for.
func GifThumb(id int64, size string) (body []byte, mime string, ok bool) {
	clip, found := gifClipByID(id)
	if !found {
		clip, found = importedGifByID(id)
		if !found {
			return nil, "", false
		}
		switch size {
		case "v", "f":
			return importedGifBytes(clip)
		default:
			if len(clip.poster) == 0 {
				return nil, "", false
			}
			return clip.poster, "image/jpeg", true
		}
	}
	switch size {
	case "v", "f":
		return GifFile(id)
	default:
		body, ok = readAsset("assets/gifs/" + clip.code + ".jpg")
		if !ok || len(body) == 0 {
			return nil, "", false
		}
		return body, "image/jpeg", true
	}
}

func gifDocument(clip gifClip) *mtproto.Document {
	if clip.imported {
		return importedGifDocument(clip)
	}
	mp4, _ := readAsset("assets/gifs/" + clip.code + ".mp4")
	jpg, _ := readAsset("assets/gifs/" + clip.code + ".jpg")
	var thumbs []*mtproto.PhotoSize
	if len(jpg) > 0 {
		thumbs = []*mtproto.PhotoSize{
			mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{
				Type:  mtproto.PhotoSZMediumType,
				W:     clip.w,
				H:     clip.h,
				Size2: int32(len(jpg)),
			}).To_PhotoSize(),
		}
	}
	var video []*mtproto.VideoSize
	if len(mp4) > 0 {
		video = []*mtproto.VideoSize{
			mtproto.MakeTLVideoSize(&mtproto.VideoSize{
				Type:  "v",
				W:     clip.w,
				H:     clip.h,
				Size2: int32(len(mp4)),
			}).To_VideoSize(),
		}
	}
	return mtproto.MakeTLDocument(&mtproto.Document{
		Id:            gifID(clip.code),
		AccessHash:    gifAccess(clip.code),
		FileReference: []byte{},
		Date:          stickerDate,
		MimeType:      "video/mp4",
		Size2_INT32:   int32(len(mp4)),
		Size2_INT64:   int64(len(mp4)),
		Size2:         int64(len(mp4)),
		Thumbs:        thumbs,
		VideoThumbs:   video,
		DcId:          dcoptions.FileDC(),
		Attributes: []*mtproto.DocumentAttribute{
			mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
				SupportsStreaming: true,
				Nosound:           true,
				Duration:          1,
				Duration_FLOAT64:  1,
				W:                 clip.w,
				H:                 clip.h,
			}).To_DocumentAttribute(),
			mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{
				FileName: clip.code + ".mp4",
			}).To_DocumentAttribute(),
			mtproto.MakeTLDocumentAttributeAnimated(&mtproto.DocumentAttribute{}).To_DocumentAttribute(),
		},
	}).To_Document()
}

func bundledGifDocuments(query string) []*mtproto.Document {
	q := foldText(query)
	var out []*mtproto.Document
	for _, clip := range allGifClips() {
		if !gifMatches(clip, q) {
			continue
		}
		out = append(out, gifDocument(clip))
	}
	return out
}

func gifMatches(clip gifClip, q string) bool {
	if q == "" {
		return true
	}
	if strings.Contains(foldText(clip.title), q) || strings.Contains(foldText(clip.code), q) {
		return true
	}
	for _, word := range clip.keywords {
		if strings.Contains(foldText(word), q) {
			return true
		}
	}
	return false
}

// BundledGifs is the default saved-GIF list. A user who has not saved
// anything still sees these in the GIF panel.
func BundledGifs(hash int64) *mtproto.Messages_SavedGifs {
	docs := bundledGifDocuments("")
	var ids []int64
	for _, doc := range docs {
		ids = append(ids, doc.GetId())
	}
	sum := hash64(ids)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesSavedGifsNotModified(&mtproto.Messages_SavedGifs{}).To_Messages_SavedGifs()
	}
	return mtproto.MakeTLMessagesSavedGifs(&mtproto.Messages_SavedGifs{
		Hash: sum,
		Gifs: docs,
	}).To_Messages_SavedGifs()
}

// GifBotUser is the @gif inline bot iOS resolves before it asks for trending GIFs.
func GifBotUser() *mtproto.User {
	return mtproto.MakeTLUser(&mtproto.User{
		Bot:                  true,
		Verified:             true,
		Id:                   mtproto.BotGifId,
		AccessHash:           wrapperspb.Int64(gifAccess("bot")),
		FirstName:            wrapperspb.String("GIF"),
		Username:             wrapperspb.String(mtproto.BotGifName),
		BotInlinePlaceholder: wrapperspb.String("Search GIFs"),
		BotInfoVersion:       wrapperspb.Int32(1),
	}).To_User()
}

// GifInlineResults is messages.getInlineBotResults for @gif.
// Results are the built-in clips plus the imported catalog. query matches
// a title, a code, or a keyword, including simplified Chinese.
func GifInlineResults(query string) []*mtproto.BotInlineResult {
	docs := bundledGifDocuments(query)
	out := make([]*mtproto.BotInlineResult, 0, len(docs))
	for _, doc := range docs {
		out = append(out, mtproto.MakeTLBotInlineMediaResult(&mtproto.BotInlineResult{
			Id:       strings.TrimSuffix(fileNameOf(doc), ".mp4"),
			Type:     "gif",
			Document: doc,
			SendMessage: mtproto.MakeTLBotInlineMessageMediaAuto(&mtproto.BotInlineMessage{
				Message: "",
			}).To_BotInlineMessage(),
		}).To_BotInlineResult())
	}
	return out
}

func fileNameOf(doc *mtproto.Document) string {
	for _, attr := range doc.GetAttributes() {
		if attr.GetPredicateName() == mtproto.Predicate_documentAttributeFilename && attr.GetFileName() != "" {
			return attr.GetFileName()
		}
	}
	return "animation.mp4"
}
