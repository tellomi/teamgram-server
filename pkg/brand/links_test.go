package brand

import "testing"

func TestRewrite(t *testing.T) {
	cases := map[string]string{
		"https://t.me/name":                    "https://tell.cc/name",
		"http://telegram.me/+Abcd":             "https://tell.cc/+Abcd",
		"see t.me/name and telegram.dog/c/1":   "see tell.cc/name and tell.cc/c/1",
		"https://telegram.org/privacy":         "https://tellomi.cn/privacy",
		"https://core.telegram.org/api/config": "https://core.tellomi.cn/api/config",
		"https://www.telegram.org/faq":         "https://tellomi.cn/faq",
		"sms@telegram.org":                     "sms@tellomi.cn",
		"https://tell.cc/already":              "https://tell.cc/already",
		"https://tellomi.cn/privacy":           "https://tellomi.cn/privacy",
	}
	for in, want := range cases {
		if got := Rewrite(in); got != want {
			t.Errorf("Rewrite(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInviteHash(t *testing.T) {
	if got := InviteHash("https://t.me/+abc"); got != "abc" {
		t.Fatal(got)
	}
	if got := InviteHash("https://tell.cc/+abc"); got != "abc" {
		t.Fatal(got)
	}
	if got := InviteHash("abc"); got != "abc" {
		t.Fatal(got)
	}
}
