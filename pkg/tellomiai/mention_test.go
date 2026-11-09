package tellomiai

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestStripMention(t *testing.T) {
	off := utf16.Encode([]rune("😀"))
	text := "😀@Tellomi 你好"
	got, ok := StripMention(text, []Mention{{
		Kind:   MentionUsername,
		Offset: len(off),
		Length: len([]rune("@Tellomi")),
	}}, false)
	if !ok || got != "😀 你好" {
		t.Fatalf("username mention: %q ok=%v", got, ok)
	}

	name := "Tellomi AI 在吗"
	nameOff := len(utf16.Encode([]rune("Tellomi AI")))
	got, ok = StripMention(name, []Mention{{
		Kind: MentionName, Offset: 0, Length: nameOff, UserID: AccountID,
	}}, false)
	if !ok || got != "在吗" {
		t.Fatalf("mention name: %q ok=%v", got, ok)
	}

	got, ok = StripMention("@other 你好", []Mention{{
		Kind: MentionUsername, Offset: 0, Length: len([]rune("@other")), UserID: 8,
	}}, false)
	if ok || got != "@other 你好" {
		t.Fatalf("other user: %q ok=%v", got, ok)
	}

	got, ok = StripMention("see @tellomi, hi", nil, true)
	if !ok || got != "see , hi" {
		t.Fatalf("plain: %q ok=%v", got, ok)
	}
	if _, ok = StripMention("see @tellomi_ai", nil, true); ok {
		t.Fatal("a longer username is not @tellomi")
	}
	if _, ok = StripMention("mail@tellomi hi", nil, true); ok {
		t.Fatal("an address is not a mention")
	}
	got, ok = StripMention("hello @tellomi", []Mention{{
		Kind: MentionUsername, Offset: 0, Length: 5, UserID: 8,
	}}, false)
	if ok || strings.Contains(got, "AI 生成") {
		t.Fatalf("entities present and not tellomi: %q ok=%v", got, ok)
	}
}

func TestGroupPromptOnlyThoseTwoTexts(t *testing.T) {
	if GroupPrompt("  你好 ", "") != "你好" {
		t.Fatal(GroupPrompt("  你好 ", ""))
	}
	if GroupPrompt("", " 上一句 ") != "上一句" {
		t.Fatal("reply only")
	}
	if GroupPrompt("你好", "上一句") != "上一句\n\n你好" {
		t.Fatal(GroupPrompt("你好", "上一句"))
	}
	if GroupPrompt("  ", " ") != "" {
		t.Fatal("empty")
	}
}
