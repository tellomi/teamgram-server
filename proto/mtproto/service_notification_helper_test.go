package mtproto

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestMakeTextAndMessageEntitiesUTF16(t *testing.T) {
	text, ents := MakeTextAndMessageEntities([]MessageBuildEntry{
		{Param: "登录验证码：", EntityType: Predicate_messageEntityBold},
		{Text: "123456。请", Param: "不要", EntityType: Predicate_messageEntityBold},
	})
	if len(ents) != 2 {
		t.Fatalf("entities %+v", ents)
	}
	if ents[0].GetOffset() != 0 || ents[0].GetLength() != int32(UTF16Len("登录验证码：")) {
		t.Fatalf("label entity off=%d len=%d", ents[0].GetOffset(), ents[0].GetLength())
	}
	if got := entityPlain(text, ents[0].GetOffset(), ents[0].GetLength()); got != "登录验证码：" {
		t.Fatalf("label span %q", got)
	}
	if got := entityPlain(text, ents[1].GetOffset(), ents[1].GetLength()); got != "不要" {
		t.Fatalf("warn span %q", got)
	}
	if n := UTF16Len(text); int(ents[1].GetOffset()+ents[1].GetLength()) > n {
		t.Fatalf("entity past text %d", n)
	}
}

func TestSignInServiceNotificationChinese(t *testing.T) {
	user := MakeTLUser(&User{
		FirstName: wrapperspb.String("小明"),
	}).To_User()
	n := MakeSignInServiceNotification(user, 42, "Android", "中国香港", "1.2.3.4")
	text := n.GetMessage_STRING()
	if strings.Contains(text, "New login") || strings.Contains(text, "Settings >") || strings.Contains(text, "Telegram") {
		t.Fatalf("english or Telegram leftover: %s", text)
	}
	for _, want := range []string{
		"新登录。",
		"亲爱的 小明",
		"在一台新设备上登录",
		"设备：Android",
		"位置：中国香港（IP = 1.2.3.4）",
		"设置 > 设备",
		"隐私与安全 > 活动会话",
		"如果这不是你本人",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	wantSpan := map[string]string{
		Predicate_messageEntityBold:    "",
		Predicate_messageEntityTextUrl: "1.2.3.4",
	}
	var bold []string
	for _, e := range n.GetEntities() {
		span := entityPlain(text, e.GetOffset(), e.GetLength())
		if span == "" {
			t.Fatalf("entity out of range %+v text=%d", e, UTF16Len(text))
		}
		if e.GetPredicateName() == Predicate_messageEntityTextUrl {
			if span != "1.2.3.4" {
				t.Fatalf("ip span %q", span)
			}
			wantSpan[Predicate_messageEntityTextUrl] = span
		}
		if e.GetPredicateName() == Predicate_messageEntityBold {
			bold = append(bold, span)
		}
	}
	if wantSpan[Predicate_messageEntityTextUrl] != "1.2.3.4" {
		t.Fatal("missing ip entity")
	}
	got := strings.Join(bold, "|")
	if got != "新登录。|设置 > 设备|隐私与安全 > 活动会话" {
		t.Fatalf("bold spans %q", got)
	}
}
