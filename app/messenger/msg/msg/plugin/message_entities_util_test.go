package plugin

import (
	"context"
	"testing"
	"unicode/utf16"

	"github.com/teamgram/proto/mtproto"
)

const douyinShare = "4.30 复制打开抖音，看看【潘潘不番的作品】那又怎样# 来长沙享梦田 # 美女自拍 # 夜生活... https://v.douyin.com/ZRf28esGQoE/ :2pm bnQ:/ J@v.Fh 01/15"

func TestRemakeMessageDouyinShare(t *testing.T) {
	const url = "https://v.douyin.com/ZRf28esGQoE/"
	urlAt := utf16Index(douyinShare, url)
	urlLen := int32(mtproto.UTF16Len(url))

	t.Run("plain", func(t *testing.T) {
		out := remake(douyinShare, nil, nil)
		assertSafeEntities(t, douyinShare, out.GetEntities())
		if n := countPred(out.GetEntities(), mtproto.Predicate_messageEntityHashtag); n != 0 {
			t.Fatalf("hashtags = %d (%v)", n, describe(douyinShare, out.GetEntities()))
		}
		if n := countPred(out.GetEntities(), mtproto.Predicate_messageEntityUrl); n != 1 {
			t.Fatalf("urls = %d (%v)", n, describe(douyinShare, out.GetEntities()))
		}
		got := findPred(out.GetEntities(), mtproto.Predicate_messageEntityUrl)
		if entityText(douyinShare, got.Offset, got.Length) != url {
			t.Fatalf("url span = %q off=%d len=%d", entityText(douyinShare, got.Offset, got.Length), got.Offset, got.Length)
		}
		if got.Offset != urlAt || got.Length != urlLen {
			t.Fatalf("url utf16 = %d+%d, want %d+%d", got.Offset, got.Length, urlAt, urlLen)
		}
	})

	t.Run("client url is not duplicated", func(t *testing.T) {
		client := mtproto.MakeTLMessageEntityUrl(&mtproto.MessageEntity{Offset: urlAt, Length: urlLen}).To_MessageEntity()
		out := remake(douyinShare, []*mtproto.MessageEntity{client, client}, nil)
		assertSafeEntities(t, douyinShare, out.GetEntities())
		if n := countPred(out.GetEntities(), mtproto.Predicate_messageEntityUrl); n != 1 {
			t.Fatalf("urls = %d (%v)", n, describe(douyinShare, out.GetEntities()))
		}
	})

	t.Run("zero length hashtag from client is dropped", func(t *testing.T) {
		bad := mtproto.MakeTLMessageEntityHashtag(&mtproto.MessageEntity{
			Offset: utf16Index(douyinShare, "# "),
			Length: 0,
		}).To_MessageEntity()
		out := remake(douyinShare, []*mtproto.MessageEntity{bad}, nil)
		assertSafeEntities(t, douyinShare, out.GetEntities())
		if n := countPred(out.GetEntities(), mtproto.Predicate_messageEntityHashtag); n != 0 {
			t.Fatalf("hashtags = %d", n)
		}
	})

	t.Run("client entities are not parsed again", func(t *testing.T) {
		bold := mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{
			Offset: 0,
			Length: 4,
		}).To_MessageEntity()
		bad := mtproto.MakeTLMessageEntityHashtag(&mtproto.MessageEntity{
			Offset: utf16Index(douyinShare, "# "),
			Length: 0,
		}).To_MessageEntity()
		out := remake(douyinShare, []*mtproto.MessageEntity{bold, bad, bad}, nil)
		assertSafeEntities(t, douyinShare, out.GetEntities())
		if n := countPred(out.GetEntities(), mtproto.Predicate_messageEntityUrl); n != 0 {
			t.Fatalf("server parsed a url even though the client sent entities")
		}
		if n := countPred(out.GetEntities(), mtproto.Predicate_messageEntityHashtag); n != 0 {
			t.Fatalf("zero-length hashtag kept")
		}
		if n := countPred(out.GetEntities(), mtproto.Predicate_messageEntityBold); n != 1 {
			t.Fatalf("bold = %d", n)
		}
	})
}

func TestHashtagRules(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"# ", nil},
		{"#", nil},
		{"怎样# 来长沙", nil},
		{"怎样#美女", nil},
		{"#123", nil},
		{"#_", nil},
		{"see #tag now", []string{"#tag"}},
		{"看 #美女自拍", []string{"#美女自拍"}},
		{"#1a", []string{"#1a"}},
		{"#ab@bot", []string{"#ab@bot"}},
		{"😀 #ab", []string{"#ab"}},
	}
	for _, tc := range cases {
		out := remake(tc.text, nil, nil)
		var got []string
		for _, entity := range out.GetEntities() {
			if entity.GetPredicateName() == mtproto.Predicate_messageEntityHashtag {
				got = append(got, entityText(tc.text, entity.Offset, entity.Length))
			}
		}
		if len(got) != len(tc.want) {
			t.Errorf("%q hashtags = %v, want %v", tc.text, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%q hashtag[%d] = %q, want %q", tc.text, i, got[i], tc.want[i])
			}
		}
	}

	emoji := remake("😀 #ab", nil, nil)
	tag := findPred(emoji.GetEntities(), mtproto.Predicate_messageEntityHashtag)
	if tag == nil || tag.Offset != 3 || tag.Length != 3 {
		t.Fatalf("emoji hashtag = %#v", tag)
	}
}

func TestCashtagAndBotCommand(t *testing.T) {
	out := remake("buy $TSLA now", nil, nil)
	cash := findPred(out.GetEntities(), mtproto.Predicate_messageEntityCashtag)
	if cash == nil || entityText("buy $TSLA now", cash.Offset, cash.Length) != "$TSLA" {
		t.Fatalf("cashtag = %#v", cash)
	}
	if findPred(remake("$ ", nil, nil).GetEntities(), mtproto.Predicate_messageEntityCashtag) != nil {
		t.Fatal("bare $ became a cashtag")
	}
	if findPred(remake("$tsla", nil, nil).GetEntities(), mtproto.Predicate_messageEntityCashtag) != nil {
		t.Fatal("lowercase ticker became a cashtag")
	}
	inch := findPred(remake("$1INCH", nil, nil).GetEntities(), mtproto.Predicate_messageEntityCashtag)
	if inch == nil || entityText("$1INCH", inch.Offset, inch.Length) != "$1INCH" {
		t.Fatalf("1INCH cashtag = %#v", inch)
	}

	hasBot := func() bool { return true }
	cmd := findPred(remake("/start", nil, hasBot).GetEntities(), mtproto.Predicate_messageEntityBotCommand)
	if cmd == nil || entityText("/start", cmd.Offset, cmd.Length) != "/start" {
		t.Fatalf("command = %#v", cmd)
	}
	if findPred(remake("/", nil, hasBot).GetEntities(), mtproto.Predicate_messageEntityBotCommand) != nil {
		t.Fatal("bare slash became a bot command")
	}
	if findPred(remake("x/start", nil, hasBot).GetEntities(), mtproto.Predicate_messageEntityBotCommand) != nil {
		t.Fatal("mid-word slash became a bot command")
	}
	if findPred(remake("/start", nil, nil).GetEntities(), mtproto.Predicate_messageEntityBotCommand) != nil {
		t.Fatal("bot command detected without a bot")
	}
}

func TestMentionLengthAndSort(t *testing.T) {
	out := remake("hi @telegram", nil, nil)
	mention := findPred(out.GetEntities(), mtproto.Predicate_messageEntityMention)
	if mention == nil || mention.Length <= 0 || entityText("hi @telegram", mention.Offset, mention.Length) != "@telegram" {
		t.Fatalf("mention = %#v", mention)
	}

	text := "4.30 https://t.me/telegram"
	plain := remake(text, nil, nil)
	assertSafeEntities(t, text, plain.GetEntities())
	if findPred(plain.GetEntities(), mtproto.Predicate_messageEntityUrl) == nil {
		t.Fatalf("plain text lost its url: %v", describe(text, plain.GetEntities()))
	}
	bold := mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{Offset: 0, Length: 4}).To_MessageEntity()
	withClient := remake(text, []*mtproto.MessageEntity{bold}, nil)
	if findPred(withClient.GetEntities(), mtproto.Predicate_messageEntityUrl) != nil {
		t.Fatalf("client entities were parsed again: %v", describe(text, withClient.GetEntities()))
	}
}

func remake(text string, entities []*mtproto.MessageEntity, hasBot func() bool) *mtproto.Message {
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Message:  text,
		Entities: entities,
	}).To_Message()
	return RemakeMessage(context.Background(), nil, msg, 1, true, hasBot)
}

func assertSafeEntities(t *testing.T, text string, entities []*mtproto.MessageEntity) {
	t.Helper()
	limit := mtproto.UTF16Len(text)
	var prev int32
	for i, entity := range entities {
		if entity.GetLength() <= 0 {
			t.Fatalf("entity %d length %d", i, entity.GetLength())
		}
		if entity.GetOffset() < 0 || int64(entity.GetOffset())+int64(entity.GetLength()) > int64(limit) {
			t.Fatalf("entity %d out of range off=%d len=%d text=%d", i, entity.GetOffset(), entity.GetLength(), limit)
		}
		if i > 0 && entity.GetOffset() < prev {
			t.Fatalf("entities not sorted at %d", i)
		}
		prev = entity.GetOffset()
		for j := 0; j < i; j++ {
			other := entities[j]
			if other.GetPredicateName() == entity.GetPredicateName() &&
				other.GetOffset() == entity.GetOffset() &&
				other.GetLength() == entity.GetLength() &&
				other.GetUrl() == entity.GetUrl() {
				t.Fatalf("duplicate %#v", entity)
			}
		}
	}
}

func entityText(s string, off, length int32) string {
	units := utf16.Encode([]rune(s))
	if off < 0 || length < 0 || int(off)+int(length) > len(units) {
		return ""
	}
	return string(utf16.Decode(units[off : off+length]))
}

func utf16Index(s, sub string) int32 {
	units := utf16.Encode([]rune(s))
	needle := utf16.Encode([]rune(sub))
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

func countPred(entities []*mtproto.MessageEntity, predicate string) int {
	n := 0
	for _, entity := range entities {
		if entity.GetPredicateName() == predicate {
			n++
		}
	}
	return n
}

func findPred(entities []*mtproto.MessageEntity, predicate string) *mtproto.MessageEntity {
	for _, entity := range entities {
		if entity.GetPredicateName() == predicate {
			return entity
		}
	}
	return nil
}

func describe(text string, entities []*mtproto.MessageEntity) []string {
	out := make([]string, 0, len(entities))
	for _, entity := range entities {
		out = append(out, entity.GetPredicateName()+":"+entityText(text, entity.Offset, entity.Length))
	}
	return out
}
