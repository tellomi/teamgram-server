package service

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/discoverrel"
	"github.com/teamgram/teamgram-server/pkg/discoverauth"
	"github.com/teamgram/teamgram-server/pkg/enforcement"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func (s *Service) DiscoverGetEngineCredential(ctx context.Context, _ *mtproto.TLDiscoverGetEngineCredential) (*mtproto.Discover_EngineCredential, error) {
	if !discoverauth.Enabled() {
		return nil, disabled()
	}
	userID, clientIP, err := s.userID(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := s.signingKeys()
	if err != nil {
		return nil, disabled()
	}
	if s.accounts == nil {
		logx.Error("discover credential: user service is not configured")
		return nil, status.Error(mtproto.ErrInternal, "INTERNAL")
	}
	acc, err := s.accounts.Account(ctx, userID)
	if err != nil {
		logx.Errorf("discover credential: account lookup failed: %v", err)
		return nil, status.Error(mtproto.ErrInternal, "INTERNAL")
	}
	if acc.Deleted {
		return nil, mtproto.ErrUserIdInvalid
	}
	now := time.Now()
	if s.now != nil {
		now = time.Unix(s.now(), 0)
	}
	if err = enforcement.Reject(userID, "discover_credential", false, now); err != nil {
		return nil, err
	}
	mainland := false
	if s.mainland != nil {
		mainland = s.mainland(clientIP)
	}
	cred, err := discoverauth.Issue(keys.Signing, keys.Pseudonym, userID, acc.CountryCode, acc.Phone, mainland, now)
	if err != nil {
		logx.Errorf("discover credential: issue failed")
		return nil, status.Error(mtproto.ErrInternal, "INTERNAL")
	}
	if err = discoverrel.Remember(ctx, userID, discoverauth.Pseudonym(keys.Pseudonym, userID)); err != nil {
		logx.Errorf("discover relations: could not record subject")
	}
	return mtproto.MakeDiscoverEngineCredential(cred.Token, cred.Region, int32(cred.Expires)), nil
}
