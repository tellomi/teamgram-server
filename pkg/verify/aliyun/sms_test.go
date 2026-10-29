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

func TestSendSmsSignsRequest(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Code":"OK","Message":"OK","RequestId":"req"}`))
	}))
	defer srv.Close()

	c := &SMSClient{
		AccessKeyID:     "id",
		AccessKeySecret: "secret",
		Region:          "cn-hangzhou",
		SignName:        "重庆半格智能",
		TemplateCode:    "SMS_338570180",
		Endpoint:        srv.URL,
		HTTP:            srv.Client(),
	}
	if err := c.SendCode(context.Background(), "13800138000", "123456"); err != nil {
		t.Fatal(err)
	}
	if got.Get("Action") != "SendSms" || got.Get("Version") != "2017-05-25" {
		t.Fatalf("action/version: %v", got)
	}
	if got.Get("PhoneNumbers") != "13800138000" || got.Get("SignName") != "重庆半格智能" {
		t.Fatalf("phone/sign: %v", got)
	}
	if got.Get("TemplateCode") != "SMS_338570180" || got.Get("TemplateParam") != `{"code":"123456"}` {
		t.Fatalf("template: %s %s", got.Get("TemplateCode"), got.Get("TemplateParam"))
	}
	if got.Get("RegionId") != "cn-hangzhou" || got.Get("Signature") == "" {
		t.Fatalf("region/signature: %v", got)
	}
	params := map[string]string{}
	for k, v := range got {
		if k == "Signature" || len(v) == 0 {
			continue
		}
		params[k] = v[0]
	}
	if Sign(params, "secret") != got.Get("Signature") {
		t.Fatal("request signature does not match")
	}
}

func TestSendSmsKeepsLeadingZeroAsString(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Code":"OK","Message":"OK","RequestId":"req"}`))
	}))
	defer srv.Close()

	c := &SMSClient{
		AccessKeyID:     "id",
		AccessKeySecret: "secret",
		SignName:        "s",
		TemplateCode:    "SMS_1",
		Endpoint:        srv.URL,
		HTTP:            srv.Client(),
	}
	if err := c.SendCode(context.Background(), "13800138000", "012345"); err != nil {
		t.Fatal(err)
	}
	if got.Get("TemplateParam") != `{"code":"012345"}` {
		t.Fatalf("template param %s", got.Get("TemplateParam"))
	}
}

func TestSendSmsRequiresKeys(t *testing.T) {
	c := &SMSClient{}
	err := c.SendCode(context.Background(), "13800138000", "12345")
	if err == nil || !strings.Contains(err.Error(), "access key") {
		t.Fatal(err)
	}
}

func TestSendSmsAliyunError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"Code":"isv.BUSINESS_LIMIT_CONTROL","Message":"limited","RequestId":"r"}`))
	}))
	defer srv.Close()
	c := &SMSClient{
		AccessKeyID:     "id",
		AccessKeySecret: "secret",
		SignName:        "s",
		TemplateCode:    "SMS_1",
		Endpoint:        srv.URL,
		HTTP:            srv.Client(),
	}
	err := c.SendCode(context.Background(), "13800138000", "12345")
	if err == nil || !strings.Contains(err.Error(), "isv.BUSINESS_LIMIT_CONTROL") {
		t.Fatal(err)
	}
}
