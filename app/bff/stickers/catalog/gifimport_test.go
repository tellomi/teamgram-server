package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestGifDirAndChineseSearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cat.mp4"), []byte("not-a-real-mp4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cat.jpg"), []byte{0xff, 0xd8, 0xff, 0xd9}, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{
	  "clips": [
	    {"file": "cat.mp4", "title": "猫咪", "keywords": ["猫", "可爱"], "width": 160, "height": 120}
	  ]
	}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	clips, err := ReadGifDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 1 || clips[0].clip.title != "猫咪" || len(clips[0].clip.keywords) != 2 || clips[0].clip.thumbPath == "" {
		t.Fatalf("%#v", clips)
	}
	if clips[0].clip.mime != "video/mp4" || clips[0].clip.objectPath == "" {
		t.Fatalf("storage %#v", clips[0].clip)
	}
	if err = ImportGifs("", dir, GifImportOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}

	builtin := BundledGifs(0)
	setGifCatalog([]gifClip{clips[0].clip})
	t.Cleanup(func() { setGifCatalog(nil) })
	saved := BundledGifs(0)
	if len(saved.GetGifs()) != len(builtin.GetGifs())+1 {
		t.Fatalf("saved %d builtin %d", len(saved.GetGifs()), len(builtin.GetGifs()))
	}
	if saved.GetHash() == builtin.GetHash() {
		t.Fatal("imported gif did not change the saved-gif hash")
	}
	hits := GifInlineResults("猫咪")
	if len(hits) != 1 || hits[0].GetDocument().GetId() != clips[0].clip.id {
		t.Fatalf("chinese search %#v", hits)
	}
	if len(GifInlineResults("可爱")) != 1 {
		t.Fatal("keyword 可爱 missed")
	}
	if len(GifInlineResults("red")) != 1 || len(GifInlineResults("purple")) != 1 {
		t.Fatal("builtin title search broke")
	}
	if len(GifInlineResults("")) != len(saved.GetGifs()) {
		t.Fatal("empty query dropped a gif")
	}
}

func TestGifDirRejectsBuiltinCode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "clip0.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadGifDir(dir); err == nil {
		t.Fatal("clip0 was accepted")
	}
}

func TestImportedGifDocumentIsAnimated(t *testing.T) {
	setGifCatalog([]gifClip{{
		id: gifID("cat"), access: gifAccess("cat"), code: "cat", title: "猫咪",
		keywords: []string{"猫"}, mime: "video/mp4", size: 12, w: 160, h: 120,
		duration: 1, fileName: "cat.mp4", objectPath: "1.dat", imported: true,
		poster: []byte{0xff, 0xd8},
	}})
	t.Cleanup(func() { setGifCatalog(nil) })
	doc := GifInlineResults("猫")[0].GetDocument()
	if doc.GetMimeType() != "video/mp4" || len(doc.GetThumbs()) == 0 || len(doc.GetVideoThumbs()) == 0 {
		t.Fatalf("%#v", doc)
	}
	var animated bool
	for _, attr := range doc.GetAttributes() {
		if attr.GetPredicateName() == mtproto.Predicate_documentAttributeAnimated {
			animated = true
		}
	}
	if !animated {
		t.Fatal("missing animated attribute")
	}
}
