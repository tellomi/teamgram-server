package aliyun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// RPCTimeout is the longest one Aliyun HTTP call may block an auth RPC.
// auth.sendCode used to call http.DefaultClient, which has no timeout, so a
// blackholed dysmsapi endpoint left the Android client spinning.
const RPCTimeout = 10 * time.Second

// rpcTimeout is RPCTimeout unless a test shortens it.
var rpcTimeout = RPCTimeout

// dialTimeout is how long we wait for TCP to dysmsapi. A dropped route
// fails here instead of sitting in SYN retry.
const dialTimeout = 5 * time.Second

var rpcTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	TLSHandshakeTimeout:   dialTimeout,
	ResponseHeaderTimeout: RPCTimeout,
	ExpectContinueTimeout: time.Second,
	ForceAttemptHTTP2:     true,
}

type rpcResponse struct {
	Code      string `json:"Code"`
	Message   string `json:"Message"`
	RequestID string `json:"RequestId"`
}

func nonce() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func postRPC(ctx context.Context, client *http.Client, endpoint, accessKeyID, accessKeySecret string, params map[string]string) error {
	if accessKeyID == "" || accessKeySecret == "" {
		return fmt.Errorf("aliyun access key is not configured")
	}
	client = httpClient(client)
	ctx, cancel := withRPCTimeout(ctx)
	defer cancel()

	params["Format"] = "JSON"
	params["AccessKeyId"] = accessKeyID
	params["SignatureMethod"] = "HMAC-SHA1"
	params["SignatureVersion"] = "1.0"
	params["SignatureNonce"] = nonce()
	params["Timestamp"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	params["Signature"] = Sign(params, accessKeySecret)

	body := canonicalQuery(params)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var parsed rpcResponse
	if jerr := json.Unmarshal(raw, &parsed); jerr != nil {
		return fmt.Errorf("aliyun http %d: %s", resp.StatusCode, truncate(raw))
	}
	if resp.StatusCode >= 300 || (parsed.Code != "" && !strings.EqualFold(parsed.Code, "OK")) {
		return fmt.Errorf("aliyun %s: %s (%s)", parsed.Code, parsed.Message, parsed.RequestID)
	}
	return nil
}

func httpClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	timeout := rpcTimeout
	if timeout <= 0 {
		timeout = RPCTimeout
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: rpcTransport,
	}
}

// withRPCTimeout caps ctx at rpcTimeout. A shorter caller deadline is kept.
func withRPCTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := rpcTimeout
	if timeout <= 0 {
		timeout = RPCTimeout
	}
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) <= timeout {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func truncate(b []byte) string {
	s := string(b)
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
