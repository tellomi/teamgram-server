// Package seamless serves RPCSeamless: URL authorization and the account web
// authorization list. Bot OAuth apps are not configured, so login prompts are
// not issued.
package seamless

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/seamless/urlauth"
)

// Service is the bff RPCSeamless server.
type Service struct {
	mtproto.UnimplementedRPCSeamlessServer
}

// New returns the seamless handler set.
func New() *Service { return &Service{} }

func userID(ctx context.Context) error {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil || md.GetUserId() == 0 {
		return mtproto.ErrAuthKeyUnregistered
	}
	return nil
}

// AccountGetWebAuthorizations
// account.getWebAuthorizations#182e6d6f = account.WebAuthorizations;
func (s *Service) AccountGetWebAuthorizations(ctx context.Context, _ *mtproto.TLAccountGetWebAuthorizations) (*mtproto.Account_WebAuthorizations, error) {
	if err := userID(ctx); err != nil {
		return nil, err
	}
	return urlauth.WebAuthorizations(), nil
}

// AccountResetWebAuthorization
// account.resetWebAuthorization#2d01b9ef hash:long = Bool;
func (s *Service) AccountResetWebAuthorization(ctx context.Context, in *mtproto.TLAccountResetWebAuthorization) (*mtproto.Bool, error) {
	if err := userID(ctx); err != nil {
		return nil, err
	}
	if err := urlauth.ResetWebAuthorization(in.GetHash()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// AccountResetWebAuthorizations
// account.resetWebAuthorizations#682d2594 = Bool;
func (s *Service) AccountResetWebAuthorizations(ctx context.Context, _ *mtproto.TLAccountResetWebAuthorizations) (*mtproto.Bool, error) {
	if err := userID(ctx); err != nil {
		return nil, err
	}
	_ = urlauth.WebAuthorizations()
	return mtproto.BoolTrue, nil
}

// MessagesRequestUrlAuth
// messages.requestUrlAuth#894cc99c flags:# peer:flags.1?InputPeer msg_id:flags.1?int button_id:flags.1?int url:flags.2?string in_app_origin:flags.3?string = UrlAuthResult;
func (s *Service) MessagesRequestUrlAuth(ctx context.Context, in *mtproto.TLMessagesRequestUrlAuth) (*mtproto.UrlAuthResult, error) {
	if err := userID(ctx); err != nil {
		return nil, err
	}
	return urlauth.Request(in.GetPeer(), in.GetMsgId(), in.GetButtonId(), in.GetUrl().GetValue(), in.GetInAppOrigin().GetValue())
}

// MessagesAcceptUrlAuth
// messages.acceptUrlAuth#18dd4e92 flags:# write_allowed:flags.0?true share_phone_number:flags.3?true peer:flags.1?InputPeer msg_id:flags.1?int button_id:flags.1?int url:flags.2?string match_code:flags.4?string tonconnect_session:flags.5?InputTonConnectOauthSession = UrlAuthResult;
func (s *Service) MessagesAcceptUrlAuth(ctx context.Context, in *mtproto.TLMessagesAcceptUrlAuth) (*mtproto.UrlAuthResult, error) {
	if err := userID(ctx); err != nil {
		return nil, err
	}
	return urlauth.Accept(in.GetPeer(), in.GetMsgId(), in.GetButtonId(), in.GetUrl().GetValue(), "")
}

// MessagesDeclineUrlAuth
// messages.declineUrlAuth#35436bbc url:string = Bool;
func (s *Service) MessagesDeclineUrlAuth(ctx context.Context, in *mtproto.TLMessagesDeclineUrlAuth) (*mtproto.Bool, error) {
	if err := userID(ctx); err != nil {
		return nil, err
	}
	if err := urlauth.Decline(in.GetUrl()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// MessagesCheckUrlAuthMatchCode
// messages.checkUrlAuthMatchCode#c9a47b0b url:string match_code:string = Bool;
func (s *Service) MessagesCheckUrlAuthMatchCode(ctx context.Context, in *mtproto.TLMessagesCheckUrlAuthMatchCode) (*mtproto.Bool, error) {
	if err := userID(ctx); err != nil {
		return nil, err
	}
	ok, err := urlauth.Match(in.GetUrl(), in.GetMatchCode())
	if err != nil {
		return nil, err
	}
	if ok {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}
