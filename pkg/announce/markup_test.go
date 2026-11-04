package announce

import (
	"os"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestParseLaunchNotice(t *testing.T) {
	raw, err := os.ReadFile("testdata/tellomi-ai-launch.md")
	if err != nil {
		t.Fatal(err)
	}
	text, ents := ParseMarkup(string(raw))
	if strings.Contains(text, "**") {
		t.Fatalf("markers left in %q", text)
	}
	if !strings.Contains(text, "• **") && !strings.Contains(text, "• 私聊提问") {
		t.Fatalf("bullet missing: %q", text)
	}
	if !strings.Contains(text, "• 私聊提问") {
		t.Fatalf("bullet text: %q", text)
	}
	want := []string{
		"Tellomi AI 正式上线",
		"Tellomi AI",
		"您可以这样使用：",
		"私聊提问",
		"发现页互动",
		"群聊助手",
		"温馨提示：",
	}
	if len(ents) != len(want) {
		t.Fatalf("bold spans %d, want %d: %+v", len(ents), len(want), ents)
	}
	units := utf16.Encode([]rune(text))
	for i, e := range ents {
		if e.Type != "bold" || e.Length < 1 || int(e.Offset+e.Length) > len(units) {
			t.Fatalf("entity %d %+v", i, e)
		}
		got := string(utf16.Decode(units[e.Offset : e.Offset+e.Length]))
		if got != want[i] {
			t.Fatalf("span %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestUnclosedMarkerStays(t *testing.T) {
	text, ents := ParseMarkup("keep **this")
	if text != "keep **this" || len(ents) != 0 {
		t.Fatalf("%q %+v", text, ents)
	}
}
