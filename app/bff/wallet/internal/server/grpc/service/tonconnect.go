package service

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/wallet/internal/ledger"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func sessionTL(s ledger.Session) *mtproto.TonConnectSession {
	manifest := mtproto.MakeTLTonConnectManifest(&mtproto.TonConnectManifest{
		Url:  s.ManifestURL,
		Name: s.Name,
	}).To_TonConnectManifest()
	out := mtproto.MakeTLTonConnectSession(&mtproto.TonConnectSession{
		Pending:      s.Pending,
		Closing:      s.Closing,
		Closed:       s.Closed,
		Id:           s.ID,
		DappClientId: s.DappClientID,
		Nonce:        append([]byte(nil), s.Nonce...),
		Manifest:     manifest,
		Date:         s.Date,
	}).To_TonConnectSession()
	if s.ClientID != "" {
		out.ClientId = wrapperspb.String(s.ClientID)
	}
	return out
}

func (s *Service) WalletTonConnectCreateSession(ctx context.Context, in *mtproto.TLWalletTonConnectCreateSession) (*mtproto.TonConnectSession, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	sess, code := s.svcCtx.Ledger.CreateSession(id, in.GetDappClientId(), in.GetManifestUrl(), time.Now())
	if code != "" {
		return nil, fail(code)
	}
	return sessionTL(sess), nil
}

func (s *Service) WalletTonConnectRegisterKey(ctx context.Context, in *mtproto.TLWalletTonConnectRegisterKey) (*mtproto.Wallet_TonConnectChallenge, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	challenge, eventID, code := s.svcCtx.Ledger.RegisterKey(id, in.GetSessionId(), in.GetClientId())
	if code != "" {
		return nil, fail(code)
	}
	return mtproto.MakeTLWalletTonConnectChallenge(&mtproto.Wallet_TonConnectChallenge{
		Challenge: challenge,
		EventId:   eventID,
	}).To_Wallet_TonConnectChallenge(), nil
}

func (s *Service) WalletTonConnectSubmitConnectResult(ctx context.Context, in *mtproto.TLWalletTonConnectSubmitConnectResult) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	code := s.svcCtx.Ledger.SubmitConnect(id, in.GetSessionId(), in.GetError(), in.GetChallengeAnswer(), in.GetBody())
	if code != "" {
		return nil, fail(code)
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) WalletTonConnectGetPending(ctx context.Context, in *mtproto.TLWalletTonConnectGetPending) (*mtproto.Wallet_TonConnectPending, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var sessionID *int64
	if in.GetSessionId() != nil {
		v := in.GetSessionId().GetValue()
		sessionID = &v
	}
	dapp := ""
	if in.GetDappClientId() != nil {
		dapp = in.GetDappClientId().GetValue()
	}
	sess, code := s.svcCtx.Ledger.Pending(id, sessionID, dapp)
	if code != "" {
		return nil, fail(code)
	}
	return mtproto.MakeTLWalletTonConnectPending(&mtproto.Wallet_TonConnectPending{
		Session:  sessionTL(sess),
		Requests: []*mtproto.TonConnectRequest{},
	}).To_Wallet_TonConnectPending(), nil
}

func (s *Service) WalletTonConnectClaimRequest(ctx context.Context, in *mtproto.TLWalletTonConnectClaimRequest) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	code := s.svcCtx.Ledger.Claim(id, in.GetSessionId(), in.GetAppRequestId())
	if code != "" {
		return nil, fail(code)
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) WalletTonConnectSubmitResponse(ctx context.Context, in *mtproto.TLWalletTonConnectSubmitResponse) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	code := s.svcCtx.Ledger.SubmitResponse(id, in.GetSessionId(), in.GetMsgId(), in.GetBody())
	if code != "" {
		return nil, fail(code)
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) WalletTonConnectNextEventId(ctx context.Context, in *mtproto.TLWalletTonConnectNextEventId) (*mtproto.TonConnectNextEventId, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	eventID, code := s.svcCtx.Ledger.NextEvent(id, in.GetSessionId())
	if code != "" {
		return nil, fail(code)
	}
	return mtproto.MakeTLTonConnectNextEventId(&mtproto.TonConnectNextEventId{EventId: eventID}).To_TonConnectNextEventId(), nil
}

func (s *Service) WalletTonConnectCloseSession(ctx context.Context, in *mtproto.TLWalletTonConnectCloseSession) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if code := s.svcCtx.Ledger.Close(id, in.GetSessionId(), in.GetBody()); code != "" {
		return nil, fail(code)
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) WalletTonConnectGetSessions(ctx context.Context, _ *mtproto.TLWalletTonConnectGetSessions) (*mtproto.Wallet_TonConnectSessions, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	rows := s.svcCtx.Ledger.Sessions(id)
	out := make([]*mtproto.TonConnectSession, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionTL(row))
	}
	return mtproto.MakeTLWalletTonConnectSessions(&mtproto.Wallet_TonConnectSessions{
		Sessions: out,
	}).To_Wallet_TonConnectSessions(), nil
}
