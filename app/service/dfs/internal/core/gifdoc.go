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
	if len(doc.GetThumbs()) == 0 {
		jpg := solidJPEG(color.RGBA{R: 32, G: 32, B: 32, A: 255})
		doc.Thumbs = []*mtproto.PhotoSize{
			mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{
				Type:  mtproto.PhotoSZMediumType,
				W:     40,
				H:     40,
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

func solidJPEG(c color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60})
	return buf.Bytes()
}
