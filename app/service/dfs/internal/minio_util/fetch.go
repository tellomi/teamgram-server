package minio_util

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"
)

// A GET whose first response byte never arrives used to sit for
// r2ResponseHeaderTimeout (5s). The S3 client then retried that wait, so
// upload.getFile for a hot-tier miss measured 6.5–8.9s. These bounds are
// the header wait after the request has been written, plus one retry.
// Dial, CONNECT, and TLS are not included: Chengdu to R2 often spends
// 0.8–1.5s on a new TLS session. Puts are not wrapped and keep the
// transport's 5s ResponseHeaderTimeout.
const (
	r2FirstByteTimeout = 2 * time.Second
	r2GetAttempts      = 2
	r2GetRetryGap      = 50 * time.Millisecond
)

// fastFirstByte retries GET and HEAD when response headers do not arrive
// within r2FirstByteTimeout after the request is written. Connection
// setup is outside that budget. Once the first response byte arrives the
// timer stops, so a large or slow body is not cancelled. After the
// attempts are spent, a timeout is returned as context.DeadlineExceeded
// so the S3 client's own retry loop does not multiply the wait.
type fastFirstByte struct {
	base http.RoundTripper
}

func (f fastFirstByte) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("object storage request is empty")
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return f.baseRoundTrip(req)
	}
	var last error
	for attempt := 0; attempt < r2GetAttempts; attempt++ {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		if attempt > 0 {
			timer := time.NewTimer(r2GetRetryGap)
			select {
			case <-req.Context().Done():
				timer.Stop()
				return nil, req.Context().Err()
			case <-timer.C:
			}
		}
		resp, err := f.roundOnce(req)
		if err == nil {
			return resp, nil
		}
		last = err
		if !retryableFirstByte(err) {
			return nil, err
		}
	}
	if retryableFirstByte(last) {
		return nil, context.DeadlineExceeded
	}
	return nil, last
}

func (f fastFirstByte) baseRoundTrip(req *http.Request) (*http.Response, error) {
	if f.base == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	return f.base.RoundTrip(req)
}

// headerWait arms r2FirstByteTimeout only after WroteRequest. Dial, the
// CONNECT proxy, and TLS happen before that and do not consume the budget.
// GotFirstResponseByte disarms it so the body can stream.
type headerWait struct {
	mu      sync.Mutex
	timer   *time.Timer
	stopped bool
	cancel  context.CancelFunc
}

func (h *headerWait) arm() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped || h.timer != nil {
		return
	}
	h.timer = time.AfterFunc(r2FirstByteTimeout, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.stopped {
			return
		}
		h.cancel()
	})
}

func (h *headerWait) stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopped = true
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
}

// roundOnce waits r2FirstByteTimeout for response headers after the
// request is written. The timer is stopped on the first response byte, so
// a large object can still stream.
func (f fastFirstByte) roundOnce(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	wait := &headerWait{cancel: cancel}
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				wait.arm()
			}
		},
		GotFirstResponseByte: func() {
			wait.stop()
		},
	})
	resp, err := f.baseRoundTrip(req.Clone(ctx))
	wait.stop()
	if err != nil {
		cancel()
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return nil, err
	}
	resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func retryableFirstByte(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded")
}
