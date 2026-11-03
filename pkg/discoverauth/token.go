// Package discoverauth issues the content-engine credential.
//
// The engine (tellomi/content-engine internal/engine/auth) checks the JWT and
// does not recompute the pseudonym. This package follows the bytes that
// engine auth.Sign produced at content-engine 4483302, as recorded by
// Signal-Server TellomiFeedTokens: a fixed header, compact claims in the
// order sub, rgn, exp, and HS256 over that signing input.
package discoverauth

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	// RegionCN and RegionGlobal are the only rgn values the engine credential
	// carries. "cn" is mainland China. Everything else is "global".
	RegionCN     = "cn"
	RegionGlobal = "global"

	// CredentialTTL is the engine access-token lifetime (Signal-Server
	// TellomiFeedController.CREDENTIAL_LIFETIME).
	CredentialTTL = time.Hour

	// ExpLeeway is the engine's clock skew on exp (60s, noted next to the
	// deletion sweep that waits out a live credential).
	ExpLeeway = 60 * time.Second

	pseudonymLen = 16
)

// headerB64 is base64url( {"alg":"HS256","typ":"JWT"} ) with no padding.
// The engine compares this segment byte for byte.
const headerB64 = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"

var (
	errBadToken = errors.New("discoverauth: token rejected")
	errBadSub   = errors.New("discoverauth: sub must be 16 bytes")
	errBadRgn   = errors.New("discoverauth: region")
	errBadUser  = errors.New("discoverauth: user id")
)

// Claims are the credential body. Sub is the base64url (no padding) form of
// the 16-byte pseudonym.
type Claims struct {
	Sub string `json:"sub"`
	Rgn string `json:"rgn"`
	Exp int64  `json:"exp"`
}

// Pseudonym is HMAC-SHA256(pseudonymKey, decimal user id) truncated to 16
// bytes. The decimal form is the UTF-8 encoding of the base-10 int64 with
// no sign and no leading zeros. The engine never sees the user id; it only
// stores this pseudonym as sub.
func Pseudonym(pseudonymKey []byte, userID int64) []byte {
	mac := hmac.New(sha256.New, pseudonymKey)
	_, _ = mac.Write([]byte(strconv.FormatInt(userID, 10)))
	sum := mac.Sum(nil)
	out := make([]byte, pseudonymLen)
	copy(out, sum)
	return out
}

// EncodeSub is the sub claim: base64url of the 16-byte pseudonym, no padding.
// The engine decodes it with base64.RawURLEncoding.
func EncodeSub(pseudonym []byte) string {
	return base64.RawURLEncoding.EncodeToString(pseudonym)
}

// Sign builds a credential JWT. pseudonym must be 16 bytes. exp is unix
// seconds. The signing key is the raw HMAC key, the same bytes as the
// engine's first ENGINE_FEED_AUTH_KEYS entry.
func Sign(signingKey, pseudonym []byte, region string, exp int64) (string, error) {
	if len(pseudonym) != pseudonymLen {
		return "", errBadSub
	}
	if region != RegionCN && region != RegionGlobal {
		return "", errBadRgn
	}
	if exp <= 0 {
		return "", errBadToken
	}
	body, err := marshalClaims(Claims{Sub: EncodeSub(pseudonym), Rgn: region, Exp: exp})
	if err != nil {
		return "", err
	}
	signing := headerB64 + "." + base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write([]byte(signing))
	return signing + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// Issue signs a credential for a logged-in user. now is truncated the same
// way as the engine's issuer: expiration is now+1h, then whole seconds.
func Issue(signingKey, pseudonymKey []byte, userID int64, countryCode, phone string, mainlandIP bool, now time.Time) (Credential, error) {
	if userID <= 0 {
		return Credential{}, errBadUser
	}
	region := ContentRegion(countryCode, phone, mainlandIP)
	exp := now.Add(CredentialTTL).Truncate(time.Second).Unix()
	token, err := Sign(signingKey, Pseudonym(pseudonymKey, userID), region, exp)
	if err != nil {
		return Credential{}, err
	}
	return Credential{Token: token, Expires: exp, Region: region}, nil
}

// Credential is what discover.getEngineCredential returns, minus the TL box.
type Credential struct {
	Token   string
	Expires int64
	Region  string
}

// Verify checks a credential the way content-engine auth.go does: exact
// header, HS256, claims {sub,rgn,exp} with no extras, sub decoding to 16
// bytes, rgn of cn or global, and exp still inside the 60s leeway.
func Verify(signingKey []byte, token string, now time.Time) (Claims, error) {
	header, payload, sig, ok := splitToken(token)
	if !ok || header != headerB64 {
		return Claims{}, errBadToken
	}
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write([]byte(header + "." + payload))
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, mac.Sum(nil)) {
		return Claims{}, errBadToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Claims{}, errBadToken
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var claims Claims
	if err = dec.Decode(&claims); err != nil {
		return Claims{}, errBadToken
	}
	var extra any
	if err = dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return Claims{}, errBadToken
	}
	sub, err := base64.RawURLEncoding.DecodeString(claims.Sub)
	if err != nil || len(sub) != pseudonymLen {
		return Claims{}, errBadToken
	}
	if claims.Rgn != RegionCN && claims.Rgn != RegionGlobal {
		return Claims{}, errBadToken
	}
	if claims.Exp <= 0 || now.After(time.Unix(claims.Exp, 0).Add(ExpLeeway)) {
		return Claims{}, errBadToken
	}
	return claims, nil
}

func marshalClaims(c Claims) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func splitToken(token string) (header, payload, sig string, ok bool) {
	header, rest, ok1 := strings.Cut(token, ".")
	payload, sig, ok2 := strings.Cut(rest, ".")
	if !ok1 || !ok2 || header == "" || payload == "" || sig == "" || strings.Contains(sig, ".") {
		return "", "", "", false
	}
	return header, payload, sig, true
}
