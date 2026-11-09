package catalog

import (
	"bytes"
	"image/jpeg"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestBuiltinPacksInstalledWithThumbs(t *testing.T) {
	s := newService(builtinPacks())
	const user int64 = 7
	all := s.AllStickers(user, 0)
	for _, set := range all.GetSets() {
		if set.GetShortName() == ShortTwemoji || set.GetShortName() == ShortFluent {
			t.Fatalf("%s is still installed", set.GetShortName())
		}
	}
	got, err := s.GetSet(user, mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{
		ShortName: ShortTwemoji,
	}).To_InputStickerSet(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetDocuments()) == 0 {
		t.Fatal("sticker set has no documents")
	}
	doc := got.GetDocuments()[0]
	if doc.GetPredicateName() != mtproto.Predicate_document || doc.GetDcId() == 0 {
		t.Fatalf("document %#v", doc)
	}
	if len(doc.GetThumbs()) == 0 || doc.GetThumbs()[0].GetType() != mtproto.PhotoSZMediumType {
		t.Fatal("sticker document has no medium thumb")
	}
	jpg, mime, ok := s.ThumbFile(doc.GetId(), mtproto.PhotoSZMediumType)
	if !ok || mime != "image/jpeg" || len(jpg) == 0 {
		t.Fatal("thumb bytes are not a jpeg")
	}
	if _, err = jpeg.Decode(bytes.NewReader(jpg)); err != nil {
		t.Fatal(err)
	}
	body, fileMIME, ok := s.DocumentFile(doc.GetId())
	if !ok || fileMIME != "image/webp" || len(body) == 0 {
		t.Fatal("sticker file is not the webp")
	}
	recent := s.Recent(user, 0, false)
	if recent.GetPredicateName() != mtproto.Predicate_messages_recentStickers {
		t.Fatalf("recent %s", recent.GetPredicateName())
	}
	featured := s.Featured(user, 0, false)
	if featured.GetPredicateName() != mtproto.Predicate_messages_featuredStickers || len(featured.GetSets()) != 0 {
		t.Fatal("foreign seed packs are featured")
	}
}

func TestSendResolvesCatalogBeforeEmptyDocument(t *testing.T) {
	s := newService(builtinPacks())
	set, err := s.GetSet(0, mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{
		ShortName: ShortTwemoji,
	}).To_InputStickerSet(), 0)
	if err != nil || len(set.GetDocuments()) == 0 {
		t.Fatal(err)
	}
	src := set.GetDocuments()[0]
	in := mtproto.MakeTLInputDocument(&mtproto.InputDocument{
		Id:         src.GetId(),
		AccessHash: src.GetAccessHash(),
	}).To_InputDocument()
	empty := mtproto.MakeTLDocumentEmpty(&mtproto.Document{Id: src.GetId()}).To_Document()
	got, err := ResolveInputDocument(in, func() (*mtproto.Document, error) {
		t.Fatal("media table was consulted for a catalog sticker")
		return empty, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_document || got.GetMimeType() != "image/webp" || len(got.GetThumbs()) == 0 {
		t.Fatalf("sent document %#v", got)
	}
	var alt string
	for _, attr := range got.GetAttributes() {
		if attr.GetPredicateName() == mtproto.Predicate_documentAttributeSticker {
			alt = attr.GetAlt()
		}
	}
	if alt == "" {
		t.Fatal("sent sticker has no alt")
	}

	bad := mtproto.MakeTLInputDocument(&mtproto.InputDocument{
		Id:         src.GetId(),
		AccessHash: src.GetAccessHash() + 1,
	}).To_InputDocument()
	if _, err = ResolveInputDocument(bad, func() (*mtproto.Document, error) {
		t.Fatal("media table was consulted after a bad access hash")
		return nil, nil
	}); err != mtproto.ErrMediaInvalid {
		t.Fatalf("bad hash err %v", err)
	}

	missing := mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 42, AccessHash: 7}).To_InputDocument()
	_, found, err := s.CatalogDocument(42, 7)
	if err != nil || found {
		t.Fatalf("missing catalog doc found=%v err=%v", found, err)
	}
	if _, err = ResolveInputDocument(missing, func() (*mtproto.Document, error) {
		return empty, nil
	}); err != mtproto.ErrMediaInvalid {
		t.Fatalf("empty stored document err %v", err)
	}
	stored := mtproto.MakeTLDocument(&mtproto.Document{
		Id: 42, AccessHash: 7, MimeType: "image/webp", DcId: 1,
	}).To_Document()
	got, err = ResolveInputDocument(missing, func() (*mtproto.Document, error) {
		return stored, nil
	})
	if err != nil || got.GetId() != 42 {
		t.Fatalf("stored doc %v %v", got, err)
	}
}
