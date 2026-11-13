package minio_util

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// A GET whose first response byte never arrives used to sit for
// r2ResponseHeaderTimeout (5s). The S3 client then retried that wait, so
// upload.getFile for a hot-tier miss measured 6.5–8.9s. These bounds are
// the first-byte budget for one attempt and one retry. Puts are not
// wrapped: the migrator still uses the transport header timeout.
const (
	r2FirstByteTimeout = 2 * time.Second
	r2GetAttempts      = 2
	r2GetRetryGap      = 50 * time.Millisecond
)

// fastFirstByte retries GET and HEAD when the origin does not send a
// response header before r2FirstByteTimeout. The body of a successful
// response is not cut off by that timer. After the attempts are spent, a
// timeout is returned as context.DeadlineExceeded so the S3 client's own
// retry loop does not multiply the wait.
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

// roundOnce waits r2FirstByteTimeout for response headers. The timer is
// stopped once they arrive, so a large object can still stream.
func (f fastFirstByte) roundOnce(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	timer := time.AfterFunc(r2FirstByteTimeout, cancel)
	resp, err := f.baseRoundTrip(req.Clone(ctx))
	if !timer.Stop() {
		cancel()
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if err == nil {
			err = context.DeadlineExceeded
		}
		return nil, err
	}
	if err != nil {
		cancel()
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
