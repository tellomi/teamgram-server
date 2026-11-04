package core

import "github.com/teamgram/proto/mtproto"

// richFromInput converts messages.saveDraft's inputRichMessage into the
// richMessage stored on draftMessage. Blocks are copied as-is. Markdown or
// HTML with no blocks becomes one paragraph so a rich-only draft is not
// cleared. Input photos and documents keep their ids; the file bytes stay
// where upload.saveFilePart put them.
func richFromInput(in *mtproto.InputRichMessage) *mtproto.RichMessage {
	if in == nil {
		return nil
	}
	blocks := append([]*mtproto.PageBlock(nil), in.GetBlocks()...)
	text := in.GetMarkdown()
	if text == "" {
		text = in.GetHtml()
	}
	if len(blocks) == 0 && text != "" {
		blocks = []*mtproto.PageBlock{paragraph(text)}
	}
	photos := photosFromInput(in.GetPhotos())
	docs := documentsFromInput(in.GetDocuments())
	if len(blocks) == 0 && len(photos) == 0 && len(docs) == 0 {
		return nil
	}
	if photos == nil {
		photos = []*mtproto.Photo{}
	}
	if docs == nil {
		docs = []*mtproto.Document{}
	}
	return mtproto.MakeTLRichMessage(&mtproto.RichMessage{
		Rtl:       in.GetRtl(),
		Blocks:    blocks,
		Photos:    photos,
		Documents: docs,
	}).To_RichMessage()
}

func paragraph(text string) *mtproto.PageBlock {
	return mtproto.MakeTLPageBlockParagraph(&mtproto.PageBlock{
		Text: mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: text}).To_RichText(),
	}).To_PageBlock()
}

func photosFromInput(in []*mtproto.InputPhoto) []*mtproto.Photo {
	var out []*mtproto.Photo
	for _, p := range in {
		if p == nil || p.GetId() == 0 {
			continue
		}
		out = append(out, mtproto.MakeTLPhoto(&mtproto.Photo{
			Id:            p.GetId(),
			AccessHash:    p.GetAccessHash(),
			FileReference: p.GetFileReference(),
		}).To_Photo())
	}
	return out
}

func documentsFromInput(in []*mtproto.InputDocument) []*mtproto.Document {
	var out []*mtproto.Document
	for _, d := range in {
		if d == nil || d.GetId() == 0 {
			continue
		}
		out = append(out, mtproto.MakeTLDocument(&mtproto.Document{
			Id:            d.GetId(),
			AccessHash:    d.GetAccessHash(),
			FileReference: d.GetFileReference(),
		}).To_Document())
	}
	return out
}
