package aliyun

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
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

func TestSendSmsStalledPeerFailsFast(t *testing.T) {
	old := rpcTimeout
	rpcTimeout = 300 * time.Millisecond
	t.Cleanup(func() { rpcTimeout = old })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(io.Discard, c)
			}(conn)
		}
	}()

	c := &SMSClient{
		AccessKeyID:     "id",
		AccessKeySecret: "secret",
		SignName:        "重庆半格智能",
		TemplateCode:    "SMS_338570180",
		Endpoint:        "http://" + ln.Addr().String() + "/",
	}
	start := time.Now()
	err = c.SendCode(context.Background(), "13800138000", "123456")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("stalled Aliyun peer returned success")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("auth SMS wait was %s, want a fast error: %v", elapsed, err)
	}
}

func TestSendSmsMissingSignDoesNotDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	c := &SMSClient{
		AccessKeyID:     "id",
		AccessKeySecret: "secret",
		SignName:        "",
		TemplateCode:    "SMS_338570180",
		Endpoint:        "http://" + ln.Addr().String() + "/",
	}
	err = c.SendCode(context.Background(), "13800138000", "123456")
	if err == nil || !strings.Contains(err.Error(), "sign or template") {
		t.Fatal(err)
	}
	if tc, ok := ln.(*net.TCPListener); ok {
		_ = tc.SetDeadline(time.Now().Add(50 * time.Millisecond))
	}
	if conn, aerr := ln.Accept(); aerr == nil {
		conn.Close()
		t.Fatal("incomplete SMS config dialed Aliyun")
	}

	c.SignName = "重庆半格智能"
	c.TemplateCode = "  "
	err = c.SendCode(context.Background(), "13800138000", "123456")
	if err == nil || !strings.Contains(err.Error(), "sign or template") {
		t.Fatal(err)
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
