package core

import (
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/teamgram/proto/mtproto"
)

func TestLoginCodeNoticeChinese(t *testing.T) {
	text, ents := loginCodeNotice("123456", "Tellomi")
	if strings.Contains(text, "Login code") || strings.Contains(text, "Telegram") {
		t.Fatalf("english or Telegram leftover: %s", text)
	}
	for _, want := range []string{
		"登录验证码：123456。请不要把此验证码告诉任何人，即使对方声称来自 Tellomi！",
		"此验证码可用于登录你的 Tellomi 账号。我们绝不会以其他理由向你索要它。",
		"如果你并未尝试在其他设备上登录，请忽略此消息。",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if len(ents) != 2 {
		t.Fatalf("entities %+v", ents)
	}
	if got := utf16Slice(text, ents[0]); got != "登录验证码：" {
		t.Fatalf("label span %q", got)
	}
	if got := utf16Slice(text, ents[1]); got != "不要" {
		t.Fatalf("warn span %q", got)
	}
	// A 6-digit code must not shift the warning onto the wrong characters.
	if ents[1].GetOffset() != int32(mtproto.UTF16Len("登录验证码：123456。请")) {
		t.Fatalf("warn offset %d", ents[1].GetOffset())
	}
}

func utf16Slice(s string, e *mtproto.MessageEntity) string {
	units := utf16.Encode([]rune(s))
	off := int(e.GetOffset())
	n := int(e.GetLength())
	if off < 0 || n < 0 || off+n > len(units) {
		return ""
	}
	return string(utf16.Decode(units[off : off+n]))
}
