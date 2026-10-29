package config

import (
	"os"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/brand"
)

// CallConfigJSON is returned by phone.getCallConfig. tgcalls ignores unknown
// keys and fills defaults for missing ones; these match the libtgvoip keys
// stock clients still read, plus the video toggles current tgcalls builds use.
const CallConfigJSON = `{"audio_frame_size":60,"jitter_min_delay_20":6,"jitter_min_delay_40":4,"jitter_min_delay_60":2,"jitter_max_delay_20":25,"jitter_max_delay_40":15,"jitter_max_delay_60":10,"jitter_max_slots_delay_20":50,"jitter_max_slots_delay_40":30,"jitter_max_slots_delay_60":20,"jitter_losses_to_reset":20,"jitter_resync_threshold":0.5,"audio_congestion_window":1024,"audio_max_bitrate":20000,"audio_init_bitrate":16000,"audio_bitrate_step_incr":1000,"audio_bitrate_step_decr":1000,"use_system_ns":true,"use_system_aec":true,"force_tcp":false,"enable_stun_marking":true,"enable_p2p":true,"enable_aec":true,"enable_ns":true,"enable_agc":true,"enable_vp8_encoder":true,"enable_vp8_decoder":true,"enable_h264_encoder":true,"enable_h264_decoder":true}`

// CustomParamsJSON is phoneCall.custom_parameters for an active call.
const CustomParamsJSON = `{"enable_vp8_encoder":true,"enable_vp8_decoder":true,"enable_h264_encoder":true,"enable_h264_decoder":true,"audio_frame_size":60}`

// Config is the phone: block of bff.yaml.
type Config struct {
	PublicIP       string `json:",optional"`
	PublicIPv6     string `json:",optional"`
	ReflectorPort  int    `json:",default=3479"`
	SFUPort        int    `json:",default=3480"`
	TurnHost       string `json:",optional"`
	TurnPort       int    `json:",default=3478"`
	StunPort       int    `json:",default=3478"`
	TurnSecret     string `json:",optional"`
	TurnTTL        int    `json:",default=86400"`
	RingTimeout    int    `json:",default=90"`
	ConfirmTimeout int    `json:",default=30"`
	CallLinkPrefix string `json:",optional"`
	RTMPURL        string `json:",optional"`
	RTMPKey        string `json:",optional"`
	EnableP2P      bool   `json:",default=true"`
	MySQL          string `json:",optional"`
	DisableReflect bool   `json:",optional"`
	DisableSFU     bool   `json:",optional"`
}

func (c Config) Normalized() Config {
	if ip := os.Getenv("TELLOMI_CALL_IP"); ip != "" {
		c.PublicIP = ip
	}
	if c.PublicIP == "" {
		c.PublicIP = "127.0.0.1"
	}
	if c.PublicIPv6 == "" {
		c.PublicIPv6 = c.PublicIP
	}
	if host := os.Getenv("TELLOMI_TURN_HOST"); host != "" {
		c.TurnHost = host
	}
	if c.TurnHost == "" {
		c.TurnHost = c.PublicIP
	}
	if secret := os.Getenv("TELLOMI_TURN_SECRET"); secret != "" {
		c.TurnSecret = secret
	}
	if c.TurnSecret == "" {
		c.TurnSecret = "tellomi-turn-secret"
	}
	if c.ReflectorPort == 0 {
		c.ReflectorPort = 3479
	}
	if c.SFUPort == 0 {
		c.SFUPort = 3480
	}
	if c.TurnPort == 0 {
		c.TurnPort = 3478
	}
	if c.StunPort == 0 {
		c.StunPort = 3478
	}
	if c.TurnTTL <= 0 {
		c.TurnTTL = 86400
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
	return c
}

func (c Config) Ring() time.Duration {
	return time.Duration(c.Normalized().RingTimeout) * time.Second
}

func (c Config) Confirm() time.Duration {
	return time.Duration(c.Normalized().ConfirmTimeout) * time.Second
}
