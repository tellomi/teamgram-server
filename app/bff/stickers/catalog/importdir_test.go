package catalog

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestPreparedPackSplitAndDryRun(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "panda")
	if err := os.Mkdir(pack, 0o755); err != nil {
		t.Fatal(err)
	}
	const n = maxStaticStickers + 1
	var stickers string
	webp := webpVP8X(512, 512)
	for i := 0; i < n; i++ {
		name := "s" + strconv.Itoa(i) + ".webp"
		if err := os.WriteFile(filepath.Join(pack, name), webp, 0o644); err != nil {
			t.Fatal(err)
		}
		if i > 0 {
			stickers += ",\n"
		}
		stickers += `{"file":"` + name + `","emoticon":"😂","keywords":{"zh-hans":["熊猫头","笑"],"en":["panda"]}}`
	}
	manifest := `{
  "title": "熊猫头",
  "short_name": "PandaHead",
  "type": "static",
  "featured": false,
  "source_url": "https://github.com/zhaoolee/ChineseBQB",
  "stickers": [` + stickers + `]
}`
	if err := os.WriteFile(filepath.Join(pack, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	opt := ImportOptions{Featured: true}
	packs, paths, err := ReadPrepared(root, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 2 || len(paths) != n {
		t.Fatalf("packs=%d files=%d", len(packs), len(paths))
	}
	if packs[0].Title != "熊猫头 1/2" || packs[1].Title != "熊猫头 2/2" {
		t.Fatalf("titles %q %q", packs[0].Title, packs[1].Title)
	}
	if packs[0].ShortName != "PandaHead" || packs[1].ShortName != "PandaHead2" {
		t.Fatalf("short names %s %s", packs[0].ShortName, packs[1].ShortName)
	}
	if !packs[0].Featured || packs[0].InstalledDefault || packs[0].Videos {
		t.Fatalf("flags featured=%t installed=%t video=%t", packs[0].Featured, packs[0].InstalledDefault, packs[0].Videos)
	}
	if len(packs[0].Items) != maxStaticStickers || len(packs[1].Items) != 1 {
		t.Fatalf("sizes %d %d", len(packs[0].Items), len(packs[1].Items))
	}
	if packs[0].Items[0].Emoticon != "😂" || len(packs[0].Items[0].LangKeywords["zh-hans"]) != 2 {
		t.Fatalf("item %#v", packs[0].Items[0])
	}
	again, _, err := ReadPrepared(root, opt)
	if err != nil {
		t.Fatal(err)
	}
	if again[0].ID != packs[0].ID || again[0].Items[0].DocumentID != packs[0].Items[0].DocumentID {
		t.Fatal("a second read changed pack or document ids")
	}
	if err = ImportPrepared("", root, ImportOptions{DryRun: true, Featured: true}); err != nil {
		t.Fatal(err)
	}

	bad := filepath.Join(root, "bad")
	if err = os.Mkdir(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(bad, "a.webp"), webp, 0o644); err != nil {
		t.Fatal(err)
	}
	escape := `{"title":"坏","short_name":"BadPack","type":"static","stickers":[{"file":"../panda/s0.webp","emoticon":"😂","keywords":["熊猫"]}]}`
	if err = os.WriteFile(filepath.Join(bad, manifestName), []byte(escape), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = ReadPrepared(bad, ImportOptions{}); err == nil {
		t.Fatal("path escape was accepted")
	}
}

func TestCustomEmojiManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.webp"), webpVP8X(512, 512), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "title": "开心表情",
  "short_name": "HappyEmoji",
  "type": "custom_emoji",
  "stickers": [{"file":"a.webp","emoticon":"🙂","keywords":{"zh-hans":["开心"]}}]
}`
	if err := os.WriteFile(filepath.Join(root, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	packs, _, err := ReadPrepared(root, ImportOptions{Featured: true, InstalledDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 || !packs[0].Emojis || packs[0].Kind != KindCustomEmoji || !packs[0].Featured || !packs[0].InstalledDefault {
		t.Fatalf("pack %#v", packs[0])
	}
	if !packs[0].Items[0].CustomEmoji || packs[0].Items[0].Emoticon != "🙂" {
		t.Fatalf("item %#v", packs[0].Items[0])
	}
}

func TestVideoManifest(t *testing.T) {
	root := t.TempDir()
	body := sampleWebM(1.5, "V_VP9", 512, 512)
	if err := os.WriteFile(filepath.Join(root, "clip.webm"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "title": "视频表情",
  "short_name": "VideoMeme",
  "type": "video",
  "installed_default": true,
  "stickers": [{"file":"clip.webm","emoticon":"🔥","keywords":"火,火焰"}]
}`
	if err := os.WriteFile(filepath.Join(root, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	packs, _, err := ReadPrepared(root, ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 || !packs[0].Videos || !packs[0].InstalledDefault || packs[0].Title != "视频表情" {
		t.Fatalf("pack %#v", packs[0])
	}
	if packs[0].Items[0].MIME != "video/webm" || len(packs[0].Items[0].Keywords) != 2 {
		t.Fatalf("item %#v", packs[0].Items[0])
	}
	over := sampleWebM(4, "V_VP9", 512, 512)
	if err = os.WriteFile(filepath.Join(root, "clip.webm"), over, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = ReadPrepared(root, ImportOptions{}); err == nil {
		t.Fatal("over-long video was accepted")
	}
}
