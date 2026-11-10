package location

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

const (
	platformIOS     = "ios"
	platformAndroid = "android"
	platformUnknown = "unknown"

	providerApple = "apple"
	providerAMap  = "amap"
)

// SessionHint is the initConnection record stored for an auth key.
type SessionHint struct {
	LangPack      string
	DeviceModel   string
	SystemVersion string
	ApiID         int32
}

type hintBox struct {
	lang, model, system string
	apiID               int32
	authKey             int64
	force               string
	forced              bool
}

type hintKey struct{}

var (
	sessionMu     sync.Mutex
	sessionSource func(context.Context, int64) (SessionHint, bool)
)

// BindSession installs the auth-key lookup used when the RPC metadata has
// no lang_pack. Tests leave it unset.
func BindSession(fn func(context.Context, int64) (SessionHint, bool)) {
	sessionMu.Lock()
	sessionSource = fn
	sessionMu.Unlock()
}

// WithPlatform forces the venue route. Tests use "ios", "android", or "unknown".
func WithPlatform(ctx context.Context, platform string) context.Context {
	return context.WithValue(ctx, hintKey{}, hintBox{force: platform, forced: true})
}

// Classify reports which venue provider a client should use.
// lang_pack "ios" or "android" wins. Otherwise device_model and
// system_version are checked, then the official Telegram api_id
// (8 iOS, 4 and 6 Android). A shared Tellomi api_id does not pick a side.
func Classify(langPack, deviceModel, systemVersion string, apiID int32) string {
	switch strings.ToLower(strings.TrimSpace(langPack)) {
	case "ios":
		return platformIOS
	case "android":
		return platformAndroid
	}
	model := strings.ToLower(strings.TrimSpace(deviceModel))
	system := strings.ToLower(strings.TrimSpace(systemVersion))
	if strings.Contains(model, "iphone") || strings.Contains(model, "ipad") || strings.Contains(model, "ipod") ||
		strings.HasPrefix(system, "ios") || strings.HasPrefix(system, "ipados") {
		return platformIOS
	}
	if strings.Contains(model, "android") || strings.Contains(system, "android") || strings.HasPrefix(system, "sdk") {
		return platformAndroid
	}
	switch apiID {
	case 8:
		return platformIOS
	case 4, 6:
		return platformAndroid
	}
	return platformUnknown
}

func route(platform string, apple, amap bool) []string {
	switch platform {
	case platformIOS:
		if apple {
			return []string{providerApple}
		}
	case platformAndroid:
		if amap {
			return []string{providerAMap}
		}
	default:
		var out []string
		if apple {
			out = append(out, providerApple)
		}
		if amap {
			out = append(out, providerAMap)
		}
		return out
	}
	return nil
}

func platformOf(ctx context.Context) string {
	h, _ := ctx.Value(hintKey{}).(hintBox)
	if h.forced {
		if h.force == "" {
			return platformUnknown
		}
		return h.force
	}
	if md := metadata.RpcMetadataFromIncoming(ctx); md != nil {
		if h.lang == "" {
			h.lang = md.GetLangpack()
		}
		if h.authKey == 0 {
			h.authKey = md.GetPermAuthKeyId()
		}
	}
	if got := Classify(h.lang, h.model, h.system, h.apiID); got != platformUnknown {
		return got
	}
	if h.authKey == 0 {
		return platformUnknown
	}
	sessionMu.Lock()
	fn := sessionSource
	sessionMu.Unlock()
	if fn == nil {
		return platformUnknown
	}
	look, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	extra, ok := fn(look, h.authKey)
	if !ok {
		return platformUnknown
	}
	if h.lang == "" {
		h.lang = extra.LangPack
	}
	if h.model == "" {
		h.model = extra.DeviceModel
	}
	if h.system == "" {
		h.system = extra.SystemVersion
	}
	if h.apiID == 0 {
		h.apiID = extra.ApiID
	}
	return Classify(h.lang, h.model, h.system, h.apiID)
}
