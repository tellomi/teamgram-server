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

package sess

import (
	"context"
	"encoding/binary"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	rpcmetadata "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/pkg/enforcement"

	"github.com/zeromicro/go-zero/core/contextx"
	"github.com/zeromicro/go-zero/core/logx"
)

// decodeWrappedQuery reads the inner RPC of invokeWithLayer and the other
// wrappers. A constructor the codec does not know is an RPC error, not a
// dropped request.
func (c *session) decodeWrappedQuery(ctx context.Context, gatewayId string, msgId *inboxMsg, raw []byte) mtproto.TLObject {
	dBuf := mtproto.NewDecodeBuf(raw)
	query := dBuf.Object()
	if err := dBuf.GetError(); err != nil || query == nil {
		if err == nil {
			err = mtproto.ErrInputRequestInvalid
		}
		c.rejectUndecodedQuery(ctx, gatewayId, msgId, raw, err)
		return nil
	}
	return query
}

func (c *session) rejectUndecodedQuery(ctx context.Context, gatewayId string, msgId *inboxMsg, raw []byte, err error) {
	ctor := "unknown"
	if len(raw) >= 4 {
		id := int32(binary.LittleEndian.Uint32(raw[:4]))
		ctor = fmt.Sprintf("0x%08x", uint32(id))
		if name := mtproto.ConstructorName(id); name != "" {
			ctor += " " + name
		}
	}
	reqID := int64(0)
	seqno := int32(0)
	if msgId != nil {
		reqID = msgId.msgId
		seqno = msgId.seqNo
	}
	logx.WithContext(ctx).Errorf("undecoded query - msg_id: %d, seqno: %d, constructor: %s, err: %v", reqID, seqno, ctor, err)
	if msgId == nil || msgId.msgId == 0 {
		return
	}
	c.sendRpcResultToQueue(ctx, gatewayId, msgId.msgId, mtproto.NewRpcError(decodeFailureStatus(err)))
	msgId.state = RECEIVED | RESPONSE_GENERATED
}

func (c *session) onInvokeWithLayer(ctx context.Context, gatewayId, clientIp string, msgId *inboxMsg, request *mtproto.TLInvokeWithLayer) {
	logx.WithContext(ctx).Infof("onInvokeWithLayer - request data: {sess: %s, conn_id: %s, msg_id: %d, seq_no: %d, request: {%s}}",
		c,
		gatewayId,
		msgId.msgId,
		msgId.seqNo,
		request)

	if request.GetQuery() == nil {
		logx.WithContext(ctx).Errorf("invokeWithLayer Query is nil, query: {%s}", request)
		return
	}

	query := c.decodeWrappedQuery(ctx, gatewayId, msgId, request.Query)
	if query == nil {
		return
	}

	c.sessList.cb.onUpdateLayer(ctx, request.Layer)

	c.processMsg(ctx, gatewayId, clientIp, msgId, query)
}

func (c *session) onInvokeAfterMsg(ctx context.Context, gatewayId, clientIp string, msgId *inboxMsg, request *mtproto.TLInvokeAfterMsg) {
	logx.WithContext(ctx).Infof("onInvokeAfterMsg - request data: {sess: %s, conn_id: %s, msg_id: %d, seq_no: %d, request: {%v}}",
		c,
		gatewayId,
		msgId.msgId,
		msgId.seqNo,
		request)

	if request.GetQuery() == nil {
		logx.WithContext(ctx).Errorf("invokeAfterMsg Query is nil, query: {%s}", request)
		return
	}

	query := c.decodeWrappedQuery(ctx, gatewayId, msgId, request.Query)
	if query == nil {
		return
	}

	c.processMsg(ctx, gatewayId, clientIp, msgId, query)
}

func (c *session) onInvokeAfterMsgs(ctx context.Context, gatewayId, clientIp string, msgId *inboxMsg, request *mtproto.TLInvokeAfterMsgs) {
	logx.WithContext(ctx).Infof("onInvokeAfterMsgs - request data: {sess: %s, conn_id: %s, msg_id: %d, seq_no: %d, request: {%v}}",
		c,
		gatewayId,
		msgId.msgId,
		msgId.seqNo,
		request)

	if request.GetQuery() == nil {
		logx.WithContext(ctx).Errorf("invokeAfterMsgs Query is nil, query: {%s}", request)
		return
	}

	query := c.decodeWrappedQuery(ctx, gatewayId, msgId, request.Query)
	if query == nil {
		return
	}

	/*
		if invokeAfterMsgs.GetQuery() == nil {
			logx.Errorf("invokeAfterMsgs Query is nil, query: {%v}", invokeAfterMsgs)
			return
		}

		dbuf := mtproto.NewDecodeBuf(invokeAfterMsgs.Query)
		query := dbuf.Object()
		if query == nil {
			logx.Errorf("Decode query error: %s", hex.EncodeToString(invokeAfterMsgs.Query))
			return
		}

		if len(invokeAfterMsgs.MsgIds) == 0 {
			// TODO(@benqi): invalid msgIds, ignore??

			messages[i].Object = query
		} else {
			var maxMsgId = invokeAfterMsgs.MsgIds[0]
			for j := 1; j < len(invokeAfterMsgs.MsgIds); j++ {
				if maxMsgId > invokeAfterMsgs.MsgIds[j] {
					maxMsgId = invokeAfterMsgs.MsgIds[j]
				}
			}


			var found = false
			for j := 0; j < i; j++ {
				if messages[j].MsgId == maxMsgId {
					messages[i].Object = query
					found = true
					break
				}
			}

			if !found {
				for j := i + 1; j < len(messages); j++ {
					if messages[j].MsgId == maxMsgId {
						// c.messages[i].Object = query
						messages[i].Object = query
						found = true
						messages = append(messages, messages[i])

						// set messages[i] = nil, will ignore this.
						messages[i] = nil
						break
					}
				}
			}

			if !found {
				// TODO(@benqi): backup message, wait.

				messages[i].Object = query
			}
		}
	*/

	c.processMsg(ctx, gatewayId, clientIp, msgId, query)
}

func (c *session) onInvokeWithoutUpdates(ctx context.Context, gatewayId, clientIp string, msgId *inboxMsg, request *mtproto.TLInvokeWithoutUpdates) {
	logx.WithContext(ctx).Infof("onInvokeWithoutUpdates - request data: {sess: %s, conn_id: %s, msg_id: %d, seq_no: %d, request: {%s}}",
		c,
		gatewayId,
		msgId.msgId,
		msgId.seqNo,
		reflect.TypeOf(request))

	if request.GetQuery() == nil {
		logx.WithContext(ctx).Errorf("invokeWithoutUpdates Query is nil, query: {%s}", request)
		return
	}

	query := c.decodeWrappedQuery(ctx, gatewayId, msgId, request.Query)
	if query == nil {
		return
	}

	c.processMsg(ctx, gatewayId, clientIp, msgId, query)
}

func (c *session) onInvokeWithMessagesRange(ctx context.Context, gatewayId, clientIp string, msgId *inboxMsg, request *mtproto.TLInvokeWithMessagesRange) {
	logx.WithContext(ctx).Infof("onInvokeWithMessagesRange - request data: {sess: %s, conn_id: %s, msg_id: %d, seq_no: %d, request: {%s}}",
		c,
		gatewayId,
		msgId.msgId,
		msgId.seqNo,
		reflect.TypeOf(request))

	if request.GetQuery() == nil {
		logx.WithContext(ctx).Errorf("invokeWithMessagesRange Query is nil, query: {%s}", request)
		return
	}

	query := c.decodeWrappedQuery(ctx, gatewayId, msgId, request.Query)
	if query == nil {
		return
	}

	c.onRpcRequest(ctx, gatewayId, clientIp, msgId, query, newTakeoutMetadata(0, request.GetRange()))
}

func (c *session) onInvokeWithTakeout(ctx context.Context, gatewayId, clientIp string, msgId *inboxMsg, request *mtproto.TLInvokeWithTakeout) {
	logx.WithContext(ctx).Infof("onInvokeWithTakeout - request data: {sess: %s, conn_id: %s, msg_id: %d, seq_no: %d, request: {%s}}",
		c,
		gatewayId,
		msgId.msgId,
		msgId.seqNo,
		reflect.TypeOf(request))

	if request.GetQuery() == nil {
		logx.WithContext(ctx).Errorf("invokeWithTakeout Query is nil, query: {%s}", request)
		return
	}

	query := c.decodeWrappedQuery(ctx, gatewayId, msgId, request.Query)
	if query == nil {
		return
	}

	query, takeout, err := newTakeoutGuard().ValidateWrappedQuery(request.GetTakeoutId(), query)
	if err != nil {
		if rpcErr, ok := err.(*mtproto.TLRpcError); ok {
			c.sendRpcResultToQueue(ctx, gatewayId, msgId.msgId, rpcErr)
			msgId.state = RECEIVED | RESPONSE_GENERATED
			return
		}

		c.sendRpcResultToQueue(ctx, gatewayId, msgId.msgId, mtproto.NewRpcError(mtproto.ErrInternalServerError))
		msgId.state = RECEIVED | RESPONSE_GENERATED
		return
	}

	c.onRpcRequest(ctx, gatewayId, clientIp, msgId, query, takeout)
}

func (c *session) onInvokeWithBusinessConnection(ctx context.Context, gatewayId, clientIp string, msgId *inboxMsg, request *mtproto.TLInvokeWithBusinessConnection) {
	logx.WithContext(ctx).Infof("onInvokeWithBusinessConnection - request data: {sess: %s, conn_id: %s, msg_id: %d, seq_no: %d, request: {%s}}",
		c,
		gatewayId,
		msgId.msgId,
		msgId.seqNo,
		reflect.TypeOf(request))

	if request.GetQuery() == nil {
		logx.WithContext(ctx).Errorf("invokeWithBusinessConnection Query is nil, query: {%s}", request)
		return
	}

	query := c.decodeWrappedQuery(ctx, gatewayId, msgId, request.Query)
	if query == nil {
		return
	}

	c.processMsg(ctx, gatewayId, clientIp, msgId, query)
}

func (c *session) onInvokeWithGooglePlayIntegrity(ctx context.Context, gatewayId, clientIp string, msgId *inboxMsg, request *mtproto.TLInvokeWithGooglePlayIntegrity) {
	logx.WithContext(ctx).Infof("onInvokeWithGooglePlayIntegrity - request data: {sess: %s, conn_id: %s, msg_id: %d, seq_no: %d, nonce: %s, token: %s, request: {%s}}",
		c,
		gatewayId,
		msgId.msgId,
		msgId.seqNo,
		request.Nonce,
		request.Token,
		reflect.TypeOf(request))

	if request.GetQuery() == nil {
		logx.WithContext(ctx).Errorf("onInvokeWithGooglePlayIntegrity Query is nil, query: {%s}", request)
		return
	}

	query := c.decodeWrappedQuery(ctx, gatewayId, msgId, request.Query)
	if query == nil {
		return
	}

	c.processMsg(ctx, gatewayId, clientIp, msgId, query)
}

// onInvokeWithApnsSecret
func (c *session) onInvokeWithApnsSecret(ctx context.Context, gatewayId, clientIp string, msgId *inboxMsg, request *mtproto.TLInvokeWithApnsSecret) {
	logx.WithContext(ctx).Infof("onInvokeWithApnsSecret - request data: {sess: %s, conn_id: %s, msg_id: %d, seq_no: %d, nonce: %s, secret: %s, request: {%s}}",
		c,
		gatewayId,
		msgId.msgId,
		msgId.seqNo,
		request.Nonce,
		request.Secret,
		reflect.TypeOf(request))

	if request.GetQuery() == nil {
		logx.WithContext(ctx).Errorf("invokeWithApnsSecret Query is nil, query: {%s}", request)
		return
	}

	query := c.decodeWrappedQuery(ctx, gatewayId, msgId, request.Query)
	if query == nil {
		return
	}

	c.processMsg(ctx, gatewayId, clientIp, msgId, query)
}

// onInvokeWithReCaptcha
func (c *session) onInvokeWithReCaptcha(ctx context.Context, gatewayId, clientIp string, msgId *inboxMsg, request *mtproto.TLInvokeWithReCaptcha) {
	logx.WithContext(ctx).Infof("onInvokeWithReCaptcha - request data: {sess: %s, conn_id: %s, msg_id: %d, seq_no: %d, nonce: %s, secret: %s, request: {%s}}",
		c,
		gatewayId,
		msgId.msgId,
		msgId.seqNo,
		request.Token,
		reflect.TypeOf(request))

	if request.GetQuery() == nil {
		logx.WithContext(ctx).Errorf("onInvokeWithReCaptcha Query is nil, query: {%s}", request)
		return
	}

	query := c.decodeWrappedQuery(ctx, gatewayId, msgId, request.Query)
	if query == nil {
		return
	}

	c.processMsg(ctx, gatewayId, clientIp, msgId, query)
}

func (c *session) onInitConnection(ctx context.Context, gatewayId, clientIp string, msgId *inboxMsg, request *mtproto.TLInitConnection) {
	logx.WithContext(ctx).Infof("onInitConnection - request data: {sess: %s, conn_id: %s, msg_id: %d, seq_no: %d, request: {%s}}",
		c,
		gatewayId,
		msgId.msgId,
		msgId.msgId,
		reflect.TypeOf(request))

	c.sessList.cb.onUpdateInitConnection(ctx, clientIp, request)

	query := c.decodeWrappedQuery(ctx, gatewayId, msgId, request.GetQuery())
	if query == nil {
		return
	}

	c.processMsg(ctx, gatewayId, clientIp, msgId, query)
}

func (c *session) onRpcRequest(ctx context.Context, gatewayId, clientIp string, msgId *inboxMsg, query mtproto.TLObject, takeout *rpcmetadata.Takeout) bool {
	logx.WithContext(ctx).Infof("onRpcRequest - request data: {sess: %s, gatewayId: %s, msg_id: %d, seq_no: %d, request: {%s}}",
		c,
		gatewayId,
		msgId.msgId,
		msgId.seqNo,
		reflect.TypeOf(query))

	// TODO(@benqi): sync AuthUserId??
	//requestMessage := &mtproto.TLMessage2{
	//	MsgId:  msgId.msgId,
	//	Seqno:  msgId.seqNo,
	//	Object: object,
	//}

	switch query.(type) {
	case *mtproto.TLAccountRegisterDevice:
		registerDevice, _ := query.(*mtproto.TLAccountRegisterDevice)
		if registerDevice.TokenType == 7 {
			pushSessionId, _ := strconv.ParseInt(registerDevice.GetToken(), 10, 64)
			c.sessList.cb.onBindPushSessionId(ctx, c.sessList, pushSessionId)
		}
	case *mtproto.TLUpdatesGetState:
		c.sessList.cb.onSetMainUpdatesSession(ctx, c)
		c.canSync = false
	case *mtproto.TLUpdatesGetDifference:
		c.sessList.cb.onSetMainUpdatesSession(ctx, c)
		c.canSync = false
		//
	case *mtproto.TLUpdatesGetChannelDifference:
		c.sessList.cb.onSetMainUpdatesSession(ctx, c)
		c.canSync = false
		//case *mtproto.TLAuthBindTempAuthKey:
		//	res, err := c.AuthSessionRpcClient.AuthBindTempAuthKey(context.Background(), query.(*mtproto.TLAuthBindTempAuthKey))
		//	if err != nil {
		//		logx.WithContext(ctx).Errorf("bindTempAuthKey error - %v", err)
		//		err = mtproto.ErrInternalServerError
		//		c.sendRpcResultToQueue(gatewayId, msgId.msgId, &mtproto.RpcError{
		//			ErrorCode:    500,
		//			ErrorMessage: "INTERNAL_SERVER_ERROR",
		//		})
		//	} else {
		//		c.sendRpcResultToQueue(gatewayId, msgId.msgId, res)
		//	}
		//	msgId.state = RECEIVED | RESPONSE_GENERATED
		//	return false

		//case *mtproto.TLAuthSignIn:
		//	if !c.isGeneric {
		//		c.isGeneric = true
		//		c.cb.setOnline()
		//	}
		//case *mtproto.TLAuthSignUp:
		//	if !c.isGeneric {
		//		c.isGeneric = true
		//		c.cb.setOnline()
		//	}
	case *mtproto.TLAccountUpdateStatus:
		c.sessList.cb.onSetMainUpdatesSession(ctx, c)
	case *mtproto.TLUsersGetUsers:
		// logx.WithContext(ctx).Infof("user.getUsers: %s", query)
	}

	if shouldRefreshAuthState(c.sessList.cb.state) {
		c.sessList.cb.refreshAuthStateIfNeeded(ctx)
	}

	switch c.sessList.cb.state {
	case mtproto.AuthStateNormal:
		if c.sessList.cb.bindingRevoked(ctx) {
			c.replyUnauthorized(ctx, gatewayId, msgId.msgId)
			msgId.state = RECEIVED | RESPONSE_GENERATED
			return false
		}
		if enforcement.OrdinarySessionBlocked(c.sessList.cb.AuthUserId, time.Now()) {
			c.sendRpcResultToQueue(ctx, gatewayId, msgId.msgId, mtproto.NewRpcError(mtproto.ErrAccountPermanentDisabled))
			msgId.state = RECEIVED | RESPONSE_GENERATED
			return false
		}
	case mtproto.AuthStateNeedPassword:
		if c.sessList.cb.bindingRevoked(ctx) {
			c.replyUnauthorized(ctx, gatewayId, msgId.msgId)
			msgId.state = RECEIVED | RESPONSE_GENERATED
			return false
		}
		if !checkRpcWithoutLogin(query) {
			c.sendRpcResultToQueue(ctx, gatewayId, msgId.msgId, mtproto.NewRpcError(mtproto.ErrSessionPasswordNeeded))
			msgId.state = RECEIVED | RESPONSE_GENERATED
			return false
		}
	default:
		if !checkRpcWithoutLogin(query) {
			if authStateRevoked(c.sessList.cb.state) {
				c.replyUnauthorized(ctx, gatewayId, msgId.msgId)
			} else {
				c.sendRpcResultToQueue(ctx, gatewayId, msgId.msgId, mtproto.NewRpcError(rpcErrorForAuthState(c.sessList.cb.state)))
			}
			msgId.state = RECEIVED | RESPONSE_GENERATED
			return false
		}
	}

	if d := c.floodDecision(ctx, clientIp, query); !d.Allowed {
		c.sendRpcResultToQueue(ctx, gatewayId, msgId.msgId, mtproto.NewRpcError(mtproto.NewErrFloodWaitX(d.WaitSec)))
		msgId.state = RECEIVED | RESPONSE_GENERATED
		return false
	}

	msgId.state = RECEIVED | RPC_PROCESSING

	c.sessList.cb.tmpRpcApiMessageList = append(
		c.sessList.cb.tmpRpcApiMessageList,
		&rpcApiMessage{
			ctx:       contextx.ValueOnlyFrom(ctx),
			sessList:  c.sessList,
			sessionId: c.sessionId,
			clientIp:  clientIp,
			reqMsgId:  msgId.msgId,
			reqMsg:    query,
			takeout:   takeout,
		})

	return true
}

// passwordNeededUser reads a legacy SESSION_PASSWORD_NEEDED_<id> suffix.
// An exact SESSION_PASSWORD_NEEDED carries no id; boundUser is the id already
// stored on the auth key.
func passwordNeededUser(message string, boundUser int64) (int64, bool) {
	const prefix = "SESSION_PASSWORD_NEEDED_"
	switch {
	case strings.HasPrefix(message, prefix):
		id, _ := strconv.ParseInt(message[len(prefix):], 10, 64)
		return id, true
	case message == "SESSION_PASSWORD_NEEDED":
		return boundUser, true
	default:
		return 0, false
	}
}

// acceptPasswordNeeded rewrites the legacy suffixed error and puts the session
// in the password step. The user id comes from the suffix or from the auth-key
// binding written before the RPC returned.
func (c *session) acceptPasswordNeeded(ctx context.Context, rpcErr *mtproto.TLRpcError) {
	if rpcErr == nil {
		return
	}
	msg := rpcErr.Message()
	id, ok := passwordNeededUser(msg, 0)
	if !ok {
		return
	}
	if id == 0 && msg == "SESSION_PASSWORD_NEEDED" {
		id = c.boundUserID(ctx)
	}
	rpcErr.SetErrorMessage("SESSION_PASSWORD_NEEDED")
	if id != 0 {
		c.onlySetNeedPasswordFromError(ctx, id)
	}
}

func (c *session) boundUserID(ctx context.Context) int64 {
	if c == nil || c.sessList == nil || c.sessList.cb == nil || c.sessList.cb.cb == nil || c.sessList.cb.cb.Dao == nil {
		return 0
	}
	if c.sessList.cb.cb.Dao.AuthsessionClient == nil {
		return 0
	}
	got, err := c.sessList.cb.cb.Dao.AuthsessionGetUserId(ctx, &authsession.TLAuthsessionGetUserId{
		AuthKeyId: c.sessList.cb.authKeyId,
	})
	if err != nil || got == nil {
		return 0
	}
	return got.GetV()
}

// onlySetNeedPasswordFromError moves Logout, Unknown, and New (and any other
// non-normal state) onto the password step. The out-of-order guard is only
// Normal: a delayed SESSION_PASSWORD_NEEDED must not demote a key that
// already passed auth.checkPassword. auth.logOut on the same permanent key
// leaves the wrapper in Logout; the next auth.signIn still has to enter
// NeedPassword or account.getPassword is invoked with user id 0.
func (c *session) onlySetNeedPasswordFromError(ctx context.Context, hackId int64) {
	if c.sessList.cb.state == mtproto.AuthStateNormal {
		return
	}
	c.sessList.cb.changeAuthState(ctx, mtproto.AuthStateNeedPassword, hackId)
}

func (c *session) onRpcResult(ctx context.Context, rpcResult *rpcApiMessage) {
	rpcErr, _ := rpcResult.TryGetRpcResultError()
	if rpcErr != nil && rpcErr.GetErrorCode() == int32(mtproto.ErrNotReturnClient) {
		logx.WithContext(ctx).Debugf("recv NOTRETURN_CLIENT")
		c.pendingQueue.Add(rpcResult.reqMsgId)
		return
	}

	hasCanSync := false
	switch request := rpcResult.reqMsg.(type) {
	case *mtproto.TLAuthBindTempAuthKey:
		if rpcErr != nil {
			_ = request
			if rpcErr.Message() == "ENCRYPTED_MESSAGE_INVALID" {
				c.sessList.cb.changeAuthState(ctx, mtproto.AuthStateUnknown, 0)
			}
		} else {
			c.sessList.changeAuthState(mtproto.AuthStatePermBound)
		}
	case *mtproto.TLAuthLogOut:
		c.sessList.cb.changeAuthState(ctx, mtproto.AuthStateLogout, 0)
	case *mtproto.TLAuthSendCode:
		if rpcErr == nil && rpcResult.rpcResult != nil && rpcResult.rpcResult.Result != nil {
			authSentCode, _ := rpcResult.rpcResult.Result.(*mtproto.Auth_SentCode)
			if authSentCode != nil && authSentCode.GetPredicateName() == mtproto.Predicate_auth_sentCodeSuccess {
				if auth := authSentCode.GetAuthorization(); auth != nil && auth.GetUser() != nil {
					c.sessList.cb.changeAuthState(ctx, mtproto.AuthStateNormal, auth.GetUser().GetId())
				}
			}
		} else if rpcErr != nil {
			c.acceptPasswordNeeded(ctx, rpcErr)
		}
	case *mtproto.TLAuthExportLoginToken:
		if rpcErr == nil && rpcResult.rpcResult != nil && rpcResult.rpcResult.Result != nil {
			authLoginToken, _ := rpcResult.rpcResult.Result.(*mtproto.Auth_LoginToken)
			if authLoginToken != nil && authLoginToken.GetPredicateName() == mtproto.Predicate_auth_loginTokenSuccess {
				if auth := authLoginToken.GetAuthorization(); auth != nil && auth.GetUser() != nil {
					c.sessList.cb.changeAuthState(ctx, mtproto.AuthStateNormal, auth.GetUser().GetId())
				}
			}
		} else if rpcErr != nil {
			c.acceptPasswordNeeded(ctx, rpcErr)
		}
	case *mtproto.TLAuthImportLoginToken:
		if rpcErr != nil {
			c.acceptPasswordNeeded(ctx, rpcErr)
		}
	case *mtproto.TLAuthSignIn:
		if rpcErr == nil && rpcResult.rpcResult != nil && rpcResult.rpcResult.Result != nil {
			authAuthorization, _ := rpcResult.rpcResult.Result.(*mtproto.Auth_Authorization)
			if authAuthorization != nil && authAuthorization.GetPredicateName() == mtproto.Predicate_auth_authorization && authAuthorization.GetUser() != nil {
				c.sessList.cb.changeAuthState(ctx, mtproto.AuthStateNormal, authAuthorization.GetUser().GetId())
			}
		} else if rpcErr != nil {
			c.acceptPasswordNeeded(ctx, rpcErr)
		}
	case *mtproto.TLAuthSignUp:
		if rpcErr == nil && rpcResult.rpcResult != nil && rpcResult.rpcResult.Result != nil {
			authAuthorization, _ := rpcResult.rpcResult.Result.(*mtproto.Auth_Authorization)
			if authAuthorization != nil && authAuthorization.GetPredicateName() == mtproto.Predicate_auth_authorization && authAuthorization.GetUser() != nil {
				c.sessList.cb.changeAuthState(ctx, mtproto.AuthStateNormal, authAuthorization.GetUser().GetId())
			}
		}
	case *mtproto.TLAuthFinishPasskeyLogin:
		if rpcErr == nil && rpcResult.rpcResult != nil && rpcResult.rpcResult.Result != nil {
			authAuthorization, _ := rpcResult.rpcResult.Result.(*mtproto.Auth_Authorization)
			if authAuthorization != nil && authAuthorization.GetPredicateName() == mtproto.Predicate_auth_authorization && authAuthorization.GetUser() != nil {
				c.sessList.cb.changeAuthState(ctx, mtproto.AuthStateNormal, authAuthorization.GetUser().GetId())
			}
		}
	case *mtproto.TLAuthImportAuthorization:
		if rpcErr == nil && c.sessList.cb.state == mtproto.AuthStateNeedPassword && rpcResult.rpcResult != nil && rpcResult.rpcResult.Result != nil {
			authAuthorization, _ := rpcResult.rpcResult.Result.(*mtproto.Auth_Authorization)
			if authAuthorization != nil && authAuthorization.GetPredicateName() == mtproto.Predicate_auth_authorization && authAuthorization.GetUser() != nil {
				c.sessList.cb.changeAuthState(ctx, mtproto.AuthStateNormal, authAuthorization.GetUser().GetId())
			}
		}
	case *mtproto.TLAuthCheckPassword:
		// Success from Logout or any other non-normal state still finishes
		// the login. A key that is already Normal is left alone so a late
		// result cannot switch the user.
		if rpcErr == nil && c.sessList.cb.state != mtproto.AuthStateNormal && rpcResult.rpcResult != nil && rpcResult.rpcResult.Result != nil {
			authAuthorization, _ := rpcResult.rpcResult.Result.(*mtproto.Auth_Authorization)
			if authAuthorization != nil && authAuthorization.GetPredicateName() == mtproto.Predicate_auth_authorization && authAuthorization.GetUser() != nil {
				c.sessList.cb.changeAuthState(ctx, mtproto.AuthStateNormal, authAuthorization.GetUser().GetId())
			}
		}
	case *mtproto.TLAuthRecoverPassword:
		if rpcErr == nil && c.sessList.cb.state == mtproto.AuthStateNeedPassword && rpcResult.rpcResult != nil && rpcResult.rpcResult.Result != nil {
			authAuthorization, _ := rpcResult.rpcResult.Result.(*mtproto.Auth_Authorization)
			if authAuthorization != nil && authAuthorization.GetPredicateName() == mtproto.Predicate_auth_authorization && authAuthorization.GetUser() != nil {
				c.sessList.cb.changeAuthState(ctx, mtproto.AuthStateNormal, authAuthorization.GetUser().GetId())
			}
		}
	case *mtproto.TLUpdatesGetState:
		hasCanSync = true
	case *mtproto.TLUpdatesGetDifference:
		hasCanSync = true
	case *mtproto.TLUpdatesGetChannelDifference:
		hasCanSync = true
	default:
	}

	// MoveRpcResult() clears rpcResult.rpcResult; duplicate delivery would get nil — guard to avoid sendRpcResult(nil).
	rpcResultData := rpcResult.MoveRpcResult()
	if rpcResultData != nil {
		c.sendRpcResult(ctx, rpcResultData)
	}
	if hasCanSync {
		c.canSync = true
		gatewayId := c.getGatewayId()
		c.sendQueueToGateway(ctx, gatewayId)
	}
}

func (c *session) sendRpcResult(ctx context.Context, rpcResult *mtproto.TLRpcResult) {
	if rpcResult == nil {
		return
	}
	msgId := c.inQueue.Lookup(rpcResult.ReqMsgId)
	if msgId == nil {
		logx.WithContext(ctx).Errorf("not found msgId, maybe removed: %d", rpcResult.ReqMsgId)
		return
	}

	gatewayId := c.getGatewayId()
	c.sendRpcResultToQueue(ctx, gatewayId, msgId.msgId, rpcResult.Result)
	msgId.state = RECEIVED | ACKNOWLEDGED

	if gatewayId == "" {
		logx.WithContext(ctx).Errorf("gatewayId is empty, send delay...")
	} else {
		c.sendQueueToGateway(ctx, gatewayId)
	}
}
