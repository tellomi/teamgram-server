package persist

import (
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const defaultRPID = "www.tellomi.cn"

// RPID is TELLOMI_PASSKEY_RPID, or www.tellomi.cn.
func RPID() string {
	if v := strings.TrimSpace(os.Getenv("TELLOMI_PASSKEY_RPID")); v != "" {
		return v
	}
	return defaultRPID
}

// DC is TELLOMI_DC_ID, or 1. The passkey user handle is "<dc>:<user id>".
func DC() int32 {
	v := strings.TrimSpace(os.Getenv("TELLOMI_DC_ID"))
	if v == "" {
		return 1
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 || n > 65535 {
		return 1
	}
	return int32(n)
}

func userHandle(userID int64) string {
	return strconv.FormatInt(int64(DC()), 10) + ":" + strconv.FormatInt(userID, 10)
}

const androidOriginPrefix = "android:apk-key-hash:"

// androidHashes is TELLOMI_PASSKEY_ANDROID_HASHES, a comma-separated list of
// apk certificate hashes. An empty list allows no Android origin.
func androidHashes() map[string]struct{} {
	out := map[string]struct{}{}
	for _, part := range strings.Split(os.Getenv("TELLOMI_PASSKEY_ANDROID_HASHES"), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out[part] = struct{}{}
		if strings.HasPrefix(part, androidOriginPrefix) {
			out[strings.TrimPrefix(part, androidOriginPrefix)] = struct{}{}
		}
	}
	return out
}

// originAllowed is the WebAuthn origin check for this relying party.
// https hosts must be the rpId or a subdomain. An Android origin is allowed
// only when its certificate hash is configured.
func originAllowed(origin string) bool {
	origin = strings.TrimSpace(origin)
	if strings.HasPrefix(origin, androidOriginPrefix) {
		hash := strings.TrimPrefix(origin, androidOriginPrefix)
		if hash == "" || strings.ContainsAny(hash, " ,") {
			return false
		}
		_, ok := androidHashes()[hash]
		return ok
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	rp := strings.ToLower(RPID())
	if host != rp && !strings.HasSuffix(host, "."+rp) {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && (host == "localhost" || host == "127.0.0.1")
}

func fullyQualified(origin string) (string, bool) {
	fq, err := protocol.FullyQualifiedOrigin(origin)
	if err != nil || fq == "" {
		return "", false
	}
	return fq, true
}

func newWebAuthn(origin string) (*webauthn.WebAuthn, error) {
	rp := RPID()
	origins := []string{"https://" + rp}
	// Only an origin that already passed originAllowed is added. A client
	// cannot satisfy the check by naming its own android hash.
	if origin != "" && originAllowed(origin) {
		if fq, ok := fullyQualified(origin); ok {
			found := false
			for _, have := range origins {
				if strings.EqualFold(have, fq) {
					found = true
					break
				}
			}
			if !found {
				origins = append(origins, fq)
			}
		}
	}
	return webauthn.New(&webauthn.Config{
		RPDisplayName:         "Tellomi",
		RPID:                  rp,
		RPOrigins:             origins,
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationRequired,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: time.Minute},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: time.Minute},
		},
	})
}
