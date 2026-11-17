// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"context"
	"math/rand"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/authorization/loginnotice"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/pkg/code/conf"
	"github.com/teamgram/teamgram-server/pkg/env2"
	"github.com/teamgram/teamgram-server/pkg/langpack"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
	"github.com/teamgram/teamgram-server/pkg/usercopy"

	"github.com/zeromicro/go-zero/core/logx"
)

type AuthorizationCore struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	MD *metadata.RpcMetadata
}

func New(ctx context.Context, svcCtx *svc.ServiceContext) *AuthorizationCore {
	return &AuthorizationCore{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		MD:     metadata.RpcMetadataFromIncoming(ctx),
	}
}

func checkPhoneNumberInvalid(phone string) (string, string, error) {
	return phonenumber.CheckPhoneNumberInvalid(phone)
}

// loginCodeNotice is the 777000 login-code message in the client's language.
// Simplified Chinese keeps the previous wording. The label and the warning
// word are bold. The digits carry messageEntitySpoiler, the same entity
// official Telegram puts on the code, so the chat list and notification
// preview hide it until the chat is opened. A non-empty SignInMessage config
// replaces this text; pushSignInMessage still wraps the code in a spoiler.
func loginCodeNotice(lang, code, appName string) (string, []*mtproto.MessageEntity) {
	return usercopy.LoginCode(lang, code, appName)
}

func (c *AuthorizationCore) clientLang() string {
	if c == nil || c.MD == nil {
		return "en"
	}
	return langpack.ClientLang(c.MD.GetLangCode(), c.MD.GetSystemLangCode(), c.MD.GetLangpack())
}

// ensureLoginCodeSpoiler covers code with messageEntitySpoiler when a custom
// SignInMessage left the digits in plain text. Official clients mask spoiler
// spans in the dialog list and in the notification preview.
func ensureLoginCodeSpoiler(text, code string, entities []*mtproto.MessageEntity) []*mtproto.MessageEntity {
	if code == "" || text == "" {
		return entities
	}
	byteAt := strings.Index(text, code)
	if byteAt < 0 {
		return entities
	}
	off := int32(mtproto.UTF16Len(text[:byteAt]))
	length := int32(mtproto.UTF16Len(code))
	if length <= 0 {
		return entities
	}
	for _, e := range entities {
		if e.GetPredicateName() != mtproto.Predicate_messageEntitySpoiler && e.GetConstructor() != mtproto.CRC32_messageEntitySpoiler {
			continue
		}
		if e.GetOffset() <= off && e.GetOffset()+e.GetLength() >= off+length {
			return entities
		}
	}
	return append(entities, mtproto.MakeTLMessageEntitySpoiler(&mtproto.MessageEntity{
		Offset: off,
		Length: length,
	}).To_MessageEntity())
}

func resetLoginNoticeClaims() { loginnotice.Reset() }

func claimLoginNotice(userID, authKey int64, now time.Time) bool {
	return loginnotice.Claim(userID, authKey, now)
}

func loginDeviceLabel(client, langpack string) string {
	return loginnotice.Label(client, langpack)
}

func loginNoticeDevice(model, client, langpack string) string {
	return loginnotice.Device(model, client, langpack)
}

// loginDevice reads the device model for this permanent key. A lookup
// failure falls back to the lang pack so the notice is still sent. The
// lookup uses its own timeout: auth.signUp runs the notice after the RPC
// context is already finished.
func (c *AuthorizationCore) loginDevice() string {
	client, langpack := "", ""
	if c != nil && c.MD != nil {
		client = c.MD.GetClient()
		langpack = c.MD.GetLangpack()
	}
	model := ""
	if c != nil && c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.AuthsessionClient != nil && c.MD != nil && c.MD.PermAuthKeyId != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		auth, err := c.svcCtx.Dao.AuthsessionGetAuthorization(ctx, &authsession.TLAuthsessionGetAuthorization{
			AuthKeyId: c.MD.PermAuthKeyId,
		})
		cancel()
		if err == nil && auth != nil {
			model = auth.GetDeviceModel()
		}
	}
	return loginNoticeDevice(model, client, langpack)
}

func loginSecurityNotice(device string, when time.Time) (string, []*mtproto.MessageEntity) {
	return loginnotice.Text(device, when)
}

// pushLoginSecurityNotice writes one in-app message from user 777000 after
// a finished authorization. Failed sign-in, the signup-required result, and
// SESSION_PASSWORD_NEEDED do not call this. QR login without a cloud password
// is sent by auth.acceptLoginToken, through the same claim map.
func (c *AuthorizationCore) pushLoginSecurityNotice(userID int64) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MsgClient == nil || c.MD == nil {
		return
	}
	loginnotice.Push(c.svcCtx.Dao.MsgClient, userID, c.MD.PermAuthKeyId, c.clientLang(), c.loginDevice(), time.Now())
}

func (c *AuthorizationCore) pushSignInMessage(ctx context.Context, signInUserId int64, code string) {
	lang := c.clientLang()
	time.AfterFunc(2*time.Second, func() {
		// The signup RPC context is already finished. A cancelled ctx made
		// the user lookup fail and the new account look deactivated.
		ctx = context.Background()
		text, entities := loginCodeNotice(lang, code, env2.MyAppName)
		message := mtproto.MakeTLMessage(&mtproto.Message{
			Out:      true,
			Date:     int32(time.Now().Unix()),
			FromId:   mtproto.MakePeerUser(777000),
			PeerId:   mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: signInUserId}).To_Peer(),
			Message:  text,
			Entities: entities,
		}).To_Message()

		if len(c.svcCtx.Config.SignInMessage) > 0 {
			builder := conf.ToMessageBuildHelper(
				c.svcCtx.Config.SignInMessage,
				map[string]interface{}{
					"code":     code,
					"app_name": env2.MyAppName,
				})
			message.Message, message.Entities = mtproto.MakeTextAndMessageEntities(builder)
		}
		message.Entities = ensureLoginCodeSpoiler(message.Message, code, message.Entities)

		c.svcCtx.Dao.MsgClient.MsgPushUserMessage(
			ctx,
			&msgpb.TLMsgPushUserMessage{
				UserId:    777000,
				AuthKeyId: 0,
				PeerType:  mtproto.PEER_USER,
				PeerId:    signInUserId,
				PushType:  1,
				Message: msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
					NoWebpage:    false,
					Background:   false,
					RandomId:     rand.Int63(),
					Message:      message,
					ScheduleDate: nil,
				}).To_OutboxMessage(),
			})
	})
}
