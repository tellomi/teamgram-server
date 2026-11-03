package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

// AuthsessionChangeAuthorizationSettings
// authsession.changeAuthorizationSettings user_id:long auth_key_id:long hash:long confirmed:bool set_encrypted_requests_disabled:bool encrypted_requests_disabled:bool set_call_requests_disabled:bool call_requests_disabled:bool = Bool;
func (c *AuthsessionCore) AuthsessionChangeAuthorizationSettings(in *authsession.TLAuthsessionChangeAuthorizationSettings) (*mtproto.Bool, error) {
	if in == nil || in.GetUserId() == 0 || in.GetAuthKeyId() == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}

	permID := in.GetAuthKeyId()
	keyData, err := c.svcCtx.Dao.QueryAuthKeyV2(c.ctx, permID)
	if err != nil {
		c.Logger.Errorf("changeAuthorizationSettings - queryAuthKeyV2(%d) is error: %v", permID, err)
		return nil, err
	} else if bound := keyData.BoundPermAuthKeyId(); bound != 0 {
		permID = bound
	}
	if permID == 0 {
		c.Logger.Errorf("changeAuthorizationSettings - perm auth key is empty")
		return nil, mtproto.ErrAuthKeyPermEmpty
	}

	if err = c.svcCtx.Dao.ChangeAuthorizationSettings(c.ctx, permID, in); err != nil {
		c.Logger.Errorf("changeAuthorizationSettings - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
