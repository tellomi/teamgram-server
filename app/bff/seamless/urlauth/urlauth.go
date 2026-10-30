// Package urlauth answers Seamless Telegram Login RPCs.
//
// This server does not host bot OAuth clients, so a well-formed link does not
// ask the user to authorize a bot. messages.requestUrlAuth returns
// urlAuthResultDefault and the client opens the URL itself.
//
// https://core.telegram.org/api/url-authorization
// https://core.telegram.org/method/messages.requestUrlAuth
package urlauth

import (
	"net/url"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Request is messages.requestUrlAuth.
func Request(peer *mtproto.InputPeer, msgID, buttonID *wrapperspb.Int32Value, rawURL, origin string) (*mtproto.UrlAuthResult, error) {
	if err := validate(peer, msgID, buttonID, rawURL, origin); err != nil {
		return nil, err
	}
	return mtproto.MakeTLUrlAuthResultDefault(&mtproto.UrlAuthResult{}).To_UrlAuthResult(), nil
}

// Accept is messages.acceptUrlAuth. A plain URL is opened as given.
// https://core.telegram.org/method/messages.acceptUrlAuth
func Accept(peer *mtproto.InputPeer, msgID, buttonID *wrapperspb.Int32Value, rawURL, origin string) (*mtproto.UrlAuthResult, error) {
	if err := validate(peer, msgID, buttonID, rawURL, origin); err != nil {
		return nil, err
	}
	if strings.TrimSpace(rawURL) == "" {
		return mtproto.MakeTLUrlAuthResultDefault(&mtproto.UrlAuthResult{}).To_UrlAuthResult(), nil
	}
	return mtproto.MakeTLUrlAuthResultAccepted(&mtproto.UrlAuthResult{
		Url_FLAGSTRING: &wrapperspb.StringValue{Value: strings.TrimSpace(rawURL)},
	}).To_UrlAuthResult(), nil
}

// Decline is messages.declineUrlAuth.
// https://core.telegram.org/method/messages.declineUrlAuth
func Decline(rawURL string) error {
	if !validURL(rawURL) {
		return mtproto.ErrUrlInvalid
	}
	return nil
}

// Match is messages.checkUrlAuthMatchCode. No match codes are issued, so a
// valid URL never matches.
// https://core.telegram.org/method/messages.checkUrlAuthMatchCode
func Match(rawURL, code string) (bool, error) {
	if !validURL(rawURL) {
		return false, mtproto.ErrUrlInvalid
	}
	_ = code
	return false, nil
}

// WebAuthorizations is account.getWebAuthorizations. There are no stored
// bot web sessions.
// https://core.telegram.org/method/account.getWebAuthorizations
func WebAuthorizations() *mtproto.Account_WebAuthorizations {
	return mtproto.MakeTLAccountWebAuthorizations(&mtproto.Account_WebAuthorizations{
		Authorizations: []*mtproto.WebAuthorization{},
		Users:          []*mtproto.User{},
	}).To_Account_WebAuthorizations()
}

// ResetWebAuthorization is account.resetWebAuthorization. No hash is stored.
// https://core.telegram.org/method/account.resetWebAuthorization
func ResetWebAuthorization(hash int64) error {
	_ = hash
	return mtproto.ErrHashInvalid
}

func validate(peer *mtproto.InputPeer, msgID, buttonID *wrapperspb.Int32Value, rawURL, origin string) error {
	if origin != "" && !validOrigin(origin) {
		return mtproto.ErrUrlInvalid
	}
	hasURL := strings.TrimSpace(rawURL) != ""
	hasButton := peer != nil || msgID != nil || buttonID != nil
	if !hasURL && !hasButton {
		return mtproto.ErrUrlInvalid
	}
	if hasURL && !validURL(rawURL) {
		return mtproto.ErrUrlInvalid
	}
	if hasButton {
		if peer == nil || peer.GetPredicateName() == "" || peer.GetPredicateName() == mtproto.Predicate_inputPeerEmpty {
			return mtproto.ErrUrlInvalid
		}
		if msgID == nil || msgID.GetValue() <= 0 || buttonID == nil || buttonID.GetValue() < 0 {
			return mtproto.ErrUrlInvalid
		}
	}
	return nil
}

func validURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return u.Host != ""
	case "tg":
		return u.Host != "" || u.Opaque != ""
	default:
		return false
	}
}

func validOrigin(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return u.Path == "" || u.Path == "/"
}
