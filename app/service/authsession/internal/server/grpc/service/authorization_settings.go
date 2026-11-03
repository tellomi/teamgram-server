package service

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/core"
)

// AuthsessionChangeAuthorizationSettings
// authsession.changeAuthorizationSettings user_id:long auth_key_id:long hash:long = Bool;
func (s *Service) AuthsessionChangeAuthorizationSettings(ctx context.Context, request *authsession.TLAuthsessionChangeAuthorizationSettings) (*authsession.ChangeAuthorizationSettingsResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("authsession.changeAuthorizationSettings - metadata: {%s}, request: {%s}", c.MD, request)

	if _, err := c.AuthsessionChangeAuthorizationSettings(request); err != nil {
		return nil, err
	}
	return &authsession.ChangeAuthorizationSettingsResult{Ok: true}, nil
}
