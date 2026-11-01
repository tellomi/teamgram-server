package helpdesk

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

// Key is one CDN RSA public key.
// https://core.telegram.org/method/help.getCdnConfig
type Key struct {
	DC  int32
	PEM string
}

// PublicKeys returns configured CDN keys. An empty list is the documented
// config when this server does not publish a secondary CDN.
func PublicKeys() []Key {
	return parseKeys(os.Getenv("TELLOMI_CDN_PUBLIC_KEYS"))
}

func parseKeys(raw string) []Key {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []Key
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		dcText, pem, ok := strings.Cut(part, "|")
		if !ok {
			continue
		}
		dc, err := strconv.Atoi(strings.TrimSpace(dcText))
		pem = strings.TrimSpace(pem)
		if err != nil || dc <= 0 || len(pem) < 32 || !strings.Contains(pem, "BEGIN") {
			logx.Errorf("help.getCdnConfig: skip invalid key entry")
			continue
		}
		out = append(out, Key{DC: int32(dc), PEM: pem})
	}
	return out
}

const maxEvents = 100

// Event is one help.saveAppLog record after validation.
type Event struct {
	UserID int64
	Time   float64
	Type   string
	Peer   int64
	Data   string
}

var (
	logMu  sync.Mutex
	recent []Event
)

// Record validates and keeps client log events.
// https://core.telegram.org/method/help.saveAppLog
// The method may be called before login, so userID may be 0.
func Record(userID int64, events []*mtproto.InputAppEvent) error {
	if len(events) > maxEvents {
		return mtproto.ErrInputRequestInvalid
	}
	var kept []Event
	for _, ev := range events {
		if ev == nil {
			return mtproto.ErrInputRequestInvalid
		}
		typ := strings.TrimSpace(ev.GetType())
		if typ == "" || len(typ) > 128 || ev.GetTime() < 0 {
			return mtproto.ErrInputRequestInvalid
		}
		data := strings.TrimSpace(ev.GetData().GetValue_STRING())
		if len(data) > 1024 {
			return mtproto.ErrInputRequestInvalid
		}
		kept = append(kept, Event{
			UserID: userID,
			Time:   ev.GetTime(),
			Type:   typ,
			Peer:   ev.GetPeer(),
			Data:   data,
		})
	}
	logMu.Lock()
	recent = append(recent, kept...)
	if len(recent) > 50 {
		recent = append([]Event(nil), recent[len(recent)-50:]...)
	}
	logMu.Unlock()
	for _, ev := range kept {
		logx.Infof("help.saveAppLog user=%d time=%g type=%s peer=%d data=%s", ev.UserID, ev.Time, ev.Type, ev.Peer, ev.Data)
	}
	return nil
}

// Recent is the last recorded events. Tests use it.
func Recent() []Event {
	logMu.Lock()
	defer logMu.Unlock()
	out := make([]Event, len(recent))
	copy(out, recent)
	return out
}

// ResetRecent clears the in-process log. Tests use it.
func ResetRecent() {
	logMu.Lock()
	recent = nil
	logMu.Unlock()
}
