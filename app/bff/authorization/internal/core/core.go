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
