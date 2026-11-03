/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/sess"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

type SessionCore struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	MD *metadata.RpcMetadata
}

func New(ctx context.Context, svcCtx *svc.ServiceContext) *SessionCore {
	return &SessionCore{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		MD:     metadata.RpcMetadataFromIncoming(ctx),
	}
}

func (c *SessionCore) getOrFetchMainAuthWrapper(mainAuthId int64) (*sess.MainAuthWrapper, error) {
	// Perm id 0 is an unbound temp key. One shared wrapper would answer
	// auth.signIn with SESSION_PASSWORD_NEEDED and then account.getPassword
	// with no SRP material, which is the iOS first-login dead end.
	if mainAuthId == 0 {
		return nil, mtproto.ErrAuthKeyPermEmpty
	}
	mainAuth := c.svcCtx.MainAuthMgr.GetMainAuthWrapper(mainAuthId)
	if mainAuth != nil {
		return mainAuth, nil
	}

	kData, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionGetAuthStateData(c.ctx, &authsession.TLAuthsessionGetAuthStateData{
		AuthKeyId: mainAuthId,
	})
	if err != nil {
		if status.Convert(err).Message() != "AUTH_KEY_PERM_EMPTY" {
			c.Logger.Errorf("getOrFetchMainAuthWrapper - error: %v", err)
			return nil, err
		}
		// Not bound yet. A wrapper under this key's own id can answer
		// ping, bad_server_salt and new_session_created. Id 0 stays rejected
		// above, so these connections do not share a session.
		kData = authsession.MakeTLAuthKeyStateData(&authsession.AuthKeyStateData{
			AuthKeyId: mainAuthId,
			KeyState:  mtproto.AuthStateNew,
		}).To_AuthKeyStateData()
	}

	mainAuth = c.svcCtx.MainAuthMgr.AllocMainAuthWrapper(
		mainAuthId,
		func(authKeyId int64) *sess.MainAuthWrapper {
			return sess.NewMainAuthWrapper(
				mainAuthId,
				kData.UserId,
				int(kData.KeyState),
				kData.Client,
				kData.GetAndroidPushSessionId().GetValue(),
				c.svcCtx.MainAuthMgr)
		})

	return mainAuth, nil
}
