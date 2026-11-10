package catalog

import (
	"sort"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/dcoptions"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

const stickerDate int32 = 1700000000

func (p *Pack) tl(installed, archived bool) *mtproto.StickerSet {
	items := p.visibleItems()
	thumbID := p.ThumbID
	if thumbID == 0 && len(items) > 0 {
		thumbID = items[0].DocumentID
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
		Count:      int32(len(items)),
		Hash:       p.stickerSetHash(),
	}
	// thumb_dc_id, thumb_version, and thumbs share flags.4. Writing only the
	// ints makes a client read the next field as a vector and fail with
	// DecodeVectorLong on messages.featuredStickers. thumb_document_id is
	// flags.8 and is the cover the panel actually uses.
	if thumbID != 0 {
		set.ThumbDocumentId = wrapperspb.Int64(thumbID)
	}
	if installed {
		set.InstalledDate = wrapperspb.Int32(stickerDate)
	}
	return mtproto.MakeTLStickerSet(set).To_StickerSet()
}

func (p *Pack) docIDs() []int64 {
	return documentIDs(p.visibleItems())
}

// stickerSetHash is the 32-bit StickerSet.hash clients fold into
// getAllStickers, getMaskStickers, and getEmojiStickers. It is the low
// 32 bits of the document-id hash, which may be negative. The list hash
// sign-extends that int32. A title change does not change it.
func (p *Pack) stickerSetHash() int32 {
	if p == nil {
		return 0
	}
	return int32(hash64(documentIDs(p.visibleItems())))
}

func (p *Pack) documents() []*mtproto.Document {
	items := p.visibleItems()
	out := make([]*mtproto.Document, 0, len(items))
	for _, item := range items {
		out = append(out, item.document(p))
	}
	return out
}

func (p *Pack) packs() []*mtproto.StickerPack {
	groups := map[string][]int64{}
	var order []string
	for _, item := range p.visibleItems() {
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
	items := p.visibleItems()
	cover := (*mtproto.Document)(nil)
	if len(items) > 0 {
		cover = items[0].document(p)
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
		Keywords:  p.stickerKeywords(),
		Documents: p.documents(),
	}).To_Messages_StickerSet()
}

// stickerKeywords is the per-document keyword list clients search inside an
// installed pack. Android getAnimatedEmojiByKeywords matches these with
// substring contains, after toLowerCase, so the strings are case-folded.
func (p *Pack) stickerKeywords() []*mtproto.StickerKeyword {
	items := p.visibleItems()
	out := make([]*mtproto.StickerKeyword, 0, len(items))
	for _, item := range items {
		words := item.searchWords()
		if len(words) == 0 {
			continue
		}
		out = append(out, mtproto.MakeTLStickerKeyword(&mtproto.StickerKeyword{
			DocumentId: item.DocumentID,
			Keyword:    words,
		}).To_StickerKeyword())
	}
	return out
}

func (item *Item) searchWords() []string {
	if item == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(words []string) {
		for _, w := range words {
			w = foldText(w)
			if w == "" || seen[w] {
				continue
			}
			seen[w] = true
			out = append(out, w)
		}
	}
	add(item.Keywords)
	for _, words := range item.LangKeywords {
		add(words)
	}
	sort.Strings(out)
	return out
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
		Mask:       p.Masks,
		MaskCoords: item.MaskCoords,
	}).To_DocumentAttribute())
	doc := mtproto.MakeTLDocument(&mtproto.Document{
		Id:            item.DocumentID,
		AccessHash:    item.AccessHash,
		FileReference: []byte{},
		Date:          stickerDate,
		MimeType:      item.MIME,
		Size2_INT32:   item.Size,
		Size2_INT64:   int64(item.Size),
		Size2:         int64(item.Size),
		DcId:          dcoptions.FileDC(),
		Attributes:    attrs,
	}).To_Document()
	if jpg := item.thumbJPEG(); len(jpg) > 0 {
		doc.Thumbs = []*mtproto.PhotoSize{
			mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{
				Type:  mtproto.PhotoSZMediumType,
				W:     thumbSide,
				H:     thumbSide,
				Size2: int32(len(jpg)),
			}).To_PhotoSize(),
		}
	}
	return doc
}
