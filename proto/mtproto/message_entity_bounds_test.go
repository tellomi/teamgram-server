package mtproto

import (
	"testing"
	"unicode/utf16"
)

const douyinShare = "4.30 复制打开抖音，看看【潘潘不番的作品】那又怎样# 来长沙享梦田 # 美女自拍 # 夜生活... https://v.douyin.com/ZRf28esGQoE/ :2pm bnQ:/ J@v.Fh 01/15"

func entityPlain(s string, off, length int32) string {
	units := utf16.Encode([]rune(s))
	if off < 0 || length < 0 || int(off)+int(length) > len(units) {
		return ""
	}
	return string(utf16.Decode(units[off : off+length]))
}

func TestSanitizeDouyinEntities(t *testing.T) {
	url := "https://v.douyin.com/ZRf28esGQoE/"
	urlAt := utf16Index(douyinShare, url)
	if urlAt < 0 {
		t.Fatal("url missing from sample")
	}
	urlLen := int32(UTF16Len(url))

	bad := []*MessageEntity{
		MakeTLMessageEntityHashtag(&MessageEntity{Offset: utf16Index(douyinShare, "# "), Length: 0}).To_MessageEntity(),
		MakeTLMessageEntityUrl(&MessageEntity{Offset: urlAt, Length: urlLen}).To_MessageEntity(),
		MakeTLMessageEntityUrl(&MessageEntity{Offset: urlAt, Length: urlLen}).To_MessageEntity(),
		MakeTLMessageEntityBold(&MessageEntity{Offset: int32(UTF16Len(douyinShare)) + 4, Length: 2}).To_MessageEntity(),
		MakeTLMessageEntityItalic(&MessageEntity{Offset: -1, Length: 3}).To_MessageEntity(),
	}

	got := SanitizeMessageEntities(douyinShare, bad)
	if len(got) != 1 {
		t.Fatalf("entities = %#v, want the single url", describeEntities(douyinShare, got))
	}
	if got[0].GetPredicateName() != Predicate_messageEntityUrl {
		t.Fatalf("predicate = %s", got[0].GetPredicateName())
	}
	if entityPlain(douyinShare, got[0].Offset, got[0].Length) != url {
		t.Fatalf("url span = %q", entityPlain(douyinShare, got[0].Offset, got[0].Length))
	}
}

func TestFixDataDropsBadEntitiesOnStoredMessage(t *testing.T) {
	url := "https://v.douyin.com/ZRf28esGQoE/"
	urlAt := utf16Index(douyinShare, url)
	msg := MakeTLMessage(&Message{
		Message: douyinShare,
		Entities: []*MessageEntity{
			MakeTLMessageEntityHashtag(&MessageEntity{Offset: utf16Index(douyinShare, "# "), Length: 0}).To_MessageEntity(),
			MakeTLMessageEntityUrl(&MessageEntity{Offset: urlAt, Length: int32(UTF16Len(url))}).To_MessageEntity(),
			MakeTLMessageEntityUrl(&MessageEntity{Offset: urlAt, Length: int32(UTF16Len(url))}).To_MessageEntity(),
		},
	}).To_Message()

	msg.FixData()
	if n := countEntities(msg.GetEntities(), Predicate_messageEntityUrl); n != 1 {
		t.Fatalf("url entities = %d", n)
	}
	if n := countEntities(msg.GetEntities(), Predicate_messageEntityHashtag); n != 0 {
		t.Fatalf("hashtag entities = %d", n)
	}
	for _, entity := range msg.GetEntities() {
		if entity.GetLength() <= 0 {
			t.Fatalf("non-positive length: %#v", entity)
		}
		if int64(entity.Offset)+int64(entity.Length) > int64(UTF16Len(msg.GetMessage())) {
			t.Fatalf("out of range: %#v", entity)
		}
	}

	upd := MakeTLUpdateNewMessage(&Update{Message_MESSAGE: msg}).To_Update()
	upd.FixData()
	if n := countEntities(upd.GetMessage_MESSAGE().GetEntities(), Predicate_messageEntityUrl); n != 1 {
		t.Fatalf("update url entities = %d", n)
	}
}

func TestSanitizeSortsByOffset(t *testing.T) {
	text := "abcdef"
	got := SanitizeMessageEntities(text, []*MessageEntity{
		MakeTLMessageEntityBold(&MessageEntity{Offset: 3, Length: 1}).To_MessageEntity(),
		MakeTLMessageEntityItalic(&MessageEntity{Offset: 0, Length: 2}).To_MessageEntity(),
	})
	if len(got) != 2 || got[0].Offset != 0 || got[1].Offset != 3 {
		t.Fatalf("order = %#v", got)
	}
}

func utf16Index(s, sub string) int32 {
	units := utf16.Encode([]rune(s))
	needle := utf16.Encode([]rune(sub))
	if len(needle) == 0 || len(needle) > len(units) {
		return -1
	}
	for i := 0; i+len(needle) <= len(units); i++ {
		match := true
		for j := range needle {
			if units[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return int32(i)
		}
	}
	return -1
}

func countEntities(entities []*MessageEntity, predicate string) int {
	n := 0
	for _, entity := range entities {
		if entity.GetPredicateName() == predicate {
			n++
		}
	}
	return n
}

func describeEntities(text string, entities []*MessageEntity) []string {
	out := make([]string, 0, len(entities))
	for _, entity := range entities {
		out = append(out, entity.GetPredicateName()+" "+entityPlain(text, entity.Offset, entity.Length))
	}
	return out
}
