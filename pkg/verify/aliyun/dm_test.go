package aliyun

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSingleSendMail(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Host, "127.0.0.1") && !strings.Contains(r.Host, "localhost") {
			t.Errorf("host %s", r.Host)
		}
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = w.Write([]byte(`{"Code":"OK","RequestId":"dm"}`))
	}))
	defer srv.Close()

	c := &DirectMailClient{
		AccessKeyID:     "id",
		AccessKeySecret: "secret",
		AccountName:     "noreply@mail.tellomi.cn",
		FromAlias:       "Tellomi",
		Endpoint:        srv.URL,
		HTTP:            srv.Client(),
	}
	if err := c.SendCode(context.Background(), "user@example.com", "654321"); err != nil {
		t.Fatal(err)
	}
	if got.Get("Action") != "SingleSendMail" || got.Get("Version") != "2015-11-23" {
		t.Fatalf("%v", got)
	}
	if got.Get("RegionId") != "cn-hangzhou" {
		t.Fatal(got.Get("RegionId"))
	}
	if got.Get("AccountName") != "noreply@mail.tellomi.cn" || got.Get("FromAlias") != "Tellomi" {
		t.Fatalf("from: %v", got)
	}
	if got.Get("ToAddress") != "user@example.com" || got.Get("AddressType") != "1" || got.Get("ReplyToAddress") != "false" {
		t.Fatalf("addr: %v", got)
	}
	if !strings.Contains(got.Get("TextBody"), "654321") || !strings.Contains(got.Get("TextBody"), "您的验证码为：") {
		t.Fatal(got.Get("TextBody"))
	}
	params := map[string]string{}
	for k, v := range got {
		if k == "Signature" || len(v) == 0 {
			continue
		}
		params[k] = v[0]
	}
	if Sign(params, "secret") != got.Get("Signature") {
		t.Fatal("signature mismatch")
	}
}

func TestDirectMailMissingAccount(t *testing.T) {
	c := &DirectMailClient{AccessKeyID: "id", AccessKeySecret: "secret"}
	if err := c.SendCode(context.Background(), "a@b.cn", "12345"); err == nil {
		t.Fatal("expected account error")
	}
}
