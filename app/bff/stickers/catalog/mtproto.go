package catalog

import (
	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

const stickerDate int32 = 1700000000

func (p *Pack) tl(installed, archived bool) *mtproto.StickerSet {
	var thumbID int64
	if len(p.Items) > 0 {
		thumbID = p.Items[0].DocumentID
	}
	set := &mtproto.StickerSet{
		Archived:   archived,
		Official:   p.Official,
		Masks:      p.Masks,
		Emojis:     p.Emojis,
		TextColor:  p.TextColor,
		Animated:   p.Animated,
		Videos:     p.Videos,
		Id:         p.ID,
		AccessHash: p.AccessHash,
		Title:      p.Title,
		ShortName:  p.ShortName,
		Count:      int32(len(p.Items)),
		Hash:       int32(hash64(p.docIDs())),
		ThumbDcId:  wrapperspb.Int32(1),
	}
	if thumbID != 0 {
		set.ThumbDocumentId = wrapperspb.Int64(thumbID)
		set.ThumbVersion = wrapperspb.Int32(1)
	}
	if installed {
		set.InstalledDate = wrapperspb.Int32(stickerDate)
	}
	return mtproto.MakeTLStickerSet(set).To_StickerSet()
}

func (p *Pack) docIDs() []int64 {
	ids := make([]int64, 0, len(p.Items))
	for _, item := range p.Items {
		ids = append(ids, item.DocumentID)
	}
	return ids
}

func (p *Pack) documents() []*mtproto.Document {
	out := make([]*mtproto.Document, 0, len(p.Items))
	for _, item := range p.Items {
		out = append(out, item.document(p))
	}
	return out
}

func (p *Pack) packs() []*mtproto.StickerPack {
	groups := map[string][]int64{}
	var order []string
	for _, item := range p.Items {
		if _, ok := groups[item.Emoticon]; !ok {
			order = append(order, item.Emoticon)
		}
		groups[item.Emoticon] = append(groups[item.Emoticon], item.DocumentID)
	}
	out := make([]*mtproto.StickerPack, 0, len(order))
	for _, emoticon := range order {
		out = append(out, mtproto.MakeTLStickerPack(&mtproto.StickerPack{
			Emoticon:  emoticon,
			Documents: groups[emoticon],
		}).To_StickerPack())
	}
	return out
}

func (p *Pack) covered(installed, archived bool) *mtproto.StickerSetCovered {
	cover := (*mtproto.Document)(nil)
	if len(p.Items) > 0 {
		cover = p.Items[0].document(p)
	}
	return mtproto.MakeTLStickerSetCovered(&mtproto.StickerSetCovered{
		Set:   p.tl(installed, archived),
		Cover: cover,
	}).To_StickerSetCovered()
}

func (p *Pack) full(installed, archived bool) *mtproto.Messages_StickerSet {
	return mtproto.MakeTLMessagesStickerSet(&mtproto.Messages_StickerSet{
		Set:       p.tl(installed, archived),
		Packs:     p.packs(),
		Keywords:  []*mtproto.StickerKeyword{},
		Documents: p.documents(),
	}).To_Messages_StickerSet()
}

func (item *Item) document(p *Pack) *mtproto.Document {
	set := mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{
		Id:         p.ID,
		AccessHash: p.AccessHash,
	}).To_InputStickerSet()
	attrs := []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeImageSize(&mtproto.DocumentAttribute{
			W: item.Width,
			H: item.Height,
		}).To_DocumentAttribute(),
		mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{
			FileName: item.FileName,
		}).To_DocumentAttribute(),
	}
	if item.CustomEmoji {
		attrs = append(attrs, mtproto.MakeTLDocumentAttributeCustomEmoji(&mtproto.DocumentAttribute{
			Alt:        item.Emoticon,
			Free:       true,
			Stickerset: set,
		}).To_DocumentAttribute())
	}
	attrs = append(attrs, mtproto.MakeTLDocumentAttributeSticker(&mtproto.DocumentAttribute{
		Alt:        item.Emoticon,
		Stickerset: set,
	}).To_DocumentAttribute())
	return mtproto.MakeTLDocument(&mtproto.Document{
		Id:            item.DocumentID,
		AccessHash:    item.AccessHash,
		FileReference: []byte{},
		Date:          stickerDate,
		MimeType:      item.MIME,
		Size2_INT32:   item.Size,
		Size2_INT64:   int64(item.Size),
		Size2:         int64(item.Size),
		DcId:          1,
		Attributes:    attrs,
	}).To_Document()
}
