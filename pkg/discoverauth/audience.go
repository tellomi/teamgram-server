package discoverauth

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const (
	// AudienceTyp is the typ claim that separates an audience list from the
	// feed credential and from a friend assertion.
	AudienceTyp = "tellomi-feed-audience"
	// AudienceKindInclude is visibility "选定用户". AudienceKindExclude is
	// visibility "排除部分人".
	AudienceKindInclude = "include"
	AudienceKindExclude = "exclude"

	// AudienceTTL is the lifetime this issuer puts in exp. The engine accepts
	// an audience token for at most AudienceMaxTTL (content-engine posting
	// contract: exp at most 15 minutes ahead; the list it documents as issued
	// is 10 minutes).
	AudienceTTL    = 10 * time.Minute
	AudienceMaxTTL = 15 * time.Minute

	// AudienceMaxMembers is the engine's member cap.
	AudienceMaxMembers = 500
)

// ErrAudienceInvalid is a rejected kind, an empty list, more than 500
// members, a non-positive id, a duplicate, or the owner inside the list.
var ErrAudienceInvalid = errors.New("discoverauth: audience invalid")

// AudienceClaims is the audience JWT body. Field order is the order
// content-engine assertion.go signs: typ, exp, owner, kind, members.
type AudienceClaims struct {
	Typ     string   `json:"typ"`
	Exp     int64    `json:"exp"`
	Owner   string   `json:"owner"`
	Kind    string   `json:"kind"`
	Members []string `json:"members"`
}

// Audience is what discover.getAudienceToken returns, minus the TL box.
type Audience struct {
	Token   string
	Expires int64
}

// ValidateAudience checks the caller's id, kind, and member ids before any
// user lookup. It does not check that the users exist or that they are blocked.
func ValidateAudience(ownerID int64, kind string, memberIDs []int64) error {
	if ownerID <= 0 || (kind != AudienceKindInclude && kind != AudienceKindExclude) {
		return ErrAudienceInvalid
	}
	if len(memberIDs) == 0 || len(memberIDs) > AudienceMaxMembers {
		return ErrAudienceInvalid
	}
	seen := make(map[int64]struct{}, len(memberIDs))
	for _, id := range memberIDs {
		if id <= 0 || id == ownerID {
			return ErrAudienceInvalid
		}
		if _, ok := seen[id]; ok {
			return ErrAudienceInvalid
		}
		seen[id] = struct{}{}
	}
	return nil
}

// SignAudience builds an audience JWT. owner and each member are 16-byte
// pseudonyms. exp is unix seconds. The signing key is the same raw HMAC key
// as the feed credential.
func SignAudience(signingKey, owner []byte, kind string, members [][]byte, exp int64) (string, error) {
	if len(owner) != pseudonymLen || exp <= 0 || (kind != AudienceKindInclude && kind != AudienceKindExclude) {
		return "", ErrAudienceInvalid
	}
	if len(members) == 0 || len(members) > AudienceMaxMembers {
		return "", ErrAudienceInvalid
	}
	claims := AudienceClaims{
		Typ:     AudienceTyp,
		Exp:     exp,
		Owner:   EncodeSub(owner),
		Kind:    kind,
		Members: make([]string, len(members)),
	}
	seen := make(map[string]struct{}, len(members)+1)
	seen[claims.Owner] = struct{}{}
	for i, member := range members {
		if len(member) != pseudonymLen {
			return "", ErrAudienceInvalid
		}
		sub := EncodeSub(member)
		if _, ok := seen[sub]; ok {
			return "", ErrAudienceInvalid
		}
		seen[sub] = struct{}{}
		claims.Members[i] = sub
	}
	body, err := marshalAudience(claims)
	if err != nil {
		return "", err
	}
	signing := headerB64 + "." + base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write([]byte(signing))
	return signing + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// IssueAudience maps user ids through Pseudonym and signs a token that
// expires AudienceTTL after now, truncated to a whole second.
func IssueAudience(signingKey, pseudonymKey []byte, ownerID int64, kind string, memberIDs []int64, now time.Time) (Audience, error) {
	if err := ValidateAudience(ownerID, kind, memberIDs); err != nil {
		return Audience{}, err
	}
	members := make([][]byte, len(memberIDs))
	for i, id := range memberIDs {
		members[i] = Pseudonym(pseudonymKey, id)
	}
	exp := now.Add(AudienceTTL).Truncate(time.Second).Unix()
	if exp > now.Add(AudienceMaxTTL).Unix() {
		return Audience{}, ErrAudienceInvalid
	}
	token, err := SignAudience(signingKey, Pseudonym(pseudonymKey, ownerID), kind, members, exp)
	if err != nil {
		return Audience{}, err
	}
	return Audience{Token: token, Expires: exp}, nil
}

// VerifyAudience checks an audience token the way the engine checks the
// credential: exact header, HS256, no unknown claims, and exp inside the
// 60s leeway. exp more than AudienceMaxTTL plus that leeway ahead of now is
// rejected. Member existence is not re-checked here.
func VerifyAudience(signingKey []byte, token string, now time.Time) (AudienceClaims, error) {
	header, payload, sig, ok := splitToken(token)
	if !ok || header != headerB64 {
		return AudienceClaims{}, errBadToken
	}
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write([]byte(header + "." + payload))
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, mac.Sum(nil)) {
		return AudienceClaims{}, errBadToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return AudienceClaims{}, errBadToken
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var claims AudienceClaims
	if err = dec.Decode(&claims); err != nil {
		return AudienceClaims{}, errBadToken
	}
	var extra any
	if err = dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return AudienceClaims{}, errBadToken
	}
	if claims.Typ != AudienceTyp || (claims.Kind != AudienceKindInclude && claims.Kind != AudienceKindExclude) {
		return AudienceClaims{}, errBadToken
	}
	owner, err := base64.RawURLEncoding.DecodeString(claims.Owner)
	if err != nil || len(owner) != pseudonymLen {
		return AudienceClaims{}, errBadToken
	}
	if len(claims.Members) == 0 || len(claims.Members) > AudienceMaxMembers {
		return AudienceClaims{}, errBadToken
	}
	seen := map[string]struct{}{claims.Owner: {}}
	for _, member := range claims.Members {
		rawMember, err := base64.RawURLEncoding.DecodeString(member)
		if err != nil || len(rawMember) != pseudonymLen {
			return AudienceClaims{}, errBadToken
		}
		if _, ok := seen[member]; ok {
			return AudienceClaims{}, errBadToken
		}
		seen[member] = struct{}{}
	}
	if claims.Exp <= 0 || now.After(time.Unix(claims.Exp, 0).Add(ExpLeeway)) {
		return AudienceClaims{}, errBadToken
	}
	if claims.Exp > now.Add(AudienceMaxTTL).Add(ExpLeeway).Unix() {
		return AudienceClaims{}, errBadToken
	}
	return claims, nil
}

func marshalAudience(c AudienceClaims) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
