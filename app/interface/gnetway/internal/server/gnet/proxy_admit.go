package gnet

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/config"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/server/gnet/pp"

	"github.com/panjf2000/gnet/v2"
	"github.com/zeromicro/go-zero/core/logx"
)

// proxyHeaderReadTimeout is how long a trusted relay may take to send one
// PROXY header. It is not the 300s idle timer of an established session.
const proxyHeaderReadTimeout = 3 * time.Second

const proxyV2HeaderMin = 16

type proxyListenerSpec struct {
	allowV1 bool
	allowV2 bool
	require bool
	peers   []*net.IPNet
}

type proxyAdmit struct {
	spec     proxyListenerSpec
	peer     string
	peerHost string
	deadline time.Time
}

type proxyParseStatus int

const (
	proxyNeedMore proxyParseStatus = iota
	proxyNotHeader
	proxyReject
	proxyOK
)

func buildProxyListeners(c config.GnetwayConfig) (map[string]proxyListenerSpec, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	out := map[string]proxyListenerSpec{}
	for _, server := range c.Server {
		proto := strings.TrimSpace(server.ProxyProtocol)
		if proto == "" {
			continue
		}
		spec := proxyListenerSpec{
			allowV1: proto == "v1" || proto == "v1v2",
			allowV2: proto == "v2" || proto == "v1v2",
			require: server.RequireHeader(),
		}
		for _, cidr := range server.TrustedProxyCIDRs {
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				return nil, err
			}
			spec.peers = append(spec.peers, network)
		}
		for _, address := range server.Addresses {
			out[address] = spec
		}
	}
	return out, nil
}

func (s proxyListenerSpec) allows(ip net.IP) bool {
	ip = canonicalIP(ip)
	if ip == nil {
		return false
	}
	for _, network := range s.peers {
		if network.Contains(ip) || network.Contains(ip.To16()) {
			return true
		}
	}
	return false
}

func canonicalIP(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip
}

func peerIP(addr net.Addr) (net.IP, string) {
	if addr == nil {
		return nil, ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		host = addr.String()
	}
	return canonicalIP(net.ParseIP(host)), host
}

// parseProxyHeader reads one PROXY header from buf.
// consumed is meaningful only for proxyOK.
func parseProxyHeader(buf []byte, allowV1, allowV2 bool) (h *pp.Header, consumed int, status proxyParseStatus, reason string) {
	if len(buf) == 0 {
		return nil, 0, proxyNeedMore, ""
	}
	if hasPrefix(buf, pp.V2Identifier) {
		if !allowV2 {
			return nil, 0, proxyReject, "version"
		}
		return parseV2Header(buf)
	}
	if hasPrefix(buf, pp.V1Identifier) {
		if !allowV1 {
			return nil, 0, proxyReject, "version"
		}
		return parseV1Header(buf)
	}
	if prefixOf(buf, pp.V2Identifier) || prefixOf(buf, pp.V1Identifier) {
		if (prefixOf(buf, pp.V2Identifier) && allowV2) || (prefixOf(buf, pp.V1Identifier) && allowV1) {
			return nil, 0, proxyNeedMore, ""
		}
	}
	return nil, 0, proxyNotHeader, "missing_header"
}

func parseV2Header(buf []byte) (*pp.Header, int, proxyParseStatus, string) {
	if len(buf) < proxyV2HeaderMin {
		return nil, 0, proxyNeedMore, ""
	}
	if (buf[12] & 0xF0) != 0x20 {
		return nil, 0, proxyReject, "version"
	}
	length := int(binary.BigEndian.Uint16(buf[14:16]))
	if length > 2048 {
		return nil, 0, proxyReject, "oversized"
	}
	total := proxyV2HeaderMin + length
	if len(buf) < total {
		return nil, 0, proxyNeedMore, ""
	}
	h, err := pp.ReadHeader(bytes.NewReader(buf[:total]))
	if err != nil || h == nil {
		return nil, 0, proxyReject, "bad_header"
	}
	if reason := rejectProxyCommand(h, buf[12], buf[13]); reason != "" {
		return nil, 0, proxyReject, reason
	}
	return h, total, proxyOK, ""
}

func parseV1Header(buf []byte) (*pp.Header, int, proxyParseStatus, string) {
	limit := 107
	if len(buf) < limit {
		limit = len(buf)
	}
	end := bytes.Index(buf[:limit], []byte("\r\n"))
	if end < 0 {
		if len(buf) >= 107 {
			return nil, 0, proxyReject, "oversized"
		}
		return nil, 0, proxyNeedMore, ""
	}
	total := end + 2
	h, err := pp.ReadHeader(bytes.NewReader(buf[:total]))
	if err != nil || h == nil {
		return nil, 0, proxyReject, "bad_header"
	}
	if h.IsLocal || h.Source == nil {
		return nil, 0, proxyReject, "local"
	}
	if _, ok := h.Source.(*net.TCPAddr); !ok {
		return nil, 0, proxyReject, "udp"
	}
	return h, total, proxyOK, ""
}

func rejectProxyCommand(h *pp.Header, verCmd, family byte) string {
	switch verCmd & 0x0F {
	case 0x00:
		return "local"
	case 0x01:
	default:
		return "command"
	}
	if h.IsLocal || h.Source == nil {
		return "local"
	}
	switch family {
	case 0x11, 0x21:
		if _, ok := h.Source.(*net.TCPAddr); !ok {
			return "bad_header"
		}
		return ""
	case 0x12, 0x22:
		return "udp"
	case 0x31, 0x32:
		return "unix"
	case 0x00:
		return "unspec"
	default:
		return "family"
	}
}

func headerHost(addr net.Addr) (string, bool) {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok || tcp == nil || tcp.IP == nil {
		return "", false
	}
	ip := canonicalIP(tcp.IP)
	if ip == nil {
		return "", false
	}
	return ip.String(), true
}

func hasPrefix(buf, sig []byte) bool {
	return len(buf) >= len(sig) && bytes.Equal(buf[:len(sig)], sig)
}

func prefixOf(buf, sig []byte) bool {
	n := len(buf)
	if n >= len(sig) {
		return false
	}
	return bytes.Equal(buf, sig[:n])
}

func looksLikeProxyHeader(buf []byte) bool {
	return hasPrefix(buf, pp.V1Identifier) || hasPrefix(buf, pp.V2Identifier)
}

func (s *Server) expireProxyHeaders() {
	now := time.Now()
	s.proxyWaiting.Range(func(key, _ any) bool {
		connId, ok := key.(int64)
		if !ok {
			return true
		}
		s.eng.Trigger(connId, func(c gnet.Conn) {
			ctx, _ := c.Context().(*connContext)
			if ctx == nil || ctx.proxy == nil || now.Before(ctx.proxy.deadline) {
				return
			}
			noteEntryReject("timeout")
			logx.Errorf("proxy reject reason=timeout peer=%s local=%s", ctx.proxy.peer, c.LocalAddr().String())
			s.proxyWaiting.Delete(connId)
			_ = c.Close()
		})
		return true
	})
}

func (s *Server) readProxyHeader(c gnet.Conn, ctx *connContext) gnet.Action {
	admit := ctx.proxy
	if admit == nil {
		return gnet.None
	}
	local := c.LocalAddr().String()
	if time.Now().After(admit.deadline) {
		noteEntryReject("timeout")
		logx.Errorf("proxy reject reason=timeout peer=%s local=%s", admit.peer, local)
		s.finishProxyWait(c, ctx)
		return gnet.Close
	}
	buf, err := c.Peek(-1)
	if err != nil || len(buf) == 0 {
		return gnet.None
	}
	h, n, status, reason := parseProxyHeader(buf, admit.spec.allowV1, admit.spec.allowV2)
	switch status {
	case proxyNeedMore:
		return gnet.None
	case proxyNotHeader:
		if admit.spec.require {
			noteEntryReject(reason)
			logx.Errorf("proxy reject reason=%s peer=%s local=%s", reason, admit.peer, local)
			s.finishProxyWait(c, ctx)
			return gnet.Close
		}
		ctx.setClientIp(admit.peerHost)
		logx.Infof("proxy no header client_ip=%s peer=%s local=%s", ctx.clientIp, admit.peer, local)
		s.finishProxyWait(c, ctx)
		return gnet.None
	case proxyReject:
		noteEntryReject(reason)
		logx.Errorf("proxy reject reason=%s peer=%s local=%s", reason, admit.peer, local)
		s.finishProxyWait(c, ctx)
		return gnet.Close
	case proxyOK:
		if n < len(buf) && looksLikeProxyHeader(buf[n:]) {
			noteEntryReject("duplicate")
			logx.Errorf("proxy reject reason=duplicate peer=%s local=%s", admit.peer, local)
			s.finishProxyWait(c, ctx)
			return gnet.Close
		}
		host, ok := headerHost(h.Source)
		if !ok {
			noteEntryReject("bad_header")
			logx.Errorf("proxy reject reason=bad_header peer=%s local=%s", admit.peer, local)
			s.finishProxyWait(c, ctx)
			return gnet.Close
		}
		if _, err := c.Discard(n); err != nil {
			noteEntryReject("discard")
			logx.Errorf("proxy reject reason=discard peer=%s local=%s", admit.peer, local)
			s.finishProxyWait(c, ctx)
			return gnet.Close
		}
		ctx.setClientIp(host)
		ctx.assignEntry(classifyNativeEntry(tcpPort(c.LocalAddr()), true, destPort(h.Destination)))
		logx.Infof("proxy accepted client_ip=%s peer=%s local=%s", host, admit.peer, local)
		s.finishProxyWait(c, ctx)
		return gnet.None
	default:
		noteEntryReject("bad_header")
		logx.Errorf("proxy reject reason=bad_header peer=%s local=%s", admit.peer, local)
		s.finishProxyWait(c, ctx)
		return gnet.Close
	}
}

func (s *Server) finishProxyWait(c gnet.Conn, ctx *connContext) {
	ctx.proxy = nil
	s.proxyWaiting.Delete(c.ConnId())
}
