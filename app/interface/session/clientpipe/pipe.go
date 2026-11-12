// Package clientpipe runs the real session handler in-process.
// gnetway tests use it so a TCP handshake reaches initConnection and the
// same client_ip is written to auths and charged to send_code_ip.
package clientpipe

import (
	"context"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/core"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/dao"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/sess"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/svc"
	"github.com/teamgram/teamgram-server/app/interface/session/session"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsessionclient "github.com/teamgram/teamgram-server/app/service/authsession/client"
	"github.com/teamgram/teamgram-server/pkg/flood"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

// Pipe is the session service a test gateway dials without etcd.
type Pipe struct {
	svc  *svc.ServiceContext
	auth *authTable
}

// Open builds a session process with a real flood limiter on rds.
// auths.client_ip is the value session passes to authsession.setInitConnection,
// which is the column AuthsDAO inserts.
func Open(rds *redis.Redis) *Pipe {
	table := &authTable{rows: map[int64]string{}}
	lim := flood.New(flood.NewRedisStore(rds), flood.DefaultConfig())
	d := dao.NewDirect(table, lim)
	mgr := sess.NewMainAuthWrapperManager(d)
	return &Pipe{
		svc: &svc.ServiceContext{
			MainAuthMgr: mgr,
			Dao:         d,
		},
		auth: table,
	}
}

// ClientIP is auths.client_ip for authKeyID. The second result is false
// until initConnection has written the row.
func (p *Pipe) ClientIP(authKeyID int64) (string, bool) {
	return p.auth.get(authKeyID)
}

// ClientIPs returns every auths.client_ip written so far.
func (p *Pipe) ClientIPs() []string {
	return p.auth.list()
}

func (p *Pipe) SessionQueryAuthKey(context.Context, *session.TLSessionQueryAuthKey) (*mtproto.AuthKeyInfo, error) {
	return nil, mtproto.ErrAuthKeyUnregistered
}

func (p *Pipe) SessionSetAuthKey(ctx context.Context, in *session.TLSessionSetAuthKey) (*mtproto.Bool, error) {
	return core.New(ctx, p.svc).SessionSetAuthKey(in)
}

func (p *Pipe) SessionCreateSession(context.Context, *session.TLSessionCreateSession) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func (p *Pipe) SessionSendDataToSession(ctx context.Context, in *session.TLSessionSendDataToSession) (*mtproto.Bool, error) {
	return core.New(ctx, p.svc).SessionSendDataToSession(in)
}

func (p *Pipe) SessionSendHttpDataToSession(context.Context, *session.TLSessionSendHttpDataToSession) (*session.HttpSessionData, error) {
	return nil, mtproto.ErrInputRequestInvalid
}

func (p *Pipe) SessionCloseSession(context.Context, *session.TLSessionCloseSession) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func (p *Pipe) SessionPushUpdatesData(context.Context, *session.TLSessionPushUpdatesData) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func (p *Pipe) SessionPushSessionUpdatesData(context.Context, *session.TLSessionPushSessionUpdatesData) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func (p *Pipe) SessionPushRpcResultData(context.Context, *session.TLSessionPushRpcResultData) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

// authTable is the auths.client_ip column. setInitConnection is the only
// writer, matching authsession.SetInitConnection.
type authTable struct {
	mu   sync.Mutex
	rows map[int64]string
}

func (t *authTable) set(authKeyID int64, ip string) {
	t.mu.Lock()
	t.rows[authKeyID] = ip
	t.mu.Unlock()
}

func (t *authTable) get(authKeyID int64) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	ip, ok := t.rows[authKeyID]
	return ip, ok
}

func (t *authTable) list() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.rows))
	for _, ip := range t.rows {
		out = append(out, ip)
	}
	return out
}

func (t *authTable) AuthsessionSetInitConnection(_ context.Context, in *authsession.TLAuthsessionSetInitConnection) (*mtproto.Bool, error) {
	if in == nil || in.GetAuthKeyId() == 0 {
		return nil, mtproto.ErrAuthKeyInvalid
	}
	t.set(in.GetAuthKeyId(), in.GetIp())
	return mtproto.BoolTrue, nil
}

func (t *authTable) AuthsessionSetAuthKey(context.Context, *authsession.TLAuthsessionSetAuthKey) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func (t *authTable) AuthsessionGetAuthStateData(_ context.Context, in *authsession.TLAuthsessionGetAuthStateData) (*authsession.AuthKeyStateData, error) {
	id := int64(0)
	if in != nil {
		id = in.GetAuthKeyId()
	}
	return authsession.MakeTLAuthKeyStateData(&authsession.AuthKeyStateData{
		AuthKeyId: id,
		KeyState:  mtproto.AuthStateNew,
	}).To_AuthKeyStateData(), nil
}

func (t *authTable) AuthsessionGetFutureSalts(context.Context, *authsession.TLAuthsessionGetFutureSalts) (*mtproto.FutureSalts, error) {
	return mtproto.MakeTLFutureSalts(&mtproto.FutureSalts{}).To_FutureSalts(), nil
}

func (t *authTable) AuthsessionSetLayer(context.Context, *authsession.TLAuthsessionSetLayer) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func (t *authTable) AuthsessionGetAuthorizations(context.Context, *authsession.TLAuthsessionGetAuthorizations) (*mtproto.Account_Authorizations, error) {
	return nil, nil
}
func (t *authTable) AuthsessionResetAuthorization(context.Context, *authsession.TLAuthsessionResetAuthorization) (*authsession.Vector_Long, error) {
	return nil, nil
}
func (t *authTable) AuthsessionGetLayer(context.Context, *authsession.TLAuthsessionGetLayer) (*mtproto.Int32, error) {
	return nil, nil
}
func (t *authTable) AuthsessionGetLangPack(context.Context, *authsession.TLAuthsessionGetLangPack) (*mtproto.String, error) {
	return nil, nil
}
func (t *authTable) AuthsessionGetClient(context.Context, *authsession.TLAuthsessionGetClient) (*mtproto.String, error) {
	return nil, nil
}
func (t *authTable) AuthsessionGetLangCode(context.Context, *authsession.TLAuthsessionGetLangCode) (*mtproto.String, error) {
	return nil, nil
}
func (t *authTable) AuthsessionGetUserId(context.Context, *authsession.TLAuthsessionGetUserId) (*mtproto.Int64, error) {
	return nil, nil
}
func (t *authTable) AuthsessionGetPushSessionId(context.Context, *authsession.TLAuthsessionGetPushSessionId) (*mtproto.Int64, error) {
	return nil, nil
}
func (t *authTable) AuthsessionQueryAuthKey(context.Context, *authsession.TLAuthsessionQueryAuthKey) (*mtproto.AuthKeyInfo, error) {
	return nil, mtproto.ErrAuthKeyUnregistered
}
func (t *authTable) AuthsessionBindAuthKeyUser(context.Context, *authsession.TLAuthsessionBindAuthKeyUser) (*mtproto.Int64, error) {
	return nil, nil
}
func (t *authTable) AuthsessionUnbindAuthKeyUser(context.Context, *authsession.TLAuthsessionUnbindAuthKeyUser) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}
func (t *authTable) AuthsessionGetPermAuthKeyId(context.Context, *authsession.TLAuthsessionGetPermAuthKeyId) (*mtproto.Int64, error) {
	return nil, nil
}
func (t *authTable) AuthsessionBindTempAuthKey(context.Context, *authsession.TLAuthsessionBindTempAuthKey) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}
func (t *authTable) AuthsessionSetClientSessionInfo(context.Context, *authsession.TLAuthsessionSetClientSessionInfo) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}
func (t *authTable) AuthsessionGetAuthorization(context.Context, *authsession.TLAuthsessionGetAuthorization) (*mtproto.Authorization, error) {
	return nil, nil
}
func (t *authTable) AuthsessionSetAndroidPushSessionId(context.Context, *authsession.TLAuthsessionSetAndroidPushSessionId) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}
func (t *authTable) AuthsessionChangeAuthorizationSettings(context.Context, *authsession.TLAuthsessionChangeAuthorizationSettings) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

var _ authsessionclient.AuthsessionClient = (*authTable)(nil)
