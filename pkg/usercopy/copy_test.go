package usercopy

import (
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/langpack"
)

func TestEveryLanguageHasServerCopy(t *testing.T) {
	when := time.Date(2026, 11, 7, 2, 30, 0, 0, time.UTC)
	stamp := "2026-11-07 10:30"
	enUpdate := AppUpdate("en")
	enInvite := Invite("en", "https://tell.cc")
	enSupport := Support("en", "")
	enDelete := AccountDeletionPending("en")
	if enUpdate != "A new version of Tellomi is available." {
		t.Fatal(enUpdate)
	}
	if AppUpdate("zh-CN") != "发现新版本 Tellomi，建议更新。" {
		t.Fatal(AppUpdate("zh-CN"))
	}
	if AppUpdate("zh-TW") != "發現新版本 Tellomi，建議更新。" {
		t.Fatal(AppUpdate("zh-TW"))
	}
	if AppUpdate("classic-zh-cn") != AppUpdate("zh-hans") || AppUpdate("taiwan") != AppUpdate("zh-hant") {
		t.Fatal("legacy chinese packs")
	}
	if AppUpdate("no-such") != enUpdate || AppUpdate("") != enUpdate {
		t.Fatal("fallback")
	}
	if Invite("zh", "https://tell.cc") == enInvite || !strings.Contains(Invite("zh-TW", "https://tell.cc"), "來加入我") {
		t.Fatal(Invite("zh", "https://tell.cc"))
	}
	if Support("en", "Ada") != "Ada" || Support("zh", "") != "Tellomi 支持" {
		t.Fatal(Support("zh", ""))
	}
	if AccountDeletionPending("zh-hans") != "你的 Tellomi 账号将在 7 天后删除。在此之前重新登录即可取消。" {
		t.Fatal(AccountDeletionPending("zh-hans"))
	}
	if _, _, ok := EmailCode("", "654321"); ok {
		t.Fatal("empty lang must keep the sender's previous text")
	}
	zhSub, zhBody, ok := EmailCode("zh-hans", "654321")
	if !ok || zhSub != "Tellomi 验证码" || zhBody != "您的验证码为：654321，请勿泄露于他人！" {
		t.Fatalf("%q %q", zhSub, zhBody)
	}
	enSub, enBody, ok := EmailCode("en-US", "654321")
	if !ok || !strings.Contains(enSub, "Tellomi") || enBody != "Your verification code is 654321. Do not share it." {
		t.Fatalf("%q %q", enSub, enBody)
	}

	for _, lang := range langpack.OfficialLanguages {
		code := lang.Code
		update := AppUpdate(code)
		if update == "" || strings.Contains(update, "%") {
			t.Fatalf("update %s %q", code, update)
		}
		if code != "en" && update == enUpdate {
			t.Fatalf("update %s fell back to English", code)
		}
		inv := Invite(code, "https://tell.cc")
		if !strings.HasSuffix(inv, "https://tell.cc") || strings.Contains(inv, "%") {
			t.Fatalf("invite %s %q", code, inv)
		}
		if code != "en" && inv == enInvite {
			t.Fatalf("invite %s fell back to English", code)
		}
		support := Support(code, "")
		if support == "" || strings.Contains(support, "%") {
			t.Fatalf("support %s %q", code, support)
		}
		if code != "en" && support == enSupport {
			t.Fatalf("support %s fell back to English", code)
		}
		deletion := AccountDeletionPending(code)
		if deletion == "" || strings.Contains(deletion, "%") {
			t.Fatalf("deletion %s %q", code, deletion)
		}
		if code != "en" && deletion == enDelete {
			t.Fatalf("deletion %s fell back to English", code)
		}
		subject, body, ok := EmailCode(code, "654321")
		if !ok || subject == "" || strings.Count(body, "654321") != 1 || strings.Contains(body, "%") {
			t.Fatalf("email %s %q %q", code, subject, body)
		}
		assertLoginCode(t, code)
		assertLoginAlert(t, code, stamp, when)
		premium := Premium(code)
		if premium == "" || strings.Contains(premium, "%") {
			t.Fatalf("premium %s %q", code, premium)
		}
		if code != "en" && premium == Premium("en") {
			t.Fatalf("premium %s fell back to English", code)
		}
	}
	assertAlertPart(t, "ja-JP", when, "新しいログイン")
	assertAlertPart(t, "zh-Hant-HK", when, "新裝置登入提醒")
	assertAlertPart(t, "zh-rTW", when, "新裝置登入提醒")
	assertAlertPart(t, "zh-Hans-CN", when, "新设备登录提醒")
	unknown, _ := LoginAlert("no-such-lang", "Pixel 8", when)
	if !strings.Contains(unknown, "New login alert") {
		t.Fatal(unknown)
	}
}

func assertAlertPart(t *testing.T, lang string, when time.Time, part string) {
	t.Helper()
	text, _ := LoginAlert(lang, "Pixel 8", when)
	if !strings.Contains(text, part) {
		t.Fatalf("%s alert %q missing %q", lang, text, part)
	}
	if strings.Contains(text, "New login alert") {
		t.Fatalf("%s fell back to English: %q", lang, text)
	}
}

func TestLoginCodeSimplifiedChinese(t *testing.T) {
	text, ents := LoginCode("zh-hans", "123456", "Tellomi")
	for _, want := range []string{
		"登录验证码：123456。请不要把此验证码告诉任何人，即使对方声称来自 Tellomi！",
		"此验证码可用于登录你的 Tellomi 账号。我们绝不会以其他理由向你索要它。",
		"如果你并未尝试在其他设备上登录，请忽略此消息。",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if strings.Contains(text, "Login code") {
		t.Fatal(text)
	}
	if got := span(text, ents[2]); got != "不要" {
		t.Fatal(got)
	}
}

func TestLoginAlertSimplifiedChinese(t *testing.T) {
	when := time.Date(2026, 11, 7, 2, 30, 0, 0, time.UTC)
	text, ents := LoginAlert("zh-hans", " iPhone 16 ", when)
	if !strings.Contains(text, "新设备登录提醒") || !strings.Contains(text, "iPhone 16") || !strings.Contains(text, "2026-11-07 10:30") || !strings.Contains(text, "北京时间") {
		t.Fatal(text)
	}
	if span(text, ents[0]) != "新设备登录提醒" {
		t.Fatal(span(text, ents[0]))
	}
	empty, _ := LoginAlert("zh-hans", " ", when)
	if !strings.Contains(empty, "未知设备") {
		t.Fatal(empty)
	}
	en, _ := LoginAlert("en", "Pixel 8", when)
	if !strings.Contains(en, "New login alert") || !strings.Contains(en, "Beijing time") || !strings.Contains(en, "Pixel 8") {
		t.Fatal(en)
	}
	if strings.Contains(en, "验证码") || strings.Contains(strings.ToLower(en), "token") {
		t.Fatal(en)
	}
}

func assertLoginCode(t *testing.T, lang string) {
	t.Helper()
	text, ents := LoginCode(lang, "123456", "Tellomi")
	if strings.Contains(text, "%!") || strings.Contains(text, "%s") || !strings.Contains(text, "Tellomi") {
		t.Fatalf("%s %q", lang, text)
	}
	if len(ents) != 3 {
		t.Fatalf("%s entities %d", lang, len(ents))
	}
	if ents[0].GetPredicateName() != mtproto.Predicate_messageEntityBold || span(text, ents[0]) == "" {
		t.Fatalf("%s label", lang)
	}
	if ents[1].GetPredicateName() != mtproto.Predicate_messageEntitySpoiler || span(text, ents[1]) != "123456" {
		t.Fatalf("%s code span %q", lang, span(text, ents[1]))
	}
	if ents[2].GetPredicateName() != mtproto.Predicate_messageEntityBold || span(text, ents[2]) == "" {
		t.Fatalf("%s warn", lang)
	}
}

func assertLoginAlert(t *testing.T, lang, stamp string, when time.Time) {
	t.Helper()
	text, ents := LoginAlert(lang, "Pixel 8", when)
	if strings.Contains(text, "%!") || strings.Contains(text, "%s") || !strings.Contains(text, "Pixel 8") || !strings.Contains(text, stamp) {
		t.Fatalf("%s %q", lang, text)
	}
	if len(ents) == 0 || ents[0].GetPredicateName() != mtproto.Predicate_messageEntityBold || span(text, ents[0]) == "" {
		t.Fatalf("%s title", lang)
	}
	empty, _ := LoginAlert(lang, "", when)
	if strings.Contains(empty, "Pixel 8") || strings.TrimSpace(empty) == "" {
		t.Fatalf("%s empty device %q", lang, empty)
	}
}

func span(s string, e *mtproto.MessageEntity) string {
	units := utf16.Encode([]rune(s))
	off := int(e.GetOffset())
	n := int(e.GetLength())
	if off < 0 || n < 0 || off+n > len(units) {
		return ""
	}
	return string(utf16.Decode(units[off : off+n]))
}
