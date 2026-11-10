package config

import (
	"testing"

	"github.com/teamgram/teamgram-server/pkg/configload"
)

func TestProxyProtocolV1Listeners(t *testing.T) {
	for _, name := range []string{"etc/gnetway.yaml", "etc2/gnetway.yaml"} {
		t.Run(name, func(t *testing.T) {
			var c Config
			configload.MustLoad(teamgramd(t, name), &c)
			if c.Gnetway == nil {
				t.Fatal("Gnetway is nil")
			}
			assertProductionPPV1(t, *c.Gnetway)
		})
	}
}

func assertProductionPPV1(t *testing.T, c GnetwayConfig) {
	t.Helper()
	var (
		plain     *GnetwayServer
		websocket *GnetwayServer
		private   *GnetwayServer
	)
	for i := range c.Server {
		s := &c.Server[i]
		switch {
		case s.Proto == "tcp" && !s.PPV1:
			if plain != nil {
				t.Fatalf("more than one public tcp group: %+v", c.Server)
			}
			plain = s
		case s.Proto == "websocket":
			if websocket != nil {
				t.Fatalf("more than one websocket group: %+v", c.Server)
			}
			websocket = s
		case s.Proto == "tcp" && s.PPV1:
			if private != nil {
				t.Fatalf("more than one PPV1 group: %+v", c.Server)
			}
			private = s
		default:
			t.Fatalf("unexpected listener group: %+v", s)
		}
	}
	if plain == nil || websocket == nil || private == nil {
		t.Fatalf("missing listener group: %+v", c.Server)
	}
	if got, want := plain.Addresses, []string{"0.0.0.0:10443", "0.0.0.0:5222"}; !sameStrings(got, want) {
		t.Fatalf("public tcp addresses = %v, want %v", got, want)
	}
	if got, want := websocket.Addresses, []string{"0.0.0.0:11443"}; !sameStrings(got, want) {
		t.Fatalf("websocket addresses = %v, want %v", got, want)
	}
	if got, want := private.Addresses, []string{"0.0.0.0:10444"}; !sameStrings(got, want) {
		t.Fatalf("private tcp addresses = %v, want %v", got, want)
	}

	// Exact string match against the listener address gnet reports.
	if !c.IsProxyProtocolV1("0.0.0.0:10444") {
		t.Fatal("10444 does not have PPV1")
	}
	for _, addr := range []string{
		"0.0.0.0:10443",
		"0.0.0.0:5222",
		"0.0.0.0:11443",
		"127.0.0.1:10444",
		"127.0.0.1:10443",
		":10444",
		"0.0.0.0:10444 ",
	} {
		if c.IsProxyProtocolV1(addr) {
			t.Fatalf("IsProxyProtocolV1(%q) = true, want false", addr)
		}
	}
	if !c.IsTcp("0.0.0.0:10444") || !c.IsTcp("0.0.0.0:10443") {
		t.Fatal("10444 or 10443 is not a tcp listener")
	}
	if c.IsWebsocket("0.0.0.0:10444") || c.IsTcp("0.0.0.0:11443") {
		t.Fatal("protocol classification crossed groups")
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func teamgramd(t *testing.T, name string) string {
	t.Helper()
	return etc2(t, "../"+name)
}
