/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package config

import (
	"fmt"
	"net"
	"strings"

	"github.com/teamgram/marmota/pkg/container2"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	RSAKey           []RSAKey
	Gnetway          *GnetwayConfig
	Session          zrpc.RpcClientConf
	UseStreamSession bool `json:",default=false"`
}

type RSAKey struct {
	KeyFile        string
	KeyFingerprint string
}

type GnetwayServer struct {
	Proto string `json:",default=tcp,options=tcp|websocket|http"`
	PPV1  bool   `json:",optional"`
	// ProxyProtocol is empty, v1, v2, or v1v2. Empty keeps today's listener:
	// no PROXY header is read. PPV1 stays the older optional-v1 switch used
	// by the loopback 10444 listener and cannot be combined with this.
	ProxyProtocol string `json:",optional"`
	// RequireProxyHeader defaults to true when ProxyProtocol is set. A trusted
	// peer that does not send one TCP header is disconnected. False keeps the
	// socket peer as client_ip when the first bytes are not a PROXY header.
	RequireProxyHeader *bool `json:",optional"`
	// TrustedProxyCIDRs is required when ProxyProtocol is set. Each entry must
	// be one IPv4 /32 or one IPv6 /128. The socket peer is matched, not the
	// address inside the header.
	TrustedProxyCIDRs []string `json:",optional"`
	Addresses         []string
}

type GnetwayConfig struct {
	Server        []GnetwayServer
	Multicore     bool
	SendBuf       int
	ReceiveBuf    int
	AuthKeyCacheM int `json:",default=10"`
}

func (c GnetwayConfig) IsWebsocket(addr string) bool {
	for _, server := range c.Server {
		if server.Proto == "websocket" {
			for _, address := range server.Addresses {
				if address == addr {
					return true
				}
			}
		}
	}
	return false
}

func (c GnetwayConfig) IsHttp(addr string) bool {
	for _, server := range c.Server {
		if server.Proto == "http" {
			for _, address := range server.Addresses {
				if address == addr {
					return true
				}
			}
		}
	}
	return false
}

func (c GnetwayConfig) IsTcp(addr string) bool {
	for _, server := range c.Server {
		if server.Proto == "tcp" {
			for _, address := range server.Addresses {
				if address == addr {
					return true
				}
			}
		}
	}
	return false
}

// IsProxyProtocolV1 reports whether addr is exactly a listener configured
// with PPV1. The comparison is the full address string, not the port and
// not a wildcard. "0.0.0.0:10444" does not match "127.0.0.1:10444".
//
// gnet copies the listening socket's address onto every accepted
// connection (teamgram/gnet acceptor_unix.go: newStreamConn(...,
// el.listeners[fd].addr, remoteAddr)). That address is
// net.ResolveTCPAddr of the configured string, from tcpSocket, not
// getsockname of the accepted fd. A client that dials 127.0.0.1:10444
// still has LocalAddr "0.0.0.0:10444" when the listener was configured
// as 0.0.0.0:10444. OnOpen passes c.LocalAddr().String() here.
func (c GnetwayConfig) IsProxyProtocolV1(addr string) bool {
	for _, server := range c.Server {
		if server.PPV1 {
			for _, address := range server.Addresses {
				if address == addr {
					return true
				}
			}
		}
	}
	return false
}

// Validate rejects a PROXY listener that is not pinned to a single trusted
// peer. Listeners with an empty ProxyProtocol are unchanged, including the
// optional PPV1 listener.
func (c GnetwayConfig) Validate() error {
	seen := map[string]struct{}{}
	for i := range c.Server {
		s := c.Server[i]
		proto := strings.TrimSpace(s.ProxyProtocol)
		hasExtra := len(s.TrustedProxyCIDRs) > 0 || s.RequireProxyHeader != nil
		if proto == "" {
			if hasExtra {
				return fmt.Errorf("gnetway server %d: TrustedProxyCIDRs and RequireProxyHeader require ProxyProtocol", i)
			}
			continue
		}
		switch proto {
		case "v1", "v2", "v1v2":
		default:
			return fmt.Errorf("gnetway server %d: ProxyProtocol %q, want v1, v2, or v1v2", i, s.ProxyProtocol)
		}
		if s.PPV1 {
			return fmt.Errorf("gnetway server %d: ProxyProtocol and PPV1 cannot share a listener", i)
		}
		if s.Proto != "" && s.Proto != "tcp" {
			return fmt.Errorf("gnetway server %d: ProxyProtocol requires Proto tcp", i)
		}
		if len(s.Addresses) == 0 {
			return fmt.Errorf("gnetway server %d: ProxyProtocol requires an address", i)
		}
		if len(s.TrustedProxyCIDRs) == 0 {
			return fmt.Errorf("gnetway server %d: ProxyProtocol requires TrustedProxyCIDRs", i)
		}
		for _, cidr := range s.TrustedProxyCIDRs {
			if _, err := parseTrustedProxyCIDR(cidr); err != nil {
				return fmt.Errorf("gnetway server %d: %w", i, err)
			}
		}
		for _, address := range s.Addresses {
			if _, ok := seen[address]; ok {
				return fmt.Errorf("gnetway server %d: duplicate listen address %s", i, address)
			}
			seen[address] = struct{}{}
			if err := checkProxyBind(address); err != nil {
				return fmt.Errorf("gnetway server %d: %w", i, err)
			}
		}
	}
	return nil
}

func parseTrustedProxyCIDR(cidr string) (*net.IPNet, error) {
	ip, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return nil, fmt.Errorf("TrustedProxyCIDRs %q: %w", cidr, err)
	}
	ones, bits := network.Mask.Size()
	switch {
	case ip.To4() != nil && ones == 32 && bits == 32:
	case ip.To4() == nil && ones == 128 && bits == 128:
	default:
		return nil, fmt.Errorf("TrustedProxyCIDRs %q must be a single host (/32 or /128)", cidr)
	}
	return network, nil
}

func checkProxyBind(address string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil || port == "" {
		return fmt.Errorf("ProxyProtocol address %q must be host:port", address)
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsUnspecified() {
		return fmt.Errorf("ProxyProtocol address %q must bind one interface, not 0.0.0.0 or ::", address)
	}
	return nil
}

// RequireHeader reports whether a trusted peer must send a PROXY header.
// An omitted flag means yes.
func (s GnetwayServer) RequireHeader() bool {
	if s.RequireProxyHeader == nil {
		return true
	}
	return *s.RequireProxyHeader
}

func (c GnetwayConfig) ToAddresses() []string {
	var addresses []string
	for _, server := range c.Server {
		for _, address := range server.Addresses {
			if ok := container2.ContainsString(addresses, address); !ok {
				addresses = append(addresses, "tcp://"+address)
			}
		}
	}
	return addresses
}
