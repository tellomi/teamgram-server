// Package aliyun signs Aliyun RPC (POP) requests with HMAC-SHA1.
package aliyun

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/url"
	"sort"
	"strings"
)

func percentEncode(s string) string {
	e := url.QueryEscape(s)
	e = strings.ReplaceAll(e, "+", "%20")
	e = strings.ReplaceAll(e, "*", "%2A")
	e = strings.ReplaceAll(e, "%7E", "~")
	return e
}

func canonicalQuery(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, percentEncode(k)+"="+percentEncode(params[k]))
	}
	return strings.Join(parts, "&")
}

// Sign returns the base64 HMAC-SHA1 POP signature for a POST RPC call.
// params must not already contain Signature.
func Sign(params map[string]string, accessKeySecret string) string {
	stringToSign := "POST&" + percentEncode("/") + "&" + percentEncode(canonicalQuery(params))
	mac := hmac.New(sha1.New, []byte(accessKeySecret+"&"))
	_, _ = mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// FormBody is the signed application/x-www-form-urlencoded POST body.
// params must not already contain Signature. The signature is added here.
func FormBody(params map[string]string, accessKeySecret string) string {
	signed := make(map[string]string, len(params)+1)
	for k, v := range params {
		if k == "Signature" {
			continue
		}
		signed[k] = v
	}
	signed["Signature"] = Sign(signed, accessKeySecret)
	return canonicalQuery(signed)
}
