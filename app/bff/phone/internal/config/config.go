package config

import (
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/brand"
)

// callConfigJSON is returned by phone.getCallConfig. tgcalls ignores unknown
// keys and fills defaults for missing ones; these match the libtgvoip keys
// stock clients still read, plus the video toggles current tgcalls builds use.
// The server list is not in this JSON. Clients learn STUN and TURN from
// phoneCall.connections (phoneConnectionWebrtc) after phone.confirmCall.
func callConfigJSON() string {
	return `{"audio_frame_size":60,"jitter_min_delay_20":6,"jitter_min_delay_40":4,"jitter_min_delay_60":2,"jitter_max_delay_20":25,"jitter_max_delay_40":15,"jitter_max_delay_60":10,"jitter_max_slots_delay_20":50,"jitter_max_slots_delay_40":30,"jitter_max_slots_delay_60":20,"jitter_losses_to_reset":20,"jitter_resync_threshold":0.5,"audio_congestion_window":1024,"audio_max_bitrate":20000,"audio_init_bitrate":16000,"audio_bitrate_step_incr":1000,"audio_bitrate_step_decr":1000,"use_system_ns":true,"use_system_aec":true,"force_tcp":false,"enable_stun_marking":true,"enable_p2p":true,"enable_aec":true,"enable_ns":true,"enable_agc":true,"enable_vp8_encoder":true,"enable_vp8_decoder":true,"enable_h264_encoder":true,"enable_h264_decoder":true}`
}

// CustomParamsJSON is phoneCall.custom_parameters for an active call.
const CustomParamsJSON = `{"enable_vp8_encoder":true,"enable_vp8_decoder":true,"enable_h264_encoder":true,"enable_h264_decoder":true,"audio_frame_size":60}`

// Endpoint is one STUN or TURN address handed to the client.
type Endpoint struct {
	Host string
	Port int
}

// Config is the phone: block of bff.yaml.
//
// Addresses are not hardcoded. Yaml is the default; these environment
// variables override it (empty means "leave the yaml value"):
//
//	TELLOMI_CALL_IP, TELLOMI_CALL_IPV6
//	TELLOMI_TURN_HOST, TELLOMI_TURN_PORT, TELLOMI_TURN_SECRET, TELLOMI_TURN_TTL
//	TELLOMI_STUN_HOST, TELLOMI_STUN_PORT
//	TELLOMI_TURN_SERVERS, TELLOMI_STUN_SERVERS  (comma-separated host:port)
//	TELLOMI_REFLECTOR_PORT, TELLOMI_ENABLE_P2P, TELLOMI_CALL_FORCE_TCP
//
// TELLOMI_TURN_SERVERS / TELLOMI_STUN_SERVERS replace the single host/port
// pair when set. IPv6 literals use brackets: [2001:db8::1]:3478.
type Config struct {
	PublicIP       string   `json:",optional"`
	PublicIPv6     string   `json:",optional"`
	ReflectorPort  int      `json:",default=3479"`
	SFUPort        int      `json:",default=3480"`
	TurnHost       string   `json:",optional"`
	TurnPort       int      `json:",default=3478"`
	StunHost       string   `json:",optional"`
	StunPort       int      `json:",default=3478"`
	TurnSecret     string   `json:",optional"`
	TurnTTL        int      `json:",default=86400"`
	TurnServers    []string `json:",optional"`
	StunServers    []string `json:",optional"`
	RingTimeout    int      `json:",default=90"`
	ConfirmTimeout int      `json:",default=30"`
	CallLinkPrefix string   `json:",optional"`
	RTMPURL        string   `json:",optional"`
	RTMPKey        string   `json:",optional"`
	EnableP2P      bool     `json:",default=true"`
	ForceTCP       bool     `json:",optional"`
	MySQL          string   `json:",optional"`
	DisableReflect bool     `json:",optional"`
	DisableSFU     bool     `json:",optional"`
}

func (c Config) Normalized() Config {
	if ip := strings.TrimSpace(os.Getenv("TELLOMI_CALL_IP")); ip != "" {
		c.PublicIP = ip
	}
	if c.PublicIP == "" {
		c.PublicIP = "127.0.0.1"
	}
	if ip := strings.TrimSpace(os.Getenv("TELLOMI_CALL_IPV6")); ip != "" {
		c.PublicIPv6 = ip
	}
	c.PublicIPv6 = ipv6Only(c.PublicIPv6)
	if c.PublicIPv6 == "" {
		c.PublicIPv6 = ipv6Only(c.PublicIP)
	}
	if host := strings.TrimSpace(os.Getenv("TELLOMI_TURN_HOST")); host != "" {
		c.TurnHost = host
	}
	if c.TurnHost == "" {
		c.TurnHost = c.PublicIP
	}
	if host := strings.TrimSpace(os.Getenv("TELLOMI_STUN_HOST")); host != "" {
		c.StunHost = host
	}
	if c.StunHost == "" {
		c.StunHost = c.TurnHost
	}
	if secret := os.Getenv("TELLOMI_TURN_SECRET"); secret != "" {
		c.TurnSecret = secret
	}
	if c.TurnSecret == "" {
		c.TurnSecret = "tellomi-turn-secret"
	}
	if port, ok := envPort("TELLOMI_REFLECTOR_PORT"); ok {
		c.ReflectorPort = port
	} else if c.ReflectorPort == 0 {
		c.ReflectorPort = 3479
	}
	if c.SFUPort == 0 {
		c.SFUPort = 3480
	}
	if port, ok := envPort("TELLOMI_TURN_PORT"); ok {
		c.TurnPort = port
	} else if c.TurnPort == 0 {
		c.TurnPort = 3478
	}
	if port, ok := envPort("TELLOMI_STUN_PORT"); ok {
		c.StunPort = port
	} else if c.StunPort == 0 {
		c.StunPort = 3478
	}
	if ttl, ok := envInt("TELLOMI_TURN_TTL"); ok && ttl > 0 {
		c.TurnTTL = ttl
	} else if c.TurnTTL <= 0 {
		c.TurnTTL = 86400
	}
	if raw := strings.TrimSpace(os.Getenv("TELLOMI_TURN_SERVERS")); raw != "" {
		c.TurnServers = splitList(raw)
	}
	if raw := strings.TrimSpace(os.Getenv("TELLOMI_STUN_SERVERS")); raw != "" {
		c.StunServers = splitList(raw)
	}
	if c.RingTimeout <= 0 {
		c.RingTimeout = 90
	}
	if c.ConfirmTimeout <= 0 {
		c.ConfirmTimeout = 30
	}
	if c.CallLinkPrefix == "" {
		c.CallLinkPrefix = brand.CallPrefix
	}
	// TELLOMI_RTMP_URL is the public ingest base returned by
	// phone.getGroupCallStreamRtmpUrl. An empty value keeps the yaml
	// setting, which itself defaults to the local MediaMTX address.
	if url := strings.TrimSpace(os.Getenv("TELLOMI_RTMP_URL")); url != "" {
		c.RTMPURL = url
	}
	if c.RTMPURL == "" {
		c.RTMPURL = "rtmp://127.0.0.1:1935/live"
	}
	if c.RTMPKey == "" {
		c.RTMPKey = "tellomi"
	}
	if v, ok := envBool("TELLOMI_ENABLE_P2P"); ok {
		c.EnableP2P = v
	}
	if v, ok := envBool("TELLOMI_CALL_FORCE_TCP"); ok {
		c.ForceTCP = v
	}
	return c
}

// CallConfigBody is the dataJSON payload for phone.getCallConfig.
// enable_p2p and force_tcp follow this config so an operator can force
// relay without rebuilding the JSON blob.
func (c Config) CallConfigBody() string {
	raw := callConfigJSON()
	if !c.EnableP2P {
		raw = strings.ReplaceAll(raw, `"enable_p2p":true`, `"enable_p2p":false`)
	}
	if c.ForceTCP {
		raw = strings.ReplaceAll(raw, `"force_tcp":false`, `"force_tcp":true`)
	}
	return raw
}

// IPv6 is the address to write into phoneConnection.ipv6. An IPv4 string
// is omitted; clients treat that field as a literal IPv6 candidate.
func (c Config) IPv6() string {
	return ipv6Only(c.PublicIPv6)
}

// TURNEndpoints is the TURN set advertised as phoneConnectionWebrtc turn:true.
// An explicit server list replaces TurnHost:TurnPort.
func (c Config) TURNEndpoints() []Endpoint {
	if eps := parseEndpoints(c.TurnServers); len(eps) > 0 {
		return eps
	}
	if c.TurnPort > 0 && c.TurnHost != "" {
		return []Endpoint{{Host: c.TurnHost, Port: c.TurnPort}}
	}
	return nil
}

// STUNEndpoints is the STUN set advertised as phoneConnectionWebrtc stun:true.
func (c Config) STUNEndpoints() []Endpoint {
	if eps := parseEndpoints(c.StunServers); len(eps) > 0 {
		return eps
	}
	if c.StunPort > 0 && c.StunHost != "" {
		return []Endpoint{{Host: c.StunHost, Port: c.StunPort}}
	}
	return nil
}

func (c Config) Ring() time.Duration {
	return time.Duration(c.Normalized().RingTimeout) * time.Second
}

func (c Config) Confirm() time.Duration {
	return time.Duration(c.Normalized().ConfirmTimeout) * time.Second
}

func ipv6Only(s string) string {
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil || ip.To4() != nil {
		return ""
	}
	return ip.String()
}

func envInt(key string) (int, bool) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func envPort(key string) (int, bool) {
	n, ok := envInt(key)
	if !ok || n > 65535 {
		return 0, false
	}
	return n, true
}

func envBool(key string) (bool, bool) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return false, false
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func splitList(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseEndpoints(list []string) []Endpoint {
	var out []Endpoint
	for _, raw := range list {
		host, port, ok := splitHostPort(raw)
		if ok {
			out = append(out, Endpoint{Host: host, Port: port})
		}
	}
	return out
}

func splitHostPort(s string) (string, int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, false
	}
	host, portStr, err := net.SplitHostPort(s)
	if err != nil || host == "" {
		return "", 0, false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, false
	}
	return host, port, true
}
