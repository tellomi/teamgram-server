package catalog

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestManageStickersAndGifs(t *testing.T) {
	s := newService(builtinPacks())
	const user int64 = 42

	if err := s.DeleteSet(user, shortName(ShortTwemoji)); err != mtproto.ErrStickersetInvalid {
		t.Fatalf("official delete: %v", err)
	}
	if err := s.CheckShortName(ShortTwemoji); err != mtproto.ErrShortNameOccupied {
		t.Fatalf("occupied: %v", err)
	}
	if err := s.CheckShortName("ab"); err != mtproto.ErrShortNameInvalid {
		t.Fatalf("short: %v", err)
	}
	if err := s.CheckShortName("Hello"); err != nil {
		t.Fatal(err)
	}
	suggested := s.SuggestShortName("Hello World")
	if err := s.CheckShortName(suggested.GetShortName()); err != nil {
		t.Fatalf("suggest %s: %v", suggested.GetShortName(), err)
	}

	base, err := s.GetSet(user, shortName(ShortTwemoji), 0)
	if err != nil {
		t.Fatal(err)
	}
	docs := base.GetDocuments()
	if len(docs) < 4 {
		t.Fatal("twemoji is too small")
	}
	created, err := s.CreateSet(CreateInput{
		Self:      user,
		Title:     "Mine",
		ShortName: "MineSet",
		Stickers: []*mtproto.InputStickerSetItem{
			stickerInput(docs[0], "😀"),
			stickerInput(docs[1], "😁"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.GetSet().GetShortName() != "MineSet" || created.GetSet().GetCount() != 2 || created.GetSet().GetInstalledDate() == nil {
		t.Fatalf("created %#v", created.GetSet())
	}
	mine := s.MyStickers(user, 0, 10)
	if mine.GetCount() != 1 || len(mine.GetSets()) != 1 {
		t.Fatalf("mine %#v", mine)
	}
	page := s.MyStickers(user, mine.GetSets()[0].GetSet().GetId(), 10)
	if page.GetCount() != 1 || len(page.GetSets()) != 0 {
		t.Fatalf("page %#v", page)
	}

	masks, err := s.CreateSet(CreateInput{
		Self:      user,
		Title:     "Masks",
		ShortName: "MaskSet",
		Masks:     true,
		Stickers:  []*mtproto.InputStickerSetItem{stickerInput(docs[2], "😎")},
	})
	if err != nil {
		t.Fatal(err)
	}
	listed := s.MaskStickers(user, 0)
	if len(listed.GetSets()) != 1 || listed.GetSets()[0].GetShortName() != "MaskSet" {
		t.Fatalf("masks %#v", listed.GetSets())
	}
	if again := s.MaskStickers(user, listed.GetHash()); again.GetPredicateName() != mtproto.Predicate_messages_allStickersNotModified {
		t.Fatalf("mask hash %s", again.GetPredicateName())
	}
	if listedInAll(s.AllStickers(user, 0), "MaskSet") {
		t.Fatal("mask set leaked into getAllStickers")
	}

	setIn := mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{
		Id:         created.GetSet().GetId(),
		AccessHash: created.GetSet().GetAccessHash(),
	}).To_InputStickerSet()
	added, err := s.AddSticker(user, setIn, stickerInput(docs[2], "😂"))
	if err != nil {
		t.Fatal(err)
	}
	if added.GetSet().GetCount() != 3 {
		t.Fatalf("add count %d", added.GetSet().GetCount())
	}
	moved, err := s.ChangePosition(user, inputDoc(docs[0]), 2)
	if err != nil {
		t.Fatal(err)
	}
	if moved.GetDocuments()[2].GetId() != docs[0].GetId() {
		t.Fatalf("position %#v", idsOf(moved.GetDocuments()))
	}
	if _, err = s.ChangePosition(user, inputDoc(docs[0]), -1); err != mtproto.ErrStickerInvalid {
		t.Fatalf("bad position: %v", err)
	}
	replaced, err := s.ReplaceSticker(user, inputDoc(docs[1]), stickerInput(docs[3], "😄"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasDoc(replaced.GetDocuments(), docs[3].GetId()) || hasDoc(replaced.GetDocuments(), docs[1].GetId()) {
		t.Fatalf("replace %#v", idsOf(replaced.GetDocuments()))
	}
	renamed, err := s.RenameSet(user, setIn, "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.GetSet().GetTitle() != "Renamed" {
		t.Fatalf("title %s", renamed.GetSet().GetTitle())
	}
	if _, err = s.RenameSet(user, setIn, ""); err != mtproto.ErrPackTitleInvalid {
		t.Fatalf("empty title: %v", err)
	}

	if _, err = s.AttachedStickers(nil); err != mtproto.ErrMediaEmpty {
		t.Fatalf("empty media: %v", err)
	}
	photo := mtproto.MakeTLInputStickeredMediaPhoto(&mtproto.InputStickeredMedia{
		Id_INPUTPHOTO: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: 7, AccessHash: 1}).To_InputPhoto(),
	}).To_InputStickeredMedia()
	attached, err := s.AttachedStickers(photo)
	if err != nil || len(attached.Datas) != 0 {
		t.Fatalf("photo %#v %v", attached, err)
	}
	docMedia := mtproto.MakeTLInputStickeredMediaDocument(&mtproto.InputStickeredMedia{
		Id_INPUTDOCUMENT: inputDoc(docs[0]),
	}).To_InputStickeredMedia()
	attached, err = s.AttachedStickers(docMedia)
	if err != nil || len(attached.Datas) != 1 {
		t.Fatalf("document %#v %v", attached, err)
	}

	fluent := shortName(ShortFluent)
	if err = s.Toggle(user, false, true, false, []*mtproto.InputStickerSet{fluent}); err != nil {
		t.Fatal(err)
	}
	arch := s.ArchivedStickers(user, false, false, 0, 50)
	if arch.GetCount() != 1 || arch.GetSets()[0].GetSet().GetShortName() != ShortFluent {
		t.Fatalf("archived %#v", arch.GetSets())
	}
	if listedInAll(s.AllStickers(user, 0), ShortFluent) {
		t.Fatal("archived set is still installed in the panel")
	}
	if err = s.Toggle(user, false, false, true, []*mtproto.InputStickerSet{fluent}); err != nil {
		t.Fatal(err)
	}
	if !listedInAll(s.AllStickers(user, 0), ShortFluent) {
		t.Fatal("unarchive did not install Fluent")
	}

	before := s.AllStickers(user, 0).GetSets()
	order := make([]int64, len(before))
	for i, set := range before {
		order[i] = set.GetId()
	}
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	if err = s.Reorder(user, false, false, order); err != nil {
		t.Fatal(err)
	}
	after := s.AllStickers(user, 0).GetSets()
	if len(after) == 0 || after[0].GetId() != order[0] {
		t.Fatalf("reorder first %d want %d", after[0].GetId(), order[0])
	}

	featured := s.Featured(user, 0, false)
	if len(featured.GetUnread()) == 0 {
		t.Fatal("featured stickers started read")
	}
	if err = s.ReadFeatured(user, featured.GetUnread()); err != nil {
		t.Fatal(err)
	}
	read := s.Featured(user, 0, false)
	if len(read.GetUnread()) != 0 {
		t.Fatalf("unread %#v", read.GetUnread())
	}
	if read.GetHash() == featured.GetHash() {
		t.Fatal("reading featured stickers did not change the hash")
	}

	if err = s.DeleteSet(user, setIn); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetSet(user, setIn, 0); err != mtproto.ErrStickersetInvalid {
		t.Fatalf("deleted set: %v", err)
	}
	if s.MaskStickers(user, 0).GetSets()[0].GetId() != masks.GetSet().GetId() {
		t.Fatal("deleting a sticker set removed the mask set")
	}

	gif := stickerItem(9001, "gif", false)
	gif.MIME = "image/gif"
	gif.FileName = "loop.gif"
	webp := stickerItem(9002, "x", false)
	gs := newService([]*Pack{newPack("Gifs", "Gifs", KindSticker, false, false, false, false, "", "", "", []*Item{gif, webp})})
	if err = gs.SaveGif(user, inputItem(gif), false); err != nil {
		t.Fatal(err)
	}
	saved := gs.SavedGifs(user, 0)
	if len(saved.GetGifs()) != 1 || saved.GetGifs()[0].GetId() != gif.DocumentID {
		t.Fatalf("gifs %#v", saved.GetGifs())
	}
	if again := gs.SavedGifs(user, saved.GetHash()); again.GetPredicateName() != mtproto.Predicate_messages_savedGifsNotModified {
		t.Fatalf("gif hash %s", again.GetPredicateName())
	}
	if err = gs.SaveGif(user, inputItem(gif), true); err != nil {
		t.Fatal(err)
	}
	if len(gs.SavedGifs(user, 0).GetGifs()) != 0 {
		t.Fatal("unsave left a gif")
	}
	if err = gs.SaveGif(user, inputItem(webp), false); err != mtproto.ErrGifContentTypeInvalid {
		t.Fatalf("webp gif: %v", err)
	}
	if err = gs.SaveGif(user, mtproto.MakeTLInputDocument(&mtproto.InputDocument{}).To_InputDocument(), false); err != mtproto.ErrGifIdInvalid {
		t.Fatalf("missing gif: %v", err)
	}
}

func stickerInput(doc *mtproto.Document, emoji string) *mtproto.InputStickerSetItem {
	return mtproto.MakeTLInputStickerSetItem(&mtproto.InputStickerSetItem{
		Document: inputDoc(doc),
		Emoji:    emoji,
	}).To_InputStickerSetItem()
}

func inputDoc(doc *mtproto.Document) *mtproto.InputDocument {
	return mtproto.MakeTLInputDocument(&mtproto.InputDocument{
		Id:         doc.GetId(),
		AccessHash: doc.GetAccessHash(),
	}).To_InputDocument()
}

func inputItem(item *Item) *mtproto.InputDocument {
	return mtproto.MakeTLInputDocument(&mtproto.InputDocument{
		Id:         item.DocumentID,
		AccessHash: item.AccessHash,
	}).To_InputDocument()
}

func listedInAll(all *mtproto.Messages_AllStickers, short string) bool {
	for _, set := range all.GetSets() {
		if set.GetShortName() == short {
			return true
		}
	}
	return false
}

func hasDoc(docs []*mtproto.Document, id int64) bool {
	for _, doc := range docs {
		if doc.GetId() == id {
			return true
		}
	}
	return false
}

func idsOf(docs []*mtproto.Document) []int64 {
	out := make([]int64, len(docs))
	for i, doc := range docs {
		out[i] = doc.GetId()
	}
	return out
}
