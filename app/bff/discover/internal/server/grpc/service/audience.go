package service

import (
	"context"
	"errors"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/discoverrel"
	"github.com/teamgram/teamgram-server/pkg/discoverauth"
	"github.com/teamgram/teamgram-server/pkg/enforcement"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

var errLookup = errors.New("discover audience: lookup")

func (s *Service) DiscoverGetAudienceToken(ctx context.Context, in *mtproto.TLDiscoverGetAudienceToken) (*mtproto.Discover_AudienceToken, error) {
	if !discoverauth.Enabled() {
		return nil, disabled()
	}
	userID, _, err := s.userID(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = s.signingKeys(); err != nil {
		return nil, disabled()
	}
	if in == nil {
		in = &mtproto.TLDiscoverGetAudienceToken{}
	}
	if err = discoverauth.ValidateAudience(userID, in.GetKind(), in.GetUserIds()); err != nil {
		return nil, audienceInvalid()
	}
	if s.accounts == nil {
		logx.Error("discover audience: user service is not configured")
		return nil, status.Error(mtproto.ErrInternal, "INTERNAL")
	}
	acc, err := s.accounts.Account(ctx, userID)
	if err != nil {
		logx.Errorf("discover audience: account lookup failed: %v", err)
		return nil, status.Error(mtproto.ErrInternal, "INTERNAL")
	}
	if acc.Deleted {
		return nil, mtproto.ErrUserIdInvalid
	}
	if s.members == nil {
		logx.Error("discover audience: member directory is not configured")
		return nil, status.Error(mtproto.ErrInternal, "INTERNAL")
	}
	views, err := s.members.Lookup(ctx, userID, in.GetUserIds())
	if err != nil {
		logx.Errorf("discover audience: member lookup failed: %v", err)
		return nil, status.Error(mtproto.ErrInternal, "INTERNAL")
	}
	for _, id := range in.GetUserIds() {
		view, ok := views[id]
		if !ok || !view.Live {
			return nil, mtproto.ErrUserIdInvalid
		}
		if view.Blocked {
			return nil, mtproto.ErrUserIsBlocked
		}
	}
	keys, err := s.signingKeys()
	if err != nil {
		return nil, disabled()
	}
	now := time.Now()
	if s.now != nil {
		now = time.Unix(s.now(), 0)
	}
	if err = enforcement.Reject(userID, "discover_credential", false, now); err != nil {
		return nil, err
	}
	issued, err := discoverauth.IssueAudience(keys.Signing, keys.Pseudonym, userID, in.GetKind(), in.GetUserIds(), now)
	if err != nil {
		logx.Errorf("discover audience: issue failed")
		return nil, status.Error(mtproto.ErrInternal, "INTERNAL")
	}
	if err = discoverrel.Remember(ctx, userID, discoverauth.Pseudonym(keys.Pseudonym, userID)); err != nil {
		logx.Errorf("discover relations: could not record subject")
	}
	return mtproto.MakeDiscoverAudienceToken(issued.Token, int32(issued.Expires)), nil
}

func audienceInvalid() error {
	return status.Error(mtproto.ErrBadRequest, "AUDIENCE_INVALID")
}
