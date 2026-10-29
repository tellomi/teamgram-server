package aliyun

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"testing"
)

func TestPercentEncode(t *testing.T) {
	if got := percentEncode("a b*~"); got != "a%20b%2A~" {
		t.Fatal(got)
	}
}

func TestSignMatchesIndependentHMAC(t *testing.T) {
	params := map[string]string{
		"AccessKeyId":      "id",
		"Action":           "SendSms",
		"Format":           "JSON",
		"PhoneNumbers":     "13800138000",
		"RegionId":         "cn-hangzhou",
		"SignName":         "重庆半格智能",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   "nonce",
		"SignatureVersion": "1.0",
		"TemplateCode":     "SMS_338570180",
		"TemplateParam":    `{"code":"123456"}`,
		"Timestamp":        "2026-01-01T00:00:00Z",
		"Version":          "2017-05-25",
	}
	secret := "secret"
	got := Sign(params, secret)

	stringToSign := "POST&" + percentEncode("/") + "&" + percentEncode(canonicalQuery(params))
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	_, _ = mac.Write([]byte(stringToSign))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if got == "" {
		t.Fatal("empty signature")
	}
	// Signature depends on every parameter, including the template value.
	params["TemplateParam"] = `{"code":"000000"}`
	if Sign(params, secret) == got {
		t.Fatal("signature did not change with the code")
	}
}
