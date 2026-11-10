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
	Proto     string `json:",default=tcp,options=tcp|websocket|http"`
	PPV1      bool   `json:",optional"`
	Addresses []string
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
