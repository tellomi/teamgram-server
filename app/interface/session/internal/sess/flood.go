package sess

import (
	"context"
	"net"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/flood"
)

func (c *session) floodDecision(ctx context.Context, clientIP string, query mtproto.TLObject) flood.Decision {
	lim := c.floodLimiter()
	if lim == nil {
		return flood.Decision{Allowed: true}
	}
	userID := int64(0)
	authKey := int64(0)
	if c.sessList != nil && c.sessList.cb != nil {
		userID = c.sessList.cb.AuthUserId
		authKey = c.sessList.cb.authKeyId
	}
	return lim.Allow(ctx, flood.Request{
		Method:  methodName(query),
		UserID:  userID,
		AuthKey: authKey,
		IP:      hostOnly(clientIP),
		Phone:   phoneOf(query),
	})
}

func (c *session) floodLimiter() *flood.Limiter {
	if c == nil || c.sessList == nil || c.sessList.cb == nil || c.sessList.cb.cb == nil {
		return nil
	}
	return c.sessList.cb.cb.Flood
}

func methodName(obj mtproto.TLObject) string {
	if obj == nil {
		return ""
	}
	if c, ok := obj.(interface{ GetConstructor() mtproto.TLConstructor }); ok {
		if name := mtproto.ConstructorName(int32(c.GetConstructor())); name != "" {
			return name
		}
	}
	return ""
}

func phoneOf(obj mtproto.TLObject) string {
	var raw string
	switch o := obj.(type) {
	case *mtproto.TLAuthSendCode:
		raw = o.GetPhoneNumber()
	case *mtproto.TLAuthResendCode:
		raw = o.GetPhoneNumber()
	case *mtproto.TLAuthSignIn:
		raw = o.GetPhoneNumber()
	case *mtproto.TLAuthSignUp:
		raw = o.GetPhoneNumber()
	default:
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hostOnly(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	// A comma-separated list is a forwarded header, not the address the
	// gateway stored after trusted PROXY admission.
	if strings.Contains(addr, ",") {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	if net.ParseIP(addr) == nil {
		return ""
	}
	return addr
}
