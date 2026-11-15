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

// originAllowed is the WebAuthn origin check for this relying party.
// https hosts must be the rpId or a subdomain. Android app origins are allowed.
func originAllowed(origin string) bool {
	origin = strings.TrimSpace(origin)
	if strings.HasPrefix(origin, "android:apk-key-hash:") && len(origin) > len("android:apk-key-hash:") {
		return true
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

func newWebAuthn(extraOrigin string) (*webauthn.WebAuthn, error) {
	rp := RPID()
	origins := []string{"https://" + rp}
	if fq, ok := fullyQualified(extraOrigin); ok {
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
	return webauthn.New(&webauthn.Config{
		RPDisplayName:         "Tellomi",
		RPID:                  rp,
		RPOrigins:             origins,
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationPreferred,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: time.Minute},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: time.Minute},
		},
	})
}
