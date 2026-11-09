// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package sess

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/marmota/pkg/queue2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/status/status"

	"github.com/zeromicro/go-zero/core/contextx"
	"github.com/zeromicro/go-zero/core/jsonx"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/syncx"
	"github.com/zeromicro/go-zero/core/threading"
)

var (
	// ErrDataChannelFull sessionDataChan 缓冲满时返回此错误，让调用方感知背压
	ErrDataChannelFull = errors.New("session data channel is full")
)

type SessionList struct {
	authId        int64
	authType      int
	state         int
	cacheSalt     *mtproto.TLFutureSalt
	cacheLastSalt *mtproto.TLFutureSalt
	sessions      map[int64]*session
	cb            *MainAuthWrapper
}

func newSessionList(kType int, cb *MainAuthWrapper) *SessionList {
	authId := int64(0)
	if kType == mtproto.AuthKeyTypePerm {
		authId = cb.authKeyId
	}
	return &SessionList{
		authId:        authId,
		authType:      kType,
		state:         0,
		cacheSalt:     nil,
		cacheLastSalt: nil,
		sessions:      make(map[int64]*session),
		cb:            cb,
	}
}

func (s *SessionList) Reset(authId int64) (lastAuthId int64) {
	lastAuthId = s.authId

	s.authId = authId
	s.state = 0
	s.cacheSalt = nil
	s.cacheLastSalt = nil
	s.sessions = make(map[int64]*session)

	return
}

func (s *SessionList) destroySession(sessionId int64) bool {
	// TODO(@benqi):
	if sess, ok := s.sessions[sessionId]; ok {
		if sess.httpQueue != nil {
			sess.httpQueue.Clear()
		}
		// s.updates.onGenericSessionClose(sess)
		delete(s.sessions, sessionId)
	} else {
		//
	}
	return true
}

func (s *SessionList) changeAuthState(state int) {
	s.state = state
}

type MainAuthWrapper struct {
	authKeyId            int64
	state                int
	AuthUserId           int64 // 不为0，则signIn
	client               *authsession.ClientSession
	pushSessionId        int64
	mainAuth             *SessionList
	tempAuth             *SessionList
	mediaTempAuth        *SessionList
	mainUpdatesSession   *session
	androidPushSession   *session
	closeChan            chan struct{}
	sessionDataChan      chan interface{} // receive from client
	rpcDataChan          chan interface{}
	rpcQueue             *queue2.SyncQueue
	finish               sync.WaitGroup
	running              *syncx.AtomicBool
	onlineExpired        int64
	clientType           int
	nextNotifyId         int64
	nextPushId           int64
	cb                   *MainAuthWrapperManager
	tmpRpcApiMessageList []*rpcApiMessage
}

func NewMainAuthWrapper(mainAuthKeyId int64, authUserId int64, state int, client *authsession.ClientSession, androidPushId int64, cb *MainAuthWrapperManager) *MainAuthWrapper {
	mainAuth := &MainAuthWrapper{
		authKeyId:            mainAuthKeyId,
		state:                state,
		AuthUserId:           authUserId,
		client:               client,
		pushSessionId:        androidPushId,
		mainAuth:             nil,
		tempAuth:             nil,
		mediaTempAuth:        nil,
		mainUpdatesSession:   nil,
		androidPushSession:   nil,
		clientType:           clientUnknown,
		nextPushId:           0,
		nextNotifyId:         math.MaxInt32,
		closeChan:            make(chan struct{}),
		sessionDataChan:      make(chan interface{}, 4096),
		rpcDataChan:          make(chan interface{}, 4096),
		rpcQueue:             queue2.NewSyncQueue(),
		finish:               sync.WaitGroup{},
		running:              syncx.NewAtomicBool(),
		cb:                   cb,
		tmpRpcApiMessageList: make([]*rpcApiMessage, 0),
	}
	mainAuth.tempAuth = newSessionList(mtproto.AuthKeyTypeTemp, mainAuth)
	mainAuth.mediaTempAuth = newSessionList(mtproto.AuthKeyTypeMediaTemp, mainAuth)
	mainAuth.mainAuth = newSessionList(mtproto.AuthKeyTypePerm, mainAuth)

	mainAuth.Start()
	return mainAuth
}

func (m *MainAuthWrapper) changeAuthState(ctx context.Context, state int, stateData interface{}) {
	m.state = state

	switch state {
	case mtproto.AuthStateUnknown:
		// m.delOnline(ctx)
		// m.AuthUserId = 0
		m.cb.DeleteByAuthKeyId(m.authKeyId)
		m.Stop()
	case mtproto.AuthStateLogout:
		// m.delOnline(ctx)
		// m.AuthUserId = 0
		m.cb.DeleteByAuthKeyId(m.authKeyId)
		m.Stop()
	case mtproto.AuthStateDeleted:
		// m.delOnline(ctx)
		// m.AuthUserId = 0
		m.cb.DeleteByAuthKeyId(m.authKeyId)
		m.Stop()
	case mtproto.AuthStateNeedPassword:
		m.AuthUserId = stateData.(int64)
	case mtproto.AuthStateNormal:
		newUserId := stateData.(int64)
		if m.AuthUserId != 0 && m.AuthUserId != newUserId {
			// 一条 authKey 生命周期内出现多个 userId，视为异常：打高危日志但保留旧值，避免 owner 悄然漂移。
			logx.WithContext(ctx).Errorf("changeAuthState - authKeyId %d switch user from %d to %d, keep original",
				m.authKeyId, m.AuthUserId, newUserId)
			return
		}
		m.AuthUserId = newUserId
	default:
		m.AuthUserId = 0
	}
}

func (m *MainAuthWrapper) applyAuthStateData(ctx context.Context, kData *authsession.AuthKeyStateData) {
	if kData == nil {
		return
	}

	if client := kData.GetClient(); client != nil {
		m.client = client
	}
	if pushId := kData.GetAndroidPushSessionId(); pushId != nil {
		m.pushSessionId = pushId.GetValue()
	}

	switch nextState := int(kData.GetKeyState()); nextState {
	case mtproto.AuthStateNormal, mtproto.AuthStateNeedPassword:
		m.changeAuthState(ctx, nextState, kData.GetUserId())
	case mtproto.AuthStateUnknown, mtproto.AuthStateLogout, mtproto.AuthStateDeleted:
		m.changeAuthState(ctx, nextState, int64(0))
	default:
		m.state = nextState
		m.AuthUserId = 0
	}
}

// bindingRevoked reloads a Normal session. AuthStateNormal used to stay
// trusted for the life of the socket, so a missed updateAccountResetAuthorization
// left the device logged in after its auth_users row was deleted.
// A lookup error keeps the session: authsession being down must not log
// every connection out. Logout and Deleted close it.
func (m *MainAuthWrapper) bindingRevoked(ctx context.Context) bool {
	if m == nil || m.state != mtproto.AuthStateNormal || m.authKeyId == 0 || m.cb == nil || m.cb.Dao == nil || m.cb.Dao.AuthsessionClient == nil {
		return false
	}
	kData, err := m.cb.Dao.AuthsessionClient.AuthsessionGetAuthStateData(ctx, &authsession.TLAuthsessionGetAuthStateData{
		AuthKeyId: m.authKeyId,
	})
	if err != nil || kData == nil {
		if err != nil {
			logx.WithContext(ctx).Errorf("bindingRevoked(%d) error: %v", m.authKeyId, err)
		}
		return false
	}
	if !authStateRevoked(int(kData.GetKeyState())) {
		return false
	}
	logx.WithContext(ctx).Infof("bindingRevoked(%d) state %d", m.authKeyId, kData.GetKeyState())
	m.changeAuthState(ctx, mtproto.AuthStateDeleted, int64(0))
	return true
}

func (m *MainAuthWrapper) refreshAuthStateIfNeeded(ctx context.Context) {
	if m.authKeyId == 0 || !shouldRefreshAuthState(m.state) {
		return
	}

	kData, err := m.cb.Dao.AuthsessionClient.AuthsessionGetAuthStateData(ctx, &authsession.TLAuthsessionGetAuthStateData{
		AuthKeyId: m.authKeyId,
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("refreshAuthStateIfNeeded(%d) error: %v", m.authKeyId, err)
		return
	}

	m.applyAuthStateData(ctx, kData)
}

func (m *MainAuthWrapper) resetAuth(kType int, authId int64) (lastAuthId int64) {
	var reset *SessionList
	switch kType {
	case mtproto.AuthKeyTypeTemp:
		reset = m.tempAuth
	case mtproto.AuthKeyTypeMediaTemp:
		reset = m.mediaTempAuth
	default:
		reset = m.mainAuth
	}
	if reset == nil {
		return 0
	}
	lastAuthId = reset.Reset(authId)
	// A temp-key or perm-key rebind replaces that key's session map only.
	// The updates session often lives on the other key and has to stay the
	// push target. Clearing it here is what dropped live updates on iOS
	// while the client was still connected.
	m.retargetAfterRebind(reset)
	return
}

// retargetAfterRebind drops a push target only when that target belonged to
// the key that was just replaced and no other generic session is still up.
// A media or upload socket is not promoted into the updates session.
func (m *MainAuthWrapper) retargetAfterRebind(reset *SessionList) {
	if m.mainUpdatesSession != nil && m.mainUpdatesSession.sessList == reset {
		if next := m.pickGenericSession(reset); next != nil {
			m.mainUpdatesSession = next
		} else {
			m.mainUpdatesSession = nil
		}
	}
	if m.androidPushSession != nil && m.androidPushSession.sessList == reset {
		m.androidPushSession = nil
	}
}

func (m *MainAuthWrapper) pickGenericSession(skip *SessionList) *session {
	for _, list := range []*SessionList{m.mainAuth, m.tempAuth, m.mediaTempAuth} {
		if list == nil || list == skip {
			continue
		}
		for _, sess := range list.sessions {
			if sess.holdsUpdates() {
				return sess
			}
		}
	}
	return nil
}

// anyConnectionOnline reports whether a generic session or the Android push
// session is still up. Upload, download, and getFile sockets do not count.
func (m *MainAuthWrapper) anyConnectionOnline() bool {
	if m == nil {
		return false
	}
	if m.mainUpdatesSession.keepsOnline() || m.androidPushSession.keepsOnline() {
		return true
	}
	for _, sess := range m.onlineSessions() {
		if sess.keepsOnline() {
			return true
		}
	}
	return false
}

func (m *MainAuthWrapper) onlineSessions() []*session {
	var out []*session
	for _, list := range []*SessionList{m.mainAuth, m.tempAuth, m.mediaTempAuth} {
		if list == nil {
			continue
		}
		for _, sess := range list.sessions {
			if sess != nil && sess.sessionOnline() {
				out = append(out, sess)
			}
		}
	}
	return out
}

func (m *MainAuthWrapper) setOnline(ctx context.Context) {
	//setOnlineTTL(s.AuthUserId, s.authKeyId, getServerID(), s.Layer, 60)
	date := time.Now().Unix()
	if (m.onlineExpired == 0 || date > m.onlineExpired-kPingAddTimeout) && m.AuthUserId != 0 {
		logx.WithContext(ctx).Infof("DEBUG] setOnline - set online: (date: %d, userId:%d, onlineExpired: %d, authKeyId: %d)",
			date,
			m.AuthUserId,
			m.onlineExpired,
			m.authKeyId)

		if m.cb == nil || m.cb.Dao == nil || m.cb.Dao.StatusClient == nil {
			m.onlineExpired = date + 60
			return
		}
		// s.setOnlineTTL(s.AuthUserId, s.authKeyId, getServerID(), s.Layer, 60)
		_, _ = m.cb.Dao.StatusClient.StatusSetSessionOnline(
			ctx,
			&status.TLStatusSetSessionOnline{
				UserId: m.AuthUserId,
				Session: &status.SessionEntry{
					UserId:        m.AuthUserId,
					AuthKeyId:     m.authKeyId,
					Gateway:       m.cb.Dao.MyServerId,
					Expired:       date + 60,
					Layer:         m.Layer(),
					PermAuthKeyId: m.authKeyId,
					Client:        m.ClientName(),
				},
			})
		m.onlineExpired = date + 60
	} else {
		//logx.WithContext(ctx).Infof("DEBUG] setOnline - not set online: (date: %d, onlineExpired: %d, AuthUserId: %d)",
		//	date,
		//	s.onlineExpired,
		//	s.AuthUserId)
	}
}

func (m *MainAuthWrapper) trySetOffline(ctx context.Context) {
	// One socket closing used to mark the whole auth key offline. Push then
	// found no sessions and the new message was never delivered live.
	if m.anyConnectionOnline() {
		return
	}
	if m.mainUpdatesSession != nil && !m.mainUpdatesSession.sessionOnline() {
		m.mainUpdatesSession = nil
	}
	if m.androidPushSession != nil && !m.androidPushSession.sessionOnline() {
		m.androidPushSession = nil
	}

	if m.AuthUserId > 0 && m.cb != nil && m.cb.Dao != nil && m.cb.Dao.StatusClient != nil {
		logx.WithContext(ctx).Infof("authSessions]]>> offline: %s", m)
		_, _ = m.cb.Dao.StatusClient.StatusSetSessionOffline(ctx, &status.TLStatusSetSessionOffline{
			UserId:    m.AuthUserId,
			AuthKeyId: m.authKeyId,
		})
	}
	m.onlineExpired = 0
}

func (m *MainAuthWrapper) delOnline(ctx context.Context) {
	if m.AuthUserId > 0 && m.cb != nil && m.cb.Dao != nil && m.cb.Dao.StatusClient != nil {
		logx.Infof("authSessions]]>> delOnline: %s", m)

		_, _ = m.cb.Dao.StatusClient.StatusSetSessionOffline(ctx, &status.TLStatusSetSessionOffline{
			UserId:    m.AuthUserId,
			AuthKeyId: m.authKeyId,
		})
	}
	m.onlineExpired = 0
}

func (m *MainAuthWrapper) getNextNotifyId() (id int64) {
	id = m.nextNotifyId
	m.nextNotifyId--
	return
}

func (m *MainAuthWrapper) getNextPushId() (id int64) {
	id = m.nextPushId
	m.nextPushId++
	return
}

func (m *MainAuthWrapper) getClient() *authsession.ClientSession {
	if m.client != nil {
		return m.client
	}

	return nil
}

func (m *MainAuthWrapper) onUpdateLayer(ctx context.Context, layer int32) {
	if layer == 0 || m.Layer() == layer {
		return
	}

	// Keep the IP already stored for this key. SetLayer's upsert writes
	// client_ip, and an empty value here wipes the address that
	// account.getAuthorizations uses for country.
	ip := ""
	if m.client != nil {
		ip = m.client.Ip
	}

	if m.client == nil {
		m.client = authsession.MakeTLClientSession(&authsession.ClientSession{
			AuthKeyId:      m.authKeyId,
			Ip:             ip,
			Layer:          layer,
			ApiId:          0,
			DeviceModel:    "",
			SystemVersion:  "",
			AppVersion:     "",
			SystemLangCode: "",
			LangPack:       "",
			LangCode:       "",
			Proxy:          "",
			Params:         "",
		}).To_ClientSession()
	} else {
		m.client.Layer = layer
	}

	_, _ = m.cb.Dao.AuthsessionClient.AuthsessionSetLayer(ctx, &authsession.TLAuthsessionSetLayer{
		AuthKeyId: m.authKeyId,
		Ip:        ip,
		Layer:     layer,
	})
}

func (m *MainAuthWrapper) onUpdateInitConnection(ctx context.Context, clientIp string, initConnection *mtproto.TLInitConnection) {
	if initConnection == nil {
		return
	}

	if m.state < mtproto.AuthStateUnauthorized {
		m.state = mtproto.AuthStateUnauthorized
	}

	clientNeedUpdate := false

	proxy, _ := jsonx.MarshalToString(initConnection.Proxy)
	params, _ := jsonx.MarshalToString(initConnection.Params)

	if m.client == nil {
		m.client = authsession.MakeTLClientSession(&authsession.ClientSession{
			AuthKeyId:      m.authKeyId,
			Ip:             clientIp,
			Layer:          0,
			ApiId:          initConnection.ApiId,
			DeviceModel:    initConnection.DeviceModel,
			SystemVersion:  initConnection.SystemVersion,
			AppVersion:     initConnection.AppVersion,
			SystemLangCode: initConnection.SystemLangCode,
			LangPack:       initConnection.LangPack,
			LangCode:       initConnection.LangCode,
			Proxy:          proxy,
			Params:         params,
		}).To_ClientSession()

		clientNeedUpdate = true
	} else {
		if m.client.Ip != clientIp {
			m.client.Ip = clientIp
			clientNeedUpdate = true
		}
		if m.client.ApiId != initConnection.ApiId {
			m.client.ApiId = initConnection.ApiId
			clientNeedUpdate = true
		}
		if m.client.DeviceModel != initConnection.DeviceModel {
			m.client.DeviceModel = initConnection.DeviceModel
			clientNeedUpdate = true
		}
		if m.client.SystemVersion != initConnection.SystemVersion {
			m.client.SystemVersion = initConnection.SystemVersion
			clientNeedUpdate = true
		}
		if m.client.AppVersion != initConnection.AppVersion {
			m.client.AppVersion = initConnection.AppVersion
			clientNeedUpdate = true
		}
		if m.client.SystemLangCode != initConnection.SystemLangCode {
			m.client.SystemLangCode = initConnection.SystemLangCode
			clientNeedUpdate = true
		}
		if m.client.LangPack != initConnection.LangPack {
			m.client.LangPack = initConnection.LangPack
			clientNeedUpdate = true
		}
		if m.client.LangCode != initConnection.LangCode {
			m.client.LangCode = initConnection.LangCode
			clientNeedUpdate = true
		}
		if m.client.Proxy != proxy {
			m.client.Proxy = proxy
			// clientNeedUpdate = true
		}
		if m.client.Params != params {
			m.client.Params = params
			// clientNeedUpdate = true
		}
	}

	if clientNeedUpdate {
		m.cb.Dao.AuthsessionClient.AuthsessionSetInitConnection(ctx, &authsession.TLAuthsessionSetInitConnection{
			AuthKeyId:      m.authKeyId,
			Ip:             clientIp,
			ApiId:          m.ApiId(),
			DeviceModel:    m.DeviceModel(),
			SystemVersion:  m.SystemVersion(),
			AppVersion:     m.AppVersion(),
			SystemLangCode: m.SystemLangCode(),
			LangPack:       m.LangPack(),
			LangCode:       m.LangCode(),
			Proxy:          m.Proxy(),
			Params:         m.Params(),
		})
	}

	m.refreshAuthStateIfNeeded(ctx)
}

func (m *MainAuthWrapper) onBindPushSessionId(ctx context.Context, sList *SessionList, sessionId int64) {
	if m.pushSessionId == 0 {
		m.pushSessionId = sessionId

		// setAndroidPushSessionId
		_, _ = m.cb.Dao.AuthsessionClient.AuthsessionSetAndroidPushSessionId(
			ctx,
			&authsession.TLAuthsessionSetAndroidPushSessionId{
				UserId:    m.AuthUserId,
				AuthKeyId: m.authKeyId,
				SessionId: sessionId,
			})
	}

	sess := m.androidPushSession
	if sess == nil {
		sess, _ = sList.sessions[sessionId]
	}
	if sess == nil {
		sess = newSession(sessionId, sList)
	}

	sess.isAndroidPush = true
	sess.canSync = true
	m.androidPushSession = sess

	m.setOnline(ctx)
}

func (m *MainAuthWrapper) onSetMainUpdatesSession(ctx context.Context, sess *session) {
	if !sess.isGeneric {
		sess.isGeneric = true
	}
	if m.mainUpdatesSession == nil || m.mainUpdatesSession.sessionId != sess.sessionId {
		m.mainUpdatesSession = sess
	}
	m.setOnline(ctx)
}

func (m *MainAuthWrapper) Layer() int32 {
	return m.getClient().GetLayer()
}

func (m *MainAuthWrapper) ApiId() int32 {
	return m.getClient().GetApiId()
}

func (m *MainAuthWrapper) DeviceModel() string {
	return m.getClient().GetDeviceModel()
}

func (m *MainAuthWrapper) SystemVersion() string {
	return m.getClient().GetSystemVersion()
}

func (m *MainAuthWrapper) AppVersion() string {
	return m.getClient().GetAppVersion()
}

func (m *MainAuthWrapper) SystemLangCode() string {
	return m.getClient().GetSystemLangCode()
}

func (m *MainAuthWrapper) LangPack() string {
	return m.getClient().GetLangPack()
}

func (m *MainAuthWrapper) LangCode() string {
	return m.getClient().GetLangCode()
}

func (m *MainAuthWrapper) ClientIp() string {
	return m.getClient().GetIp()
}

func (m *MainAuthWrapper) Proxy() string {
	return m.getClient().GetProxy()
}

func (m *MainAuthWrapper) Params() string {
	return m.getClient().GetParams()
}

func (m *MainAuthWrapper) ClientName() string {
	c := m.getClient().GetLangPack()

	if c == "android" {
		if strings.Index(m.getClient().GetAppVersion(), "TDLib") >= 0 {
			c = "react"
		}
	} else if c == "" {
		if strings.HasSuffix(m.getClient().GetAppVersion(), " A") {
			c = "weba"
		} else if strings.HasSuffix(m.getClient().GetAppVersion(), " Z") {
			c = "weba"
		}
	}

	return c
}

func (m *MainAuthWrapper) getSessionList(kType int) (sList *SessionList) {
	switch kType {
	case mtproto.AuthKeyTypeTemp:
		sList = m.tempAuth
	case mtproto.AuthKeyTypeMediaTemp:
		sList = m.mediaTempAuth
	default:
		sList = m.mainAuth
	}

	return
}

func (m *MainAuthWrapper) getSessionListById(authId int64) (sList *SessionList) {
	switch authId {
	case m.tempAuth.authId:
		sList = m.tempAuth
	case m.mediaTempAuth.authId:
		sList = m.mediaTempAuth
	case m.mainAuth.authId:
		sList = m.mainAuth
	}

	return
}

func (m *MainAuthWrapper) String() string {
	return fmt.Sprintf("{auth_key_id: %d, user_id: %d, layer: %d}", m.authKeyId, m.AuthUserId, m.Layer())
}

func (m *MainAuthWrapper) Start() {
	m.running.Set(true)
	m.finish.Add(2)
	go m.rpcRunLoop()
	go m.runLoop()
}

func (m *MainAuthWrapper) Stop() {
	m.running.Set(false)
	// m.rpcQueue.Close()
}

func (m *MainAuthWrapper) runLoop() {
	defer func() {
		if m.anyConnectionOnline() {
			m.delOnline(context.Background())
		}
		m.finish.Done()
		m.rpcQueue.Close()
		close(m.closeChan)
		// Do not close sessionDataChan here to avoid panics from senders writing to a closed channel.
		m.finish.Wait()
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for m.running.True() {
		select {
		case <-m.closeChan:
			// log.Info("runLoop -> To Close ", this.String())
			return

		case sessionMsg, _ := <-m.sessionDataChan:
			switch ctxData := sessionMsg.(type) {
			case *sessionDataCtx:
				threading.RunSafe(func() {
					m.onSessionData(ctxData.ctx, &ctxData.sessionData)
				})
			case *sessionHttpDataCtx:
				threading.RunSafe(func() {
					m.onSessionHttpData(ctxData.ctx, &ctxData.sessionHttpData)
				})
			case *syncRpcResultDataCtx:
				threading.RunSafe(func() {
					m.onSyncRpcResultData(ctxData.ctx, &ctxData.syncRpcResultData)
				})
			case *syncDataCtx:
				threading.RunSafe(func() {
					m.onSyncData(ctxData.ctx, &ctxData.syncData)
				})
			case *syncSessionDataCtx:
				threading.RunSafe(func() {
					m.onSyncSessionData(ctxData.ctx, &ctxData.syncSessionData)
				})
			case *connDataCtx:
				threading.RunSafe(func() {
					if ctxData.isNew {
						m.onSessionNew(ctxData.ctx, &ctxData.connData)
					} else {
						m.onSessionClosed(ctxData.ctx, &ctxData.connData)
					}
				})
			default:
				panic("receive invalid type msg")
			}
		case rpcMessages, _ := <-m.rpcDataChan:
			threading.RunSafe(func() {
				rpcResult, _ := rpcMessages.(*rpcApiMessage)
				_ = rpcResult
				if sess, ok := rpcResult.sessList.sessions[rpcResult.sessionId]; ok {
					// log.Debugf("onRpcResult result: %s", rpcResult)
					sess.onRpcResult(rpcResult.ctx, rpcResult)
				} else {
					logx.WithContext(rpcResult.ctx).Errorf("onRpcResult - not found rpcSession by sessionId: %d", rpcResult.sessionId)
				}
			})
		case <-ticker.C:
			threading.RunSafe(func() {
				m.onTimer(context.Background())
			})
		}
	}

	logx.Infof("%s -> quit runLoop...", m)
}

func (m *MainAuthWrapper) rpcRunLoop() {
	defer func() {
		m.finish.Done()
		close(m.rpcDataChan)
	}()
	for {
		apiRequest := m.rpcQueue.Pop()
		if apiRequest == nil {
			logx.Infof("%s -> quit rpcRunLoop...", m)
			return
		} else {
			batch := apiRequest.([]*rpcApiMessage)
			threading.RunSafe(func() {
				for _, request := range batch {
					request := request
					if isFileRPC(request.reqMsg) {
						// upload.getFile used to run on this loop. A dfs read
						// that waited on R2 blocked getDifference and sends,
						// which is the client stuck on "Updating...".
						threading.GoSafe(func() {
							ctx, cancel := context.WithTimeout(request.ctx, fileRPCTimeout)
							defer cancel()
							if !acquireFileSlot(ctx) {
								request.setResult(internalRPCResult(request.reqMsgId))
								m.rpcDataChan <- request
								return
							}
							defer func() { <-fileRPCSlots }()
							m.finishRPC(ctx, request)
						})
						continue
					}
					m.finishRPC(request.ctx, request)
				}
			})
		}
	}
}

func (m *MainAuthWrapper) finishRPC(ctx context.Context, request *rpcApiMessage) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rpcTimeout(request.reqMsg))
		defer cancel()
	}
	md := &metadata.RpcMetadata{
		ServerId:       m.cb.Dao.MyServerId,
		ClientAddr:     request.clientIp,
		AuthId:         request.sessList.authId,
		SessionId:      request.sessionId,
		ReceiveTime:    time.Now().Unix(),
		UserId:         m.AuthUserId,
		ClientMsgId:    request.reqMsgId,
		Layer:          m.Layer(),
		Client:         m.ClientName(),
		Langpack:       m.LangPack(),
		PermAuthKeyId:  m.authKeyId,
		LangCode:       m.LangCode(),
		SystemLangCode: m.SystemLangCode(),
		Takeout:        request.takeout,
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = recover() }()
		doRpcRequest(ctx, m.cb.Dao, md, request)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		// The handler ignored the deadline. Answer anyway.
		request.setResult(internalRPCResult(request.reqMsgId))
	}
	m.rpcDataChan <- request
}

func rpcTimeout(msg mtproto.TLObject) time.Duration {
	if isFileRPC(msg) {
		return fileRPCTimeout
	}
	switch msg.(type) {
	case *mtproto.TLMessagesSendMedia,
		*mtproto.TLMessagesSendMultiMedia,
		*mtproto.TLMessagesUploadMedia,
		*mtproto.TLMessagesEditMessage,
		*mtproto.TLMessagesTranscribeAudio:
		return 60 * time.Second
	default:
		return defaultRPCTimeout
	}
}

func internalRPCResult(reqMsgId int64) *mtproto.TLRpcResult {
	return &mtproto.TLRpcResult{
		ReqMsgId: reqMsgId,
		Result:   mtproto.NewInternalError(),
	}
}

func (m *MainAuthWrapper) sendToRpcQueue(ctx context.Context, rpcMessage []*rpcApiMessage) {
	m.rpcQueue.Push(rpcMessage)
}

func (m *MainAuthWrapper) onTimer(ctx context.Context) {
	for _, sess := range m.mainAuth.sessions {
		sess.onTimer(ctx)
	}
	for _, sess := range m.tempAuth.sessions {
		sess.onTimer(ctx)
	}
	for _, sess := range m.mediaTempAuth.sessions {
		sess.onTimer(ctx)
	}

	if m.anyConnectionOnline() {
		m.setOnline(ctx)
	}

	for _, sess := range m.mainAuth.sessions {
		if !sess.sessionClosed() {
			return
		}
	}
	for _, sess := range m.tempAuth.sessions {
		if !sess.sessionClosed() {
			return
		}
	}
	for _, sess := range m.mediaTempAuth.sessions {
		if !sess.sessionClosed() {
			return
		}
	}

	m.cb.DeleteByAuthKeyId(m.authKeyId)
	m.Stop()
}

func (m *MainAuthWrapper) SessionClientNew(ctx context.Context, kType int, kId int64, gatewayId string, sessionId int64) error {
	cData := &connDataCtx{
		ctx: contextx.ValueOnlyFrom(ctx),
		connData: connData{
			authType:  kType,
			authId:    kId,
			isNew:     true,
			gatewayId: gatewayId,
			sessionId: sessionId,
		},
	}

	select {
	case m.sessionDataChan <- cData:
		return nil
	default:
		logx.WithContext(ctx).Errorf("sessionDataChan full, dropping SessionClientNew (authKeyId:%d, sessionId:%d)", kId, sessionId)
		return ErrDataChannelFull
	}
}

func (m *MainAuthWrapper) SessionDataArrived(ctx context.Context, kType int, kId int64, gatewayId, clientIp string, sessionId, salt int64, buf []byte) error {
	sData := &sessionDataCtx{
		ctx: contextx.ValueOnlyFrom(ctx),
		sessionData: sessionData{
			authType:  kType,
			authId:    kId,
			gatewayId: gatewayId,
			clientIp:  clientIp,
			sessionId: sessionId,
			salt:      salt,
			buf:       buf,
		},
	}

	select {
	case m.sessionDataChan <- sData:
		return nil
	default:
		logx.WithContext(ctx).Errorf("sessionDataChan full, dropping SessionDataArrived (authKeyId:%d, sessionId:%d)", kId, sessionId)
		return ErrDataChannelFull
	}
}

func (m *MainAuthWrapper) SessionHttpDataArrived(ctx context.Context, kType int, kId int64, gatewayId, clientIp string, sessionId, salt int64, buf []byte, resChan chan interface{}) error {
	sData := &sessionHttpDataCtx{
		ctx: contextx.ValueOnlyFrom(ctx),
		sessionHttpData: sessionHttpData{
			authType:   kType,
			authId:     kId,
			gatewayId:  gatewayId,
			clientIp:   clientIp,
			sessionId:  sessionId,
			salt:       salt,
			buf:        buf,
			resChannel: resChan,
		},
	}

	select {
	case m.sessionDataChan <- sData:
		return nil
	default:
		logx.WithContext(ctx).Errorf("sessionDataChan full, dropping SessionHttpDataArrived (authKeyId:%d, sessionId:%d)", kId, sessionId)
		return ErrDataChannelFull
	}
}

func (m *MainAuthWrapper) SessionClientClosed(ctx context.Context, kType int, kId int64, gatewayId string, sessionId, closedAt int64) error {
	cData := &connDataCtx{
		ctx: contextx.ValueOnlyFrom(ctx),
		connData: connData{
			authType:  kType,
			authId:    kId,
			isNew:     false,
			gatewayId: gatewayId,
			sessionId: sessionId,
			closedAt:  closedAt,
		},
	}

	select {
	case m.sessionDataChan <- cData:
		return nil
	default:
		logx.WithContext(ctx).Errorf("sessionDataChan full, dropping SessionClientClosed (authKeyId:%d, sessionId:%d)", kId, sessionId)
		return ErrDataChannelFull
	}
}

func (m *MainAuthWrapper) SyncRpcResultDataArrived(ctx context.Context, kId int64, sessionId, clientMsgId int64, data []byte) error {
	rData := &syncRpcResultDataCtx{
		ctx: contextx.ValueOnlyFrom(ctx),
		syncRpcResultData: syncRpcResultData{
			authType:    mtproto.AuthKeyTypeUnknown,
			authId:      kId,
			sessionId:   sessionId,
			clientMsgId: clientMsgId,
			data:        data,
		},
	}

	select {
	case m.sessionDataChan <- rData:
		return nil
	default:
		logx.WithContext(ctx).Errorf("sessionDataChan full, dropping SyncRpcResultDataArrived (authKeyId:%d, sessionId:%d)", kId, sessionId)
		return ErrDataChannelFull
	}
}

func (m *MainAuthWrapper) SyncSessionDataArrived(ctx context.Context, kId int64, sessionId int64, updates *mtproto.Updates) error {
	sData := &syncSessionDataCtx{
		ctx: contextx.ValueOnlyFrom(ctx),
		syncSessionData: syncSessionData{
			authType:  mtproto.AuthKeyTypeUnknown,
			authId:    kId,
			sessionId: sessionId,
			data:      &messageData{obj: updates},
		},
	}

	select {
	case m.sessionDataChan <- sData:
		return nil
	default:
		logx.WithContext(ctx).Errorf("sessionDataChan full, dropping SyncSessionDataArrived (authKeyId:%d, sessionId:%d)", kId, sessionId)
		return ErrDataChannelFull
	}
}

func (m *MainAuthWrapper) SyncDataArrived(ctx context.Context, needAndroidPush bool, updates *mtproto.Updates) error {
	sData := &syncDataCtx{
		ctx: contextx.ValueOnlyFrom(ctx),
		syncData: syncData{
			needAndroidPush: needAndroidPush,
			data:            &messageData{obj: updates},
		},
	}

	select {
	case m.sessionDataChan <- sData:
		return nil
	default:
		logx.WithContext(ctx).Errorf("sessionDataChan full, dropping SyncDataArrived (authKeyId:%d)", m.authKeyId)
		return ErrDataChannelFull
	}
}

func (m *MainAuthWrapper) onSessionNew(ctx context.Context, connMsg *connData) {
	sList := m.getSessionList(connMsg.authType)

	if sList.authId == 0 {
		m.resetAuth(connMsg.authType, connMsg.authId)
	} else if sList.authId != connMsg.authId {
		m.resetAuth(connMsg.authType, connMsg.authId)
	}

	sess, ok := sList.sessions[connMsg.sessionId]
	if !ok {
		logx.WithContext(ctx).Infof("onSessionNew - newSession(%d), conn: %s", m.authKeyId, connMsg)
		sess = newSession(connMsg.sessionId, sList)
		sList.sessions[connMsg.sessionId] = sess
	} else {
		sess.sessionState = kSessionStateNew
		logx.WithContext(ctx).Infof("onSessionNew - session(%d) found, conn: %s", m.authKeyId, connMsg)
	}

	sess.onSessionConnNew(ctx, connMsg.gatewayId)
}

func (m *MainAuthWrapper) onSessionData(ctx context.Context, sessionMsg *sessionData) {
	sList := m.getSessionList(sessionMsg.authType)

	if sList.authId == 0 {
		m.resetAuth(sessionMsg.authType, sessionMsg.authId)
	} else if sList.authId != sessionMsg.authId {
		m.resetAuth(sessionMsg.authType, sessionMsg.authId)
	}

	message2 := new(mtproto.TLMessage2)
	dBuf := mtproto.GetDecodeBuf(sessionMsg.buf)
	err := message2.Decode(dBuf)
	mtproto.PutDecodeBuf(dBuf)
	if err != nil {
		m.answerDecodeFailure(ctx, "onSessionData", sessionMsg.authType, sessionMsg.gatewayId, sessionMsg.sessionId, sessionMsg.salt, nil, sessionMsg.buf, message2, err)
		return
	}

	// TODO(@benqi): load onNew
	if sList.cacheSalt == nil {
		sList.cacheSalt, sList.cacheLastSalt, _ = m.cb.Dao.GetOrFetchNewSalt(ctx, sList.authId)
	} else {
		if int32(time.Now().Unix()) >= sList.cacheSalt.GetValidUntil() {
			sList.cacheSalt, sList.cacheLastSalt, _ = m.cb.Dao.GetOrFetchNewSalt(ctx, sList.authId)
		}
	}

	if sList.cacheSalt == nil {
		// Leave it empty. checkBadServerSalt adopts the client's salt
		// instead of ignoring the message, which made iOS handshake again.
		logx.WithContext(ctx).Infof("onSessionData - no stored salt yet, data: {sessions: %s, conn_id: %s}", m, sessionMsg.gatewayId)
	}

	sess, ok := sList.sessions[sessionMsg.sessionId]
	if !ok {
		sess = newSession(sessionMsg.sessionId, sList)
		sList.sessions[sessionMsg.sessionId] = sess
	}

	sess.onSessionConnNew(ctx, sessionMsg.gatewayId)
	sess.onSessionMessageData(ctx, sessionMsg.gatewayId, sessionMsg.clientIp, sessionMsg.salt, message2)
}

func (m *MainAuthWrapper) onSessionHttpData(ctx context.Context, sessionMsg *sessionHttpData) {
	sList := m.getSessionList(sessionMsg.authType)

	if sList.authId == 0 {
		m.resetAuth(sessionMsg.authType, sessionMsg.authId)
	} else if sList.authId != sessionMsg.authId {
		m.resetAuth(sessionMsg.authType, sessionMsg.authId)
	}

	message2 := new(mtproto.TLMessage2)
	dBuf := mtproto.GetDecodeBuf(sessionMsg.buf)
	err := message2.Decode(dBuf)
	mtproto.PutDecodeBuf(dBuf)
	if err != nil {
		m.answerDecodeFailure(ctx, "onSessionHttpData", sessionMsg.authType, sessionMsg.gatewayId, sessionMsg.sessionId, sessionMsg.salt, sessionMsg.resChannel, sessionMsg.buf, message2, err)
		return
	}

	// Load or refresh salt.
	if sList.cacheSalt == nil {
		sList.cacheSalt, sList.cacheLastSalt, _ = m.cb.Dao.GetOrFetchNewSalt(ctx, sList.authId)
	} else if int32(time.Now().Unix()) >= sList.cacheSalt.GetValidUntil() {
		sList.cacheSalt, sList.cacheLastSalt, _ = m.cb.Dao.GetOrFetchNewSalt(ctx, sList.authId)
	}

	if sList.cacheSalt == nil {
		logx.WithContext(ctx).Infof("onSessionHttpData - no stored salt yet, data: {sessions: %s, conn_id: %s}", m, sessionMsg.gatewayId)
	}

	sess, ok := sList.sessions[sessionMsg.sessionId]
	if !ok {
		sess = newSession(sessionMsg.sessionId, sList)
		sList.sessions[sessionMsg.sessionId] = sess
	}

	sess.isHttp = true
	sess.httpQueue.Push(sessionMsg.resChannel)
	sess.onSessionConnNew(ctx, sessionMsg.gatewayId)
	sess.onSessionMessageData(ctx, sessionMsg.gatewayId, sessionMsg.clientIp, sessionMsg.salt, message2)
}

// answerDecodeFailure logs the constructor the codec rejected and, when the
// message is an RPC the client is waiting on, returns rpc_error. A content
// related message has an odd seqno. Service messages stay unanswered.
func (m *MainAuthWrapper) answerDecodeFailure(ctx context.Context, where string, authType int, gatewayID string, sessionID, clientSalt int64, httpCh chan interface{}, buf []byte, message *mtproto.TLMessage2, err error) {
	ctor := messageConstructor(buf)
	name := ""
	if ctor != 0 || len(buf) >= 20 {
		name = mtproto.ConstructorName(ctor)
	}
	label := "unknown"
	if len(buf) >= 20 {
		label = fmt.Sprintf("0x%08x", uint32(ctor))
		if name != "" {
			label += " " + name
		}
	}
	msgID := int64(0)
	seqno := int32(0)
	if message != nil {
		msgID = message.MsgId
		seqno = message.Seqno
	}
	logx.WithContext(ctx).Errorf("%s - error: {%s}, msg_id: %d, seqno: %d, constructor: %s, sessions: %s, gate_id: %s",
		where, err, msgID, seqno, label, m, gatewayID)
	if message == nil || msgID == 0 || seqno&1 == 0 {
		return
	}

	sList := m.getSessionList(authType)
	// A corrupt packet must not become the stored salt. Borrow the client's
	// salt only for this reply, then drop it. Answering with salt 0 makes
	// iOS drop the auth key.
	if sList.cacheSalt == nil {
		now := int32(time.Now().Unix())
		sList.cacheSalt = mtproto.MakeTLFutureSalt(&mtproto.FutureSalt{
			ValidSince: now,
			ValidUntil: now + 30*60,
			Salt:       clientSalt,
		})
		defer func() { sList.cacheSalt = nil }()
	}
	sess, ok := sList.sessions[sessionID]
	if !ok {
		sess = newSession(sessionID, sList)
		sList.sessions[sessionID] = sess
	}
	if httpCh != nil {
		sess.isHttp = true
		sess.httpQueue.Push(httpCh)
	}
	sess.onSessionConnNew(ctx, gatewayID)
	sess.sendRpcResultToQueue(ctx, gatewayID, msgID, mtproto.NewRpcError(decodeFailureStatus(err)))
	sess.sendQueueToGateway(ctx, gatewayID)
}

func messageConstructor(buf []byte) int32 {
	if len(buf) < 20 {
		return 0
	}
	return int32(binary.LittleEndian.Uint32(buf[16:20]))
}

func decodeFailureStatus(err error) error {
	if err != nil && strings.Contains(err.Error(), "can't find registered classId") {
		return mtproto.ErrInputConstructorInvalid
	}
	return mtproto.ErrInputRequestInvalid
}

func (m *MainAuthWrapper) onSessionClosed(ctx context.Context, connMsg *connData) {
	sList := m.getSessionList(connMsg.authType)

	if sess, ok := sList.sessions[connMsg.sessionId]; !ok {
		logx.WithContext(ctx).Errorf("onSessionClosed - session conn closed -  conn: %s", connMsg)
	} else {
		logx.WithContext(ctx).Infof("onSessionClosed - conn: %s, sess: %s", connMsg, sess)
		sess.onSessionConnClose(ctx, connMsg.gatewayId, connMsg.closedAt)
	}
}

func (m *MainAuthWrapper) onSyncRpcResultData(ctx context.Context, syncMsg *syncRpcResultData) {
	logx.WithContext(ctx).Infof("onSyncRpcResultData - receive data: {sess: %s}, data: {auth_id: %d, session_id: %d, client_msg_id: %d}",
		m,
		syncMsg.authId,
		syncMsg.sessionId,
		syncMsg.clientMsgId)

	var (
		sess *session
	)
	sess, _ = m.mainAuth.sessions[syncMsg.sessionId]
	if sess == nil {
		sess, _ = m.tempAuth.sessions[syncMsg.sessionId]
	}
	if sess == nil {
		sess, _ = m.mediaTempAuth.sessions[syncMsg.sessionId]
	}

	if sess != nil {
		sess.onSyncRpcResultData(ctx, syncMsg.clientMsgId, syncMsg.data)
	} else {
		logx.WithContext(ctx).Errorf("onSyncRpcResultData - not found session by sessionId: %d", syncMsg.sessionId)
	}
}

func (m *MainAuthWrapper) onSyncSessionData(ctx context.Context, syncMsg *syncSessionData) {
	logx.WithContext(ctx).Infof("onSyncSessionData - receive data: {sess: %s}",
		m)

	sList := m.getSessionListById(syncMsg.authId)
	if sList == nil {
		logx.WithContext(ctx).Errorf("onSyncRpcResultData - not found sessionList by authId: %d", syncMsg.authId)
		return
	}

	sess, _ := sList.sessions[syncMsg.sessionId]
	if sess != nil {
		sess.onSyncSessionData(ctx, syncMsg.data.obj)
	}
}

func (m *MainAuthWrapper) onSyncData(ctx context.Context, syncMsg *syncData) {
	logx.WithContext(ctx).Info("authSessions - ", reflect.TypeOf(syncMsg.data.obj))

	if upds, ok := syncMsg.data.obj.(*mtproto.Updates); ok {
		if upds.PredicateName == mtproto.Predicate_updateAccountResetAuthorization {
			if !authorizationResetTargets(m.authKeyId, upds.GetAuthKeyId()) {
				// Another device was reset. This connection stays signed in.
				return
			}
			logx.WithContext(ctx).Infof("recv updateAccountResetAuthorization - authKeyId: %d", m.authKeyId)
			m.changeAuthState(ctx, mtproto.AuthStateDeleted, 0)
			return
		}
	}

	// Updates go to every generic session that is still connected. A file
	// socket (upload, download, getFile) is not generic and must not see them.
	seen := make(map[*session]struct{})
	deliver := func(sess *session, obj mtproto.TLObject) {
		if sess == nil || !sess.sessionOnline() {
			return
		}
		if _, ok := seen[sess]; ok {
			return
		}
		seen[sess] = struct{}{}
		sess.onSyncData(ctx, obj)
	}
	for _, sess := range m.onlineSessions() {
		if !sess.holdsUpdates() {
			continue
		}
		deliver(sess, syncMsg.data.obj)
	}
	if m.mainUpdatesSession.holdsUpdates() {
		deliver(m.mainUpdatesSession, syncMsg.data.obj)
	}
	if syncMsg.needAndroidPush {
		deliver(m.androidPushSession, nil)
	}
}

func doRpcRequest(ctx context.Context, dao *dao.Dao, md *metadata.RpcMetadata, request *rpcApiMessage) {
	var (
		err       error
		rpcResult mtproto.TLObject
	)

	// TODO(@benqi): change state.
	switch request.reqMsg.(type) {
	case *mtproto.TLAuthBindTempAuthKey:
		r := request.reqMsg.(*mtproto.TLAuthBindTempAuthKey)
		rpcResult, err = dao.AuthsessionClient.AuthsessionBindTempAuthKey(
			ctx,
			&authsession.TLAuthsessionBindTempAuthKey{
				PermAuthKeyId:    r.PermAuthKeyId,
				Nonce:            r.Nonce,
				ExpiresAt:        r.ExpiresAt,
				EncryptedMessage: r.EncryptedMessage,
			})
	default:
		rpcResult, err = dao.InvokeContext(ctx, md, request.reqMsg)
	}

	reply := &mtproto.TLRpcResult{
		ReqMsgId: request.reqMsgId,
		Result:   nil,
	}

	if err != nil {
		logx.WithContext(ctx).Error(err.Error())
		if mtproto.IsUnregisteredServiceError(err) {
			// Typed empty / NotModified for startup RPCs, otherwise 400
			// METHOD_NOT_IMPL. Never the gRPC "unknown service" 500.
			reply.Result = mtproto.FallbackForUnregisteredService(request.reqMsg)
		} else if mtproto.IsDeadlineExceeded(err) {
			reply.Result = mtproto.NewInternalError()
		} else {
			reply.Result = mtproto.NewRpcError(err)
		}
	} else {
		logx.WithContext(ctx).Infof("invokeRpcRequest - rpc_result: {%s}", reflect.TypeOf(rpcResult))
		mtproto.SanitizeOutgoingObject(rpcResult)
		reply.Result = rpcResult
	}

	request.setResult(reply)
}
