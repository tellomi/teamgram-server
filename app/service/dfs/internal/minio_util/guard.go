package minio_util

import (
	"fmt"
	"net/http"
	"strings"
)

// guardTransport rejects ListBuckets and MakeBucket. Credentials for this
// deployment can only touch existing buckets named tellomi-*, and those
// calls fail closed (or would create a bucket the token cannot create).
type guardTransport struct {
	base http.RoundTripper
}

func (g guardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("object storage request is empty")
	}
	if isListBucketsRequest(req) {
		return nil, fmt.Errorf("refusing ListBuckets (%s %s); buckets must already exist", req.Method, req.URL.RequestURI())
	}
	if isMakeBucketRequest(req) {
		return nil, fmt.Errorf("refusing MakeBucket (%s %s); buckets must already exist", req.Method, req.URL.RequestURI())
	}
	base := g.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func isListBucketsRequest(req *http.Request) bool {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return false
	}
	p := req.URL.EscapedPath()
	return p == "" || p == "/"
}

func isMakeBucketRequest(req *http.Request) bool {
	if req.Method != http.MethodPut {
		return false
	}
	if len(req.URL.Query()) > 0 {
		return false
	}
	trimmed := strings.Trim(req.URL.EscapedPath(), "/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return false
	}
	return true
}
