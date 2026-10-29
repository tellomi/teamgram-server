// Package sfu speaks the tgcalls group-call JSON used by stock Telegram
// clients (TelegramMessenger/tgcalls tools/go_sfu). The SFU is the DTLS
// client (fingerprint setup "active"); tgcalls GroupInstanceCustomImpl is
// the DTLS server and the ICE-controlled agent.
package sfu

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strconv"
)

type joinPayload struct {
	SSRC         int32             `json:"ssrc"`
	Ufrag        string            `json:"ufrag"`
	Pwd          string            `json:"pwd"`
	Fingerprints []fingerprintJSON `json:"fingerprints"`
	SSRCGroups   []ssrcGroupJSON   `json:"ssrc-groups"`
}

type ssrcGroupJSON struct {
	Semantics string  `json:"semantics"`
	Sources   []int32 `json:"sources"`
}

type fingerprintJSON struct {
	Hash        string `json:"hash"`
	Fingerprint string `json:"fingerprint"`
	Setup       string `json:"setup"`
}

type joinResponse struct {
	Transport transportJSON      `json:"transport"`
	Video     *videoResponseJSON `json:"video,omitempty"`
}

type videoResponseJSON struct {
	Endpoint     string                 `json:"endpoint"`
	PayloadTypes []videoPayloadTypeJSON `json:"payload-types"`
	RTPHdrexts   []rtpHdrextJSON        `json:"rtp-hdrexts"`
}

type videoPayloadTypeJSON struct {
	ID         int                `json:"id"`
	Name       string             `json:"name"`
	Clockrate  int                `json:"clockrate"`
	Parameters map[string]string  `json:"parameters,omitempty"`
	Feedback   []rtcpFeedbackJSON `json:"rtcp-fbs,omitempty"`
}

type rtcpFeedbackJSON struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`
}

type rtpHdrextJSON struct {
	ID  int    `json:"id"`
	URI string `json:"uri"`
}

type transportJSON struct {
	Ufrag        string            `json:"ufrag"`
	Pwd          string            `json:"pwd"`
	Fingerprints []fingerprintJSON `json:"fingerprints"`
	Candidates   []candidateJSON   `json:"candidates"`
}

type candidateJSON struct {
	Port       string `json:"port"`
	Protocol   string `json:"protocol"`
	Network    string `json:"network"`
	Generation string `json:"generation"`
	ID         string `json:"id"`
	Component  string `json:"component"`
	Foundation string `json:"foundation"`
	Priority   string `json:"priority"`
	IP         string `json:"ip"`
	Type       string `json:"type"`
}

// Group is one ssrc-group from a tgcalls join payload (SIM or FID).
type Group struct {
	Semantics string
	Sources   []int32
}

// Offer is the client's phone.joinGroupCall params object.
type Offer struct {
	AudioSSRC   uint32
	Ufrag       string
	Pwd         string
	Fingerprint string
	VideoGroups []ssrcGroupJSON
}

// SourceGroups copies the video SSRC groups for participant.video.
func (o Offer) SourceGroups() []Group {
	out := make([]Group, len(o.VideoGroups))
	for i, g := range o.VideoGroups {
		out[i] = Group{Semantics: g.Semantics, Sources: append([]int32(nil), g.Sources...)}
	}
	return out
}

// Local is this SFU's ICE/DTLS identity for one participant.
type Local struct {
	Ufrag       string
	Pwd         string
	Fingerprint string
	IP          string
	Port        int
	Endpoint    string
}

// ParseOffer reads a tgcalls join payload. A missing ssrc is filled with a
// random non-zero value so the participant still has a source.
func ParseOffer(raw string) (Offer, error) {
	var p joinPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Offer{}, err
	}
	if p.Ufrag == "" || p.Pwd == "" {
		return Offer{}, fmt.Errorf("join payload missing ufrag or pwd")
	}
	fp := ""
	for _, f := range p.Fingerprints {
		if f.Hash == "sha-256" || fp == "" {
			fp = f.Fingerprint
			if f.Hash == "sha-256" {
				break
			}
		}
	}
	ssrc := uint32(p.SSRC)
	if ssrc == 0 {
		var b [4]byte
		_, _ = rand.Read(b[:])
		ssrc = uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
		if ssrc == 0 {
			ssrc = 1
		}
	}
	return Offer{
		AudioSSRC:   ssrc,
		Ufrag:       p.Ufrag,
		Pwd:         p.Pwd,
		Fingerprint: fp,
		VideoGroups: p.SSRCGroups,
	}, nil
}

// AnswerJSON is the updateGroupCallConnection params object.
func AnswerJSON(offer Offer, local Local) (string, error) {
	if local.Ufrag == "" || local.Pwd == "" || local.Fingerprint == "" || local.IP == "" || local.Port == 0 {
		return "", fmt.Errorf("incomplete local transport")
	}
	resp := joinResponse{
		Transport: transportJSON{
			Ufrag: local.Ufrag,
			Pwd:   local.Pwd,
			Fingerprints: []fingerprintJSON{{
				Hash:        "sha-256",
				Fingerprint: local.Fingerprint,
				Setup:       "active",
			}},
			Candidates: []candidateJSON{{
				Port:       strconv.Itoa(local.Port),
				Protocol:   "udp",
				Network:    "0",
				Generation: "0",
				ID:         "tellomi",
				Component:  "1",
				Foundation: "1",
				Priority:   "2130706431",
				IP:         local.IP,
				Type:       "host",
			}},
		},
	}
	if len(offer.VideoGroups) > 0 {
		if local.Endpoint == "" {
			local.Endpoint = "1"
		}
		resp.Video = &videoResponseJSON{
			Endpoint: local.Endpoint,
			PayloadTypes: []videoPayloadTypeJSON{
				{
					ID:        100,
					Name:      "H264",
					Clockrate: 90000,
					Parameters: map[string]string{
						"profile-level-id":   "42e01f",
						"packetization-mode": "1",
					},
					Feedback: []rtcpFeedbackJSON{
						{Type: "goog-remb"},
						{Type: "transport-cc"},
						{Type: "ccm", Subtype: "fir"},
						{Type: "nack"},
						{Type: "nack", Subtype: "pli"},
					},
				},
				{
					ID:         101,
					Name:       "rtx",
					Clockrate:  90000,
					Parameters: map[string]string{"apt": "100"},
				},
			},
			RTPHdrexts: []rtpHdrextJSON{
				{ID: 2, URI: "http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time"},
				{ID: 3, URI: "http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01"},
				{ID: 13, URI: "urn:3gpp:video-orientation"},
			},
		}
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
