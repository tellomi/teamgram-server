package gnet

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/teamgram/marmota/pkg/cache"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/crypto"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/config"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/dao"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/server/gnet/codec"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/svc"
	"github.com/teamgram/teamgram-server/app/interface/session/clientpipe"
	"github.com/teamgram/teamgram-server/app/interface/session/session"

	"github.com/alicebob/miniredis/v2"
	"github.com/panjf2000/gnet/v2/pkg/pool/goroutine"
	goredis "github.com/zeromicro/go-zero/core/stores/redis"
)

func TestProxyHandshakeInitConnectionUsesHeaderIP(t *testing.T) {
	cases := []struct {
		name   string
		proto  string
		header func() []byte
		want   string
	}{
		{
			name:   "v2 tcp4",
			proto:  "v1v2",
			header: func() []byte { return proxyV2TCP4("203.0.113.77", 40000) },
			want:   "203.0.113.77",
		},
		{
			name:   "v1 tcp6",
			proto:  "v1v2",
			header: func() []byte { return proxyV1("TCP6", "2001:db8::77", "2001:db8::1", 40001, 10445) },
			want:   "2001:db8::77",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := startProxyGateway(t, tc.proto, "127.0.0.1/32", true)
			if err := gw.handshake(tc.header(), tc.want); err != nil {
				t.Fatal(err)
			}
			gw.waitClientIP(t, tc.want)
			relay := "flood:send_code_ip:ip:127.0.0.1"
			if gw.redis.Exists(relay) {
				t.Fatalf("send_code_ip bucket used the socket peer %s", relay)
			}
			wantKey := "flood:send_code_ip:ip:" + tc.want
			if !gw.redis.Exists(wantKey) {
				t.Fatalf("send_code_ip bucket %s missing, keys %v", wantKey, gw.redis.Keys())
			}
		})
	}
}

func TestProxyOptionalHeaderKeepsSocketPeer(t *testing.T) {
	gw := startProxyGateway(t, "v1v2", "127.0.0.1/32", false)
	if err := gw.handshake(nil, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	gw.waitClientIP(t, "127.0.0.1")
	if gw.redis.Exists("flood:send_code_ip:ip:203.0.113.77") {
		t.Fatal("optional listener invented a header address")
	}
}

func TestPlainListenerIgnoresProxyHeader(t *testing.T) {
	gw := startProxyGateway(t, "v1v2", "127.0.0.1/32", true)
	if err := gw.handshakeAt(gw.plain, nil, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	gw.waitClientIP(t, "127.0.0.1")

	conn, err := net.DialTimeout("tcp", gw.plain, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(append(proxyV1("TCP4", "203.0.113.9", "10.0.0.1", 9, 10443), 0xef)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("plain listener answered a PROXY header")
	}
	for _, ip := range gw.pipe.ClientIPs() {
		if ip == "203.0.113.9" {
			t.Fatal("plain listener stored the PROXY header address")
		}
	}
}

func TestRelayFrontendProxyKeepsClientIP(t *testing.T) {
	before := EntryTotals()
	gw := startProxyGateway(t, "v1v2", "127.0.0.1/32", true)
	front443 := proxyV2(0x11, net.ParseIP("203.0.113.50").To4(), net.ParseIP("8.137.198.240").To4(), 40000, 443)
	if err := gw.handshake(front443, "203.0.113.50"); err != nil {
		t.Fatal(err)
	}
	front10443 := proxyV1("TCP4", "203.0.113.51", "8.137.198.240", 40001, 10443)
	if err := gw.handshake(front10443, "203.0.113.51"); err != nil {
		t.Fatal(err)
	}
	gw.waitClientIP(t, "203.0.113.50")
	gw.waitClientIP(t, "203.0.113.51")
	for _, peer := range []string{"127.0.0.1", "8.137.198.240", "8.156.40.105", "10.20.0.218"} {
		if gw.redis.Exists("flood:send_code_ip:ip:" + peer) {
			t.Fatalf("send_code_ip stored %s instead of the header source", peer)
		}
		for _, ip := range gw.pipe.ClientIPs() {
			if ip == peer {
				t.Fatalf("auths.client_ip stored %s", peer)
			}
		}
	}
	after := EntryTotals()
	if got := after.Conns[entryRelay240443] - before.Conns[entryRelay240443]; got != 1 {
		t.Fatalf("relay240_443 connections %+d", got)
	}
	if got := after.Conns[entryRelay10445] - before.Conns[entryRelay10445]; got != 1 {
		t.Fatalf("relay_10445 connections %+d", got)
	}
	if got := after.Conns[entryDirect10443] - before.Conns[entryDirect10443]; got != 0 {
		t.Fatalf("direct_10443 connections %+d", got)
	}
}

func TestPPV1StaysOptional(t *testing.T) {
	gw := startProxyGateway(t, "v1v2", "127.0.0.1/32", true)
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+gw.ppv1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client, err := openObfuscated(conn)
	if err != nil {
		t.Fatal(err)
	}
	defer client.conn.Close()
	if _, err := client.reqPQ(); err != nil {
		t.Fatal(err)
	}

	conn, err = net.DialTimeout("tcp", "127.0.0.1:"+gw.ppv1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(proxyV1("TCP4", "198.51.100.8", "10.0.0.1", 8, 10444)); err != nil {
		t.Fatal(err)
	}
	client, err = openObfuscated(conn)
	if err != nil {
		t.Fatal(err)
	}
	defer client.conn.Close()
	if err := client.finishHandshake("198.51.100.8"); err != nil {
		t.Fatal(err)
	}
	gw.waitClientIP(t, "198.51.100.8")
}

type proxyGateway struct {
	plain string
	proxy string
	ppv1  string
	pipe  *clientpipe.Pipe
	redis *miniredis.Miniredis
	stop  func()
}

func startProxyGateway(t *testing.T, proto, cidr string, require bool) *proxyGateway {
	t.Helper()
	t.Setenv("TELLOMI_MTPROTO_EXTRA_KEY_FILE", "")
	t.Setenv("TELLOMI_MTPROTO_EXTRA_KEY_FINGERPRINT", "")
	mr := miniredis.RunT(t)
	rds, err := goredis.NewRedis(goredis.RedisConf{Host: mr.Addr(), Type: goredis.NodeType})
	if err != nil {
		t.Fatal(err)
	}
	pipe := clientpipe.Open(rds)
	plain := freePort(t)
	proxy := freePort(t)
	ppv1Port := freePort(t)
	var requirePtr *bool
	if !require {
		f := false
		requirePtr = &f
	}
	cfg := config.Config{
		RSAKey: []config.RSAKey{{
			KeyFile:        repoPath(t, "teamgramd/bin/server_pkcs1.key"),
			KeyFingerprint: strconv.FormatUint(upstreamRSAFingerprint, 10),
		}},
		Gnetway: &config.GnetwayConfig{
			Server: []config.GnetwayServer{
				{Proto: "tcp", Addresses: []string{"127.0.0.1:" + plain}},
				{Proto: "tcp", PPV1: true, Addresses: []string{"0.0.0.0:" + ppv1Port}},
				{
					Proto:              "tcp",
					ProxyProtocol:      proto,
					RequireProxyHeader: requirePtr,
					TrustedProxyCIDRs:  []string{cidr},
					Addresses:          []string{"127.0.0.1:" + proxy},
				},
			},
		},
	}
	if err := cfg.Gnetway.Validate(); err != nil {
		t.Fatal(err)
	}
	s := &Server{
		c:              &cfg,
		authSessionMgr: NewAuthSessionManager(),
		timeoutWheel:   newTimeoutWheel(),
		handshake:      mustNewHandshake(cfg.RSAKey),
		cache:          cache.NewLRUCache(1 << 20),
		pool:           goroutine.Default(),
		booted:         make(chan struct{}),
		svcCtx: &svc.ServiceContext{
			Config:    cfg,
			Dao:       &dao.Dao{SessionDispatcher: dispatchPipe{pipe}, ShardingSessionClient: dao.NewFixedShardingSessionClient(pipe)},
			GatewayId: "test-gw",
		},
	}
	errCh := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("%v", r)
			}
		}()
		s.Serve()
	}()
	t.Cleanup(func() { s.Close() })
	select {
	case <-s.booted:
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("gnetway did not boot")
	}
	waitListen(t, errCh, plain)
	waitListen(t, errCh, proxy)
	waitListen(t, errCh, ppv1Port)
	return &proxyGateway{
		plain: "127.0.0.1:" + plain,
		proxy: "127.0.0.1:" + proxy,
		ppv1:  ppv1Port,
		pipe:  pipe,
		redis: mr,
	}
}

func (g *proxyGateway) handshake(header []byte, want string) error {
	return g.handshakeAt(g.proxy, header, want)
}

func (g *proxyGateway) handshakeAt(addr string, header []byte, want string) error {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return err
	}
	if len(header) > 0 {
		if _, err := conn.Write(header); err != nil {
			conn.Close()
			return err
		}
	}
	client, err := openObfuscated(conn)
	if err != nil {
		conn.Close()
		return err
	}
	defer client.conn.Close()
	return client.finishHandshake(want)
}

func (g *proxyGateway) waitClientIP(t *testing.T, want string) {
	t.Helper()
	key := "flood:send_code_ip:ip:" + want
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ips := g.pipe.ClientIPs()
		for _, ip := range ips {
			if ip == want && g.redis.Exists(key) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("auths.client_ip %v, flood keys %v, want %s", g.pipe.ClientIPs(), g.redis.Keys(), want)
}

type dispatchPipe struct {
	pipe *clientpipe.Pipe
}

func (d dispatchPipe) SendData(ctx context.Context, _ int64, in *session.TLSessionSendDataToSession) error {
	_, err := d.pipe.SessionSendDataToSession(ctx, in)
	return err
}

func (d dispatchPipe) SendHttpData(context.Context, int64, *session.TLSessionSendHttpDataToSession) (*session.HttpSessionData, error) {
	return nil, fmt.Errorf("http")
}

func (d dispatchPipe) CloseSession(context.Context, int64, *session.TLSessionCloseSession) error {
	return nil
}

func (d dispatchPipe) QueryAuthKey(context.Context, int64, *session.TLSessionQueryAuthKey) (*mtproto.AuthKeyInfo, error) {
	return nil, mtproto.ErrAuthKeyUnregistered
}

func (d dispatchPipe) Close() {}

type mtClient struct {
	conn      net.Conn
	encrypt   *crypto.AesCTR128Encrypt
	decrypt   *crypto.AesCTR128Encrypt
	pub       *rsa.PublicKey
	nonce     []byte
	server    []byte
	newNonce  []byte
	authKey   []byte
	authKeyID int64
	salt      int64
	gb        []byte
	keyPath   string
}

func openObfuscated(conn net.Conn) (*mtClient, error) {
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	var buf [64]byte
	for {
		if _, err := rand.Read(buf[:]); err != nil {
			return nil, err
		}
		first := binary.LittleEndian.Uint32(buf[:4])
		second := binary.BigEndian.Uint32(buf[4:8])
		if buf[0] == codec.ABRIDGED_FLAG ||
			first == codec.INTERMEDIATE_FLAG ||
			first == codec.PADDED_INTERMEDIATE_FLAG ||
			first == codec.HTTP_HEAD_FLAG ||
			first == codec.HTTP_POST_FLAG ||
			first == codec.HTTP_GET_FLAG ||
			first == codec.HTTP_OPTION_FLAG ||
			first == codec.PVRG_FLAG ||
			first == codec.UNKNOWN_FLAG ||
			second == codec.FULL_FLAG {
			continue
		}
		break
	}
	plain := append([]byte(nil), buf[:]...)
	binary.BigEndian.PutUint32(plain[56:60], codec.ABRIDGED_INT32_FLAG)
	binary.BigEndian.PutUint16(plain[60:62], 1)
	enc, err := crypto.NewAesCTR128Encrypt(buf[8:40], buf[40:56])
	if err != nil {
		return nil, err
	}
	enc.Encrypt(plain)
	copy(buf[56:], plain[56:])
	rev := make([]byte, 48)
	for i := 0; i < 48; i++ {
		rev[i] = buf[55-i]
	}
	dec, err := crypto.NewAesCTR128Encrypt(rev[:32], rev[32:48])
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(buf[:]); err != nil {
		return nil, err
	}
	return &mtClient{conn: conn, encrypt: enc, decrypt: dec, keyPath: rsaKeyPath()}, nil
}

func (c *mtClient) finishHandshake(wantIP string) error {
	res, err := c.reqPQ()
	if err != nil {
		return err
	}
	if err := c.reqDH(res); err != nil {
		return err
	}
	if err := c.setClientDH(); err != nil {
		return err
	}
	return c.sendInitConnection(wantIP)
}

func (c *mtClient) reqPQ() (*mtproto.TLResPQ, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	c.nonce = nonce
	payload, err := encodeUnencrypted(&mtproto.TLReqPqMulti{
		Constructor: mtproto.CRC32_req_pq_multi,
		Nonce:       nonce,
	})
	if err != nil {
		return nil, err
	}
	if err := c.writePayload(payload); err != nil {
		return nil, err
	}
	frame, err := c.readFrame()
	if err != nil {
		return nil, err
	}
	obj, err := decodeUnencrypted(frame)
	if err != nil {
		return nil, err
	}
	res, ok := obj.(*mtproto.TLResPQ)
	if !ok {
		return nil, fmt.Errorf("resPQ got %T", obj)
	}
	c.server = append([]byte(nil), res.GetServerNonce()...)
	return res, nil
}

func (c *mtClient) reqDH(res *mtproto.TLResPQ) error {
	newNonce := make([]byte, 32)
	if _, err := rand.Read(newNonce); err != nil {
		return err
	}
	c.newNonce = newNonce
	pub, err := loadPublicKey(c.keyPath)
	if err != nil {
		return err
	}
	c.pub = pub
	inner := mtproto.MakeTLPQInnerData(&mtproto.P_QInnerData{
		Pq:          pq,
		P:           string(p),
		Q:           string(q),
		Nonce:       c.nonce,
		ServerNonce: c.server,
		NewNonce:    newNonce,
	})
	raw, err := encodeObject(inner, 0)
	if err != nil {
		return err
	}
	enc, err := crypto.EncryptRSAPad(pub, raw, rand.Reader)
	if err != nil {
		return err
	}
	payload, err := encodeUnencrypted(&mtproto.TLReq_DHParams{
		Constructor:          mtproto.CRC32_req_DH_params,
		Nonce:                c.nonce,
		ServerNonce:          c.server,
		P:                    string(p),
		Q:                    string(q),
		PublicKeyFingerprint: fingerprintInt64(),
		EncryptedData:        string(enc),
	})
	if err != nil {
		return err
	}
	if err := c.writePayload(payload); err != nil {
		return err
	}
	frame, err := c.readFrame()
	if err != nil {
		return err
	}
	obj, err := decodeUnencrypted(frame)
	if err != nil {
		return err
	}
	ok, is := obj.(*mtproto.TLServer_DHParamsOk)
	if !is {
		return fmt.Errorf("server_DH got %T", obj)
	}
	key, iv := dhAES(c.newNonce, c.server)
	ige := crypto.NewAES256IGECryptor(key, iv)
	plain, err := ige.Decrypt([]byte(ok.GetEncryptedAnswer()))
	if err != nil {
		return err
	}
	if len(plain) < 20 {
		return fmt.Errorf("short server dh")
	}
	body, err := decodeObject(plain[20:])
	if err != nil {
		return err
	}
	innerDH, is := body.(*mtproto.TLServer_DHInnerData)
	if !is {
		return fmt.Errorf("server_DH_inner got %T", body)
	}
	g := big.NewInt(int64(innerDH.GetG()))
	prime := new(big.Int).SetBytes([]byte(innerDH.GetDhPrime()))
	ga := new(big.Int).SetBytes([]byte(innerDH.GetGA()))
	b := make([]byte, 256)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	bigB := new(big.Int).SetBytes(b)
	gb := new(big.Int).Exp(g, bigB, prime)
	auth := new(big.Int).Exp(ga, bigB, prime)
	c.authKey = leftPad(auth.Bytes(), 256)
	sum := sha1.Sum(c.authKey)
	c.authKeyID = int64(binary.LittleEndian.Uint64(sum[12:]))
	c.gb = gb.Bytes()
	return nil
}

func (c *mtClient) setClientDH() error {
	inner := mtproto.MakeTLClient_DHInnerData(&mtproto.Client_DH_Inner_Data{
		Nonce:       c.nonce,
		ServerNonce: c.server,
		RetryId:     0,
		GB:          string(c.gb),
	})
	raw, err := encodeObject(inner, 0)
	if err != nil {
		return err
	}
	sum := sha1.Sum(raw)
	padded := append(sum[:], raw...)
	if extra := len(padded) % 16; extra != 0 {
		padded = append(padded, make([]byte, 16-extra)...)
	}
	key, iv := dhAES(c.newNonce, c.server)
	ige := crypto.NewAES256IGECryptor(key, iv)
	enc, err := ige.Encrypt(padded)
	if err != nil {
		return err
	}
	payload, err := encodeUnencrypted(&mtproto.TLSetClient_DHParams{
		Constructor:   mtproto.CRC32_set_client_DH_params,
		Nonce:         c.nonce,
		ServerNonce:   c.server,
		EncryptedData: string(enc),
	})
	if err != nil {
		return err
	}
	if err := c.writePayload(payload); err != nil {
		return err
	}
	frame, err := c.readFrame()
	if err != nil {
		return err
	}
	obj, err := decodeUnencrypted(frame)
	if err != nil {
		return err
	}
	if _, ok := obj.(*mtproto.TLDhGenOk); !ok {
		return fmt.Errorf("dh_gen got %T", obj)
	}
	return nil
}

func (c *mtClient) sendInitConnection(want string) error {
	_ = want
	sendCode, err := encodeObject(&mtproto.TLAuthSendCode{
		Constructor: mtproto.CRC32_auth_sendCode,
		PhoneNumber: "+86 138 0013 8000",
		ApiId:       10840,
		ApiHash:     "test",
		Settings:    mtproto.MakeTLCodeSettings(&mtproto.CodeSettings{}).To_CodeSettings(),
	}, mtproto.Layer)
	if err != nil {
		return err
	}
	initRaw, err := encodeObject(&mtproto.TLInitConnection{
		Constructor:    mtproto.CRC32_initConnection_c1cd5ea9,
		ApiId:          10840,
		DeviceModel:    "proxy-test",
		SystemVersion:  "test",
		AppVersion:     "test",
		SystemLangCode: "en",
		LangPack:       "android",
		LangCode:       "en",
		Query:          sendCode,
	}, 0)
	if err != nil {
		return err
	}
	invoke := &mtproto.TLInvokeWithLayer{
		Constructor: mtproto.CRC32_invokeWithLayer,
		Layer:       mtproto.Layer,
		Query:       initRaw,
	}
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	c.salt = int64(binary.LittleEndian.Uint64(salt))
	sessionID := int64(binary.LittleEndian.Uint64(salt) ^ 0x1111)
	msgID := (time.Now().Unix() << 32) | 4
	x := mtproto.NewEncodeBuf(512)
	x.Long(c.salt)
	x.Long(sessionID)
	if err := (&mtproto.TLMessage2{MsgId: msgID, Seqno: 1, Object: invoke}).Encode(x, 0); err != nil {
		return err
	}
	raw := append([]byte(nil), x.GetBuf()...)
	key := crypto.NewClientAuthKey(c.authKeyID, c.authKey)
	msgKey, enc, err := key.AesIgeEncrypt(raw)
	if err != nil {
		return err
	}
	frame := make([]byte, 8+len(msgKey)+len(enc))
	binary.LittleEndian.PutUint64(frame[:8], uint64(c.authKeyID))
	copy(frame[8:], msgKey)
	copy(frame[8+len(msgKey):], enc)
	return c.writePayload(frame)
}

func (c *mtClient) writePayload(payload []byte) error {
	if len(payload)%4 != 0 {
		return fmt.Errorf("frame length %d", len(payload))
	}
	size := len(payload) / 4
	var frame []byte
	if size < 127 {
		frame = make([]byte, 1+len(payload))
		frame[0] = byte(size)
		copy(frame[1:], payload)
	} else {
		frame = make([]byte, 4+len(payload))
		binary.LittleEndian.PutUint32(frame, uint32(size<<8|127))
		copy(frame[4:], payload)
	}
	c.encrypt.Encrypt(frame)
	_, err := c.conn.Write(frame)
	return err
}

func (c *mtClient) readFrame() ([]byte, error) {
	b, err := c.readDecrypt(1)
	if err != nil {
		return nil, err
	}
	n := int(b[0])
	if b[0] == 0x7f {
		rest, err := c.readDecrypt(3)
		if err != nil {
			return nil, err
		}
		n = int(rest[0]) | int(rest[1])<<8 | int(rest[2])<<16
	}
	if n <= 0 || n > 1<<20 {
		return nil, fmt.Errorf("bad frame len %d", n)
	}
	return c.readDecrypt(n * 4)
}

func (c *mtClient) readDecrypt(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(c.conn, buf); err != nil {
		return nil, err
	}
	c.decrypt.Encrypt(buf)
	return buf, nil
}

func encodeUnencrypted(obj mtproto.TLObject) ([]byte, error) {
	body, err := encodeObject(obj, 0)
	if err != nil {
		return nil, err
	}
	x := mtproto.NewEncodeBuf(len(body) + 32)
	x.Long(0)
	x.Long((time.Now().Unix() << 32) | 4)
	x.Int(int32(len(body)))
	x.Bytes(body)
	return append([]byte(nil), x.GetBuf()...), nil
}

func encodeObject(obj mtproto.TLObject, layer int32) ([]byte, error) {
	x := mtproto.NewEncodeBuf(256)
	if err := obj.Encode(x, layer); err != nil {
		return nil, err
	}
	return append([]byte(nil), x.GetBuf()...), nil
}

func decodeUnencrypted(frame []byte) (mtproto.TLObject, error) {
	if len(frame) < 20 {
		return nil, fmt.Errorf("short unencrypted %d", len(frame))
	}
	return decodeObject(frame[8+8+4:])
}

func decodeObject(body []byte) (mtproto.TLObject, error) {
	d := mtproto.NewDecodeBuf(body)
	obj := d.Object()
	if err := d.GetError(); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("empty object")
	}
	return obj, nil
}

func dhAES(newNonce, serverNonce []byte) (key, iv []byte) {
	tmp := make([]byte, 64)
	a := sha1.Sum(append(append([]byte{}, newNonce...), serverNonce...))
	b := sha1.Sum(append(append([]byte{}, serverNonce...), newNonce...))
	c := sha1.Sum(append(append([]byte{}, newNonce...), newNonce...))
	copy(tmp, a[:])
	copy(tmp[20:], b[:])
	copy(tmp[40:], c[:])
	copy(tmp[60:], newNonce[:4])
	return append([]byte(nil), tmp[:32]...), append([]byte(nil), tmp[32:]...)
}

func leftPad(b []byte, n int) []byte {
	if len(b) >= n {
		return append([]byte(nil), b[len(b)-n:]...)
	}
	out := make([]byte, n)
	copy(out[n-len(b):], b)
	return out
}

func loadPublicKey(path string) (*rsa.PublicKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("pem")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return &key.PublicKey, nil
}

func fingerprintInt64() int64 {
	fp := upstreamRSAFingerprint
	return int64(fp)
}

func rsaKeyPath() string {
	return "../" + "../" + "../" + "../" + "../" + "../" + "teamgramd/bin/server_pkcs1.key"
}

func proxyV1(proto, src, dst string, sport, dport int) []byte {
	return []byte(fmt.Sprintf("PROXY %s %s %s %d %d\r\n", proto, src, dst, sport, dport))
}

func proxyV2TCP4(ip string, port int) []byte {
	return proxyV2(0x11, net.ParseIP(ip).To4(), net.IPv4(10, 0, 0, 1).To4(), port, 443)
}

func proxyV2TCP6(ip string, port int) []byte {
	return proxyV2(0x21, net.ParseIP(ip).To16(), net.ParseIP("2001:db8::1").To16(), port, 443)
}

func proxyV2(family byte, src, dst net.IP, sport, dport int) []byte {
	addrLen := 12
	if family == 0x21 || family == 0x22 {
		addrLen = 36
	}
	buf := make([]byte, 16+addrLen)
	copy(buf, []byte{0x0d, 0x0a, 0x0d, 0x0a, 0x00, 0x0d, 0x0a, 0x51, 0x55, 0x49, 0x54, 0x0a})
	buf[12] = 0x21
	buf[13] = family
	binary.BigEndian.PutUint16(buf[14:16], uint16(addrLen))
	if family == 0x11 || family == 0x12 {
		copy(buf[16:20], src.To4())
		copy(buf[20:24], dst.To4())
		binary.BigEndian.PutUint16(buf[24:26], uint16(sport))
		binary.BigEndian.PutUint16(buf[26:28], uint16(dport))
	} else {
		copy(buf[16:32], src.To16())
		copy(buf[32:48], dst.To16())
		binary.BigEndian.PutUint16(buf[48:50], uint16(sport))
		binary.BigEndian.PutUint16(buf[50:52], uint16(dport))
	}
	return buf
}
