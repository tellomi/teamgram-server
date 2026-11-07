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
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/pkg/code/conf"
	"github.com/teamgram/teamgram-server/pkg/env2"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"

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

// loginCodeNotice is the Simplified Chinese default sent by user 777000.
// Bold spans match Telegram's login-code notice: the label, and "不要".
// The digits carry messageEntitySpoiler, the same entity official Telegram
// puts on the code, so the chat list and notification preview hide it until
// the chat is opened. A non-empty SignInMessage config replaces this text;
// pushSignInMessage still wraps the code in a spoiler.
func loginCodeNotice(code, appName string) (string, []*mtproto.MessageEntity) {
	return mtproto.MakeTextAndMessageEntities([]mtproto.MessageBuildEntry{
		{
			Param:      "登录验证码：",
			EntityType: mtproto.Predicate_messageEntityBold,
		},
		{
			Param:      code,
			EntityType: mtproto.Predicate_messageEntitySpoiler,
		},
		{
			Text:       "。请",
			Param:      "不要",
			EntityType: mtproto.Predicate_messageEntityBold,
		},
		{
			Text: fmt.Sprintf("把此验证码告诉任何人，即使对方声称来自 %s！\n\n此验证码可用于登录你的 %s 账号。我们绝不会以其他理由向你索要它。\n\n如果你并未尝试在其他设备上登录，请忽略此消息。", appName, appName),
		},
	})
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

// shanghaiTime is the clock shown in the 777000 login notice.
var shanghaiTime = time.FixedZone("CST", 8*3600)

// loginNoticeAt suppresses a second notice for the same user and auth key.
// Sending the admin message is not itself a login, and a success path must
// not fan out a duplicate when two handlers both observe the same key.
var (
	loginNoticeMu sync.Mutex
	loginNoticeAt = map[string]int64{}
)

func resetLoginNoticeClaims() {
	loginNoticeMu.Lock()
	loginNoticeAt = map[string]int64{}
	loginNoticeMu.Unlock()
}

// claimLoginNotice is false for the service user and for a repeat of the
// same auth key inside two minutes.
func claimLoginNotice(userID, authKey int64, now time.Time) bool {
	if userID == 0 || userID == 777000 {
		return false
	}
	key := fmt.Sprintf("%d:%d", userID, authKey)
	nowUnix := now.Unix()
	loginNoticeMu.Lock()
	defer loginNoticeMu.Unlock()
	if prev, ok := loginNoticeAt[key]; ok && nowUnix-prev < 120 {
		return false
	}
	loginNoticeAt[key] = nowUnix
	return true
}

// loginDeviceLabel is the client string from initConnection, then langpack.
func loginDeviceLabel(client, langpack string) string {
	client = strings.TrimSpace(client)
	if client != "" {
		return client
	}
	return strings.TrimSpace(langpack)
}

// loginSecurityNotice is the Simplified Chinese 777000 chat message for a
// finished login. It carries the device and Beijing time. It does not carry
// a verification code, a token, or an auth key.
func loginSecurityNotice(device string, when time.Time) (string, []*mtproto.MessageEntity) {
	device = strings.TrimSpace(device)
	if device == "" {
		device = "未知设备"
	}
	stamp := when.In(shanghaiTime).Format("2006-01-02 15:04")
	return mtproto.MakeTextAndMessageEntities([]mtproto.MessageBuildEntry{
		{
			Param:      "新设备登录提醒",
			EntityType: mtproto.Predicate_messageEntityBold,
		},
		{
			Text: fmt.Sprintf("\n你的账号于 %s（北京时间）在 %s 上登录。\n如果这是你本人的操作，可以忽略这条消息。如果不是，请到设置 > 设备中结束该会话。", stamp, device),
		},
	})
}

// pushLoginSecurityNotice writes one in-app message from user 777000 after
// a finished authorization. Failed sign-in, the signup-required result, and
// SESSION_PASSWORD_NEEDED do not call this. The message is not a new-message
// push to the user's contacts.
func (c *AuthorizationCore) pushLoginSecurityNotice(userID int64) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MsgClient == nil || c.MD == nil {
		return
	}
	when := time.Now()
	if !claimLoginNotice(userID, c.MD.PermAuthKeyId, when) {
		return
	}
	device := loginDeviceLabel(c.MD.GetClient(), c.MD.GetLangpack())
	time.AfterFunc(2*time.Second, func() {
		text, entities := loginSecurityNotice(device, when)
		message := mtproto.MakeTLMessage(&mtproto.Message{
			Out:      true,
			Date:     int32(when.Unix()),
			FromId:   mtproto.MakePeerUser(777000),
			PeerId:   mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: userID}).To_Peer(),
			Message:  text,
			Entities: entities,
		}).To_Message()
		c.svcCtx.Dao.MsgClient.MsgPushUserMessage(
			context.Background(),
			&msgpb.TLMsgPushUserMessage{
				UserId:    777000,
				AuthKeyId: 0,
				PeerType:  mtproto.PEER_USER,
				PeerId:    userID,
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

func (c *AuthorizationCore) pushSignInMessage(ctx context.Context, signInUserId int64, code string) {
	time.AfterFunc(2*time.Second, func() {
		// The signup RPC context is already finished. A cancelled ctx made
		// the user lookup fail and the new account look deactivated.
		ctx = context.Background()
		text, entities := loginCodeNotice(code, env2.MyAppName)
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
