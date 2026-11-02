package e2e

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/crypto"
	"github.com/gotd/td/exchange"
	gotdmt "github.com/gotd/td/mtproto"
	"github.com/gotd/td/mtproxy"
	"github.com/gotd/td/mtproxy/obfuscated2"
	"github.com/gotd/td/proto/codec"
	"github.com/gotd/td/tgerr"
	"github.com/gotd/td/transport"
	"github.com/teamgram/proto/mtproto"
)

// Profile is the initConnection device block official clients send.
type Profile struct {
	DeviceModel    string
	SystemVersion  string
	AppVersion     string
	SystemLangCode string
	LangPack       string
	LangCode       string
}

// ProfileIOS matches a current Telegram iOS initConnection.
var ProfileIOS = Profile{
	DeviceModel:    "iPhone 16",
	SystemVersion:  "18.5",
	AppVersion:     "11.12.0",
	SystemLangCode: "en",
	LangPack:       "ios",
	LangCode:       "en",
}

// ProfileAndroid matches a current Telegram Android initConnection.
var ProfileAndroid = Profile{
	DeviceModel:    "Samsung SM-S928B",
	SystemVersion:  "SDK 35",
	AppVersion:     "11.12.0",
	SystemLangCode: "en",
	LangPack:       "android",
	LangCode:       "en",
}

// Client is one MTProto session. gotd performs the transport and key
// exchange. Payloads are layer 232 objects from the server schema, wrapped
// in invokeWithLayer + initConnection on every call.
type Client struct {
	cfg     Config
	profile Profile
	conn    *gotdmt.Conn
	cancel  context.CancelFunc
	done    chan struct{}

	handler *sessionHandler
}

type sessionHandler struct {
	mu   sync.Mutex
	sess gotdmt.Session
	ok   bool
}

func (h *sessionHandler) OnMessage(*bin.Buffer) error { return nil }

func (h *sessionHandler) OnSession(s gotdmt.Session) error {
	h.mu.Lock()
	h.sess = s
	h.ok = true
	h.mu.Unlock()
	return nil
}

func (h *sessionHandler) Session() (gotdmt.Session, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sess, h.ok
}

// Dial opens an obfuscated padded-intermediate connection, the transport
// Telegram iOS and Android use, and completes the DH exchange with the
// server's RSA key.
func Dial(ctx context.Context, cfg Config, profile Profile) (*Client, error) {
	return dial(ctx, cfg, profile, gotdmt.Session{})
}

// DialSession reconnects with a previously issued auth key.
func DialSession(ctx context.Context, cfg Config, profile Profile, sess gotdmt.Session) (*Client, error) {
	return dial(ctx, cfg, profile, sess)
}

func dial(ctx context.Context, cfg Config, profile Profile, sess gotdmt.Session) (*Client, error) {
	pub, err := cfg.PublicKey()
	if err != nil {
		return nil, err
	}
	h := &sessionHandler{}
	opts := gotdmt.Options{
		DC:                cfg.DC,
		PublicKeys:        []exchange.PublicKey{pub},
		Handler:           h,
		MaxRetries:        2,
		RetryInterval:     time.Second,
		CompressThreshold: -1,
		RequestTimeout: func(uint32) time.Duration {
			return 20 * time.Second
		},
		AckInterval:  time.Second,
		PingInterval: time.Minute,
		Random:       crypto.DefaultRand(),
	}
	if !sess.Key.Value.Zero() {
		opts.Key = sess.Key
		opts.Salt = sess.Salt
	}
	dc := cfg.DC
	addr := cfg.Addr
	conn := gotdmt.New(func(ctx context.Context) (transport.Conn, error) {
		return dialObfuscated(ctx, addr, dc)
	}, opts)

	runCtx, cancel := context.WithCancel(context.Background())
	c := &Client{
		cfg:     cfg,
		profile: profile,
		conn:    conn,
		cancel:  cancel,
		done:    make(chan struct{}),
		handler: h,
	}
	errc := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		defer close(c.done)
		errc <- conn.Run(runCtx, func(ctx context.Context) error {
			close(ready)
			<-ctx.Done()
			return nil
		})
	}()
	select {
	case <-ready:
		return c, nil
	case err := <-errc:
		cancel()
		if err == nil {
			err = fmt.Errorf("connection closed before it was ready")
		}
		return nil, err
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	}
}

func (c *Client) Close() {
	c.cancel()
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
	}
}

func (c *Client) SetProfile(p Profile) { c.profile = p }

func (c *Client) Session() (gotdmt.Session, bool) { return c.handler.Session() }

// Invoke sends one RPC and decodes the layer 232 result.
func (c *Client) Invoke(ctx context.Context, req mtproto.TLObject) (mtproto.TLObject, error) {
	raw, err := encodeTL(req)
	if err != nil {
		return nil, err
	}
	wrapped, err := wrapInit(c.cfg, c.profile, raw)
	if err != nil {
		return nil, err
	}
	var out captured
	if err := c.conn.Invoke(ctx, rawTL(wrapped), &out); err != nil {
		return nil, err
	}
	return out.obj, nil
}

// dialObfuscated speaks the obfuscated2 + padded intermediate transport.
// The protocol tag 0xdddddddd is endian-independent. The DC id is written
// little-endian by gotd and read big-endian by gnetway; any non-zero id is
// accepted, and DC 1 stays non-zero after that swap.
func dialObfuscated(ctx context.Context, addr string, dc int) (transport.Conn, error) {
	var nd net.Dialer
	raw, err := nd.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	// The obfuscated header says padded intermediate, which is what iOS and
	// Android send. Frames themselves are plain intermediate: 4-byte length
	// plus a 4-aligned payload. gnetway rejects a padded length that is not
	// a multiple of 4, and a zero-padding intermediate frame is a valid
	// padded-intermediate frame.
	obfs := obfuscated2.NewObfuscated2(rand.Reader, raw)
	if err := obfs.Handshake(codec.PaddedIntermediate{}.ObfuscatedTag(), dc, mtproxy.Secret{}); err != nil {
		_ = raw.Close()
		return nil, err
	}
	proto := transport.NewProtocol(func() transport.Codec {
		return codec.NoHeader{Codec: codec.Intermediate{}}
	})
	conn, err := proto.Handshake(&rwConn{Conn: raw, rw: obfs})
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	return conn, nil
}

type rwConn struct {
	net.Conn
	rw interface {
		Read([]byte) (int, error)
		Write([]byte) (int, error)
	}
}

func (c *rwConn) Read(p []byte) (int, error)  { return c.rw.Read(p) }
func (c *rwConn) Write(p []byte) (int, error) { return c.rw.Write(p) }

// RPCError is a decoded MTProto rpc_error.
type RPCError struct {
	Code    int
	Message string
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc %d %s", e.Code, e.Message)
}

func asRPCError(err error) (*RPCError, bool) {
	var tg *tgerr.Error
	if !errors.As(err, &tg) {
		return nil, false
	}
	return &RPCError{Code: tg.Code, Message: tg.Message}, true
}

// serverError reports errors that leave official clients on "Updating":
// HTTP-style 500, timeouts, and transport failures. A 4xx rpc_error is not
// one of those.
func serverError(err error) bool {
	if err == nil {
		return false
	}
	if rpc, ok := asRPCError(err); ok {
		return rpc.Code >= 500 || rpc.Code == 0
	}
	return true
}
