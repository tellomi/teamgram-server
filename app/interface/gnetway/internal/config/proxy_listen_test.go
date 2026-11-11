package config

import (
	"testing"

	"github.com/teamgram/teamgram-server/pkg/configload"
)

func TestProxyListenerDisabledByDefault(t *testing.T) {
	for _, name := range []string{"etc/gnetway.yaml", "etc2/gnetway.yaml"} {
		t.Run(name, func(t *testing.T) {
			var c Config
			configload.MustLoad(teamgramd(t, name), &c)
			if err := c.Gnetway.Validate(); err != nil {
				t.Fatal(err)
			}
			for _, s := range c.Gnetway.Server {
				if s.ProxyProtocol != "" || len(s.TrustedProxyCIDRs) > 0 || s.RequireProxyHeader != nil {
					t.Fatalf("production listener enables private PROXY: %+v", s)
				}
			}
		})
	}
}

func TestProxyListenerRefusesStartWithoutTrustedCIDR(t *testing.T) {
	base := GnetwayServer{
		Proto:         "tcp",
		ProxyProtocol: "v1v2",
		Addresses:     []string{"10.20.0.218:10445"},
	}
	cases := []struct {
		name   string
		server GnetwayServer
	}{
		{name: "no cidr", server: base},
		{name: "empty cidr", server: withCIDRs(base, "")},
		{name: "slash 12", server: withCIDRs(base, "172.16.0.0/12")},
		{name: "any v4", server: withCIDRs(base, "0.0.0.0/0")},
		{name: "any v6", server: withCIDRs(base, "::/0")},
		{name: "slash 31", server: withCIDRs(base, "172.19.44.222/31")},
		{name: "unspecified bind", server: bind(withCIDRs(base, "172.19.44.222/32"), "0.0.0.0:10445")},
		{name: "wildcard v6 bind", server: bind(withCIDRs(base, "172.19.44.222/32"), "[::]:10445")},
		{name: "with ppv1", server: ppv1(withCIDRs(base, "172.19.44.222/32"))},
		{name: "bad protocol", server: protocol(withCIDRs(base, "172.19.44.222/32"), "v3")},
		{name: "cidr without protocol", server: GnetwayServer{Proto: "tcp", Addresses: []string{"10.20.0.218:10445"}, TrustedProxyCIDRs: []string{"172.19.44.222/32"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := (GnetwayConfig{Server: []GnetwayServer{tc.server}}).Validate()
			if err == nil {
				t.Fatal("Validate succeeded")
			}
		})
	}
}

func TestProxyListenerAcceptsSingleHostCIDR(t *testing.T) {
	falseValue := false
	err := (GnetwayConfig{Server: []GnetwayServer{{
		Proto:              "tcp",
		ProxyProtocol:      "v1v2",
		RequireProxyHeader: &falseValue,
		TrustedProxyCIDRs:  []string{"172.19.44.222/32", "2001:db8::1/128"},
		Addresses:          []string{"10.20.0.218:10445"},
	}}}).Validate()
	if err != nil {
		t.Fatal(err)
	}
}

func withCIDRs(s GnetwayServer, cidr string) GnetwayServer {
	if cidr != "" {
		s.TrustedProxyCIDRs = []string{cidr}
	} else {
		s.TrustedProxyCIDRs = []string{""}
	}
	return s
}

func bind(s GnetwayServer, address string) GnetwayServer {
	s.Addresses = []string{address}
	return s
}

func ppv1(s GnetwayServer) GnetwayServer {
	s.PPV1 = true
	return s
}

func protocol(s GnetwayServer, proto string) GnetwayServer {
	s.ProxyProtocol = proto
	return s
}
