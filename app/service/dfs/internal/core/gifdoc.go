package core

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
	"github.com/teamgram/teamgram-server/pkg/dcoptions"
)

func (c *DfsCore) uploadedBytes(creator, id int64) ([]byte, error) {
	info, err := c.svcCtx.Dao.GetFileInfo(c.ctx, creator, id)
	if err != nil || info == nil {
		if err == nil {
			err = mtproto.ErrMediaInvalid
		}
		return nil, err
	}
	return c.svcCtx.Dao.NewSSDBReader(info).ReadAll(c.ctx)
}

// finishGifDocument fills dc id, a single .mp4 filename, and a video thumb.
// An animated GIF without those fields stays blank or spins in the panel.
func finishGifDocument(doc *mtproto.Document, mp4 []byte) *mtproto.Document {
	if doc == nil {
		return nil
	}
	doc.DcId = dcoptions.FileDC()
	var w, h int32
	named := false
	for _, attr := range doc.GetAttributes() {
		switch attr.GetPredicateName() {
		case mtproto.Predicate_documentAttributeFilename:
			attr.FileName = model.AnimatedFileName(attr.GetFileName())
			named = true
		case mtproto.Predicate_documentAttributeVideo:
			w, h = attr.GetW(), attr.GetH()
		}
	}
	if !named {
		doc.Attributes = append(doc.Attributes, mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{
			FileName: "animation.mp4",
		}).To_DocumentAttribute())
	}
	if len(doc.GetThumbs()) == 0 && w > 0 && h > 0 {
		// Telegram clients size the bubble from this photo, and ignore
		// documentAttributeVideo while it has a non-zero w and h. A 40×40
		// stand-in makes a tall video a full-width square and center-crops
		// the top and bottom. The placeholder keeps the video's aspect.
		tw, th := fitAspect(w, h, 40)
		jpg := solidJPEG(color.RGBA{R: 32, G: 32, B: 32, A: 255}, tw, th)
		doc.Thumbs = []*mtproto.PhotoSize{
			mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{
				Type:  mtproto.PhotoSZMediumType,
				W:     tw,
				H:     th,
				Size2: int32(len(jpg)),
			}).To_PhotoSize(),
		}
	}
	if len(doc.GetVideoThumbs()) == 0 && len(mp4) > 0 {
		if w == 0 {
			w = 160
		}
		if h == 0 {
			h = 120
		}
		doc.VideoThumbs = []*mtproto.VideoSize{
			mtproto.MakeTLVideoSize(&mtproto.VideoSize{
				Type:  "v",
				W:     w,
				H:     h,
				Size2: int32(len(mp4)),
			}).To_VideoSize(),
		}
	}
	return doc
}

func solidJPEG(c color.Color, w, h int32) []byte {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	for y := 0; y < int(h); y++ {
		for x := 0; x < int(w); x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60})
	return buf.Bytes()
}
