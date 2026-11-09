package enforcement

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/teamgram/teamgram-server/pkg/discoverauth"
)

// NetworkAddr is the docker-network listen address for credential recheck
// and content review. It is not published on the host.
const NetworkAddr = "0.0.0.0:8820"

// ServeNetwork listens for content-engine on the compose network.
// Accepted hosts are 0.0.0.0, ::, 127.0.0.1, and ::1. A specific public
// address is refused. The port is not published by compose.
func ServeNetwork(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	switch host {
	case "0.0.0.0", "::", "127.0.0.1", "::1":
	default:
		return errors.New("enforcement: listener must be 0.0.0.0 or loopback")
	}
	key, err := LoadServiceKey()
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/internal/v1/enforcement/credentials/check", CredentialHandler{
		Key:    key,
		Verify: networkSubjects(),
	})
	content := ContentHandler{Key: key}
	mux.Handle("/internal/v1/enforcement/content/review", content)
	mux.Handle("/internal/v1/enforcement/content/decision", content)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return srv.ListenAndServe()
}

type fnVerifier struct {
	fn func(context.Context, string, time.Time) (int64, error)
}

func (f fnVerifier) Subject(ctx context.Context, credential string, now time.Time) (int64, error) {
	if f.fn == nil {
		return 0, errors.New("enforcement: credential verifier is not configured")
	}
	return f.fn(ctx, credential, now)
}

func networkSubjects() SubjectVerifier {
	keys, err := discoverauth.LoadKeys()
	if err != nil {
		return nil
	}
	return fnVerifier{fn: func(_ context.Context, token string, now time.Time) (int64, error) {
		s := Current()
		if s == nil {
			return 0, errors.New("enforcement: store is not installed")
		}
		return FeedSubject(keys.Signing, s.UserForSubject, token, now)
	}}
}
