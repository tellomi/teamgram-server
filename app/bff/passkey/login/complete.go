// Package login finishes auth.finishPasskeyLogin the same way auth.signIn
// finishes a password-free sign-in: bind the auth key, send the sign-in
// service notification, cancel a pending account deletion, and push the
// 777000 login notice. A cloud password stops at SESSION_PASSWORD_NEEDED
// after the bind, and auth.checkPassword sends the notice.
package login

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/account/deletion"
	"github.com/teamgram/teamgram-server/app/bff/authorization/loginnotice"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	authsession "github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/enforcement"
	"github.com/teamgram/teamgram-server/pkg/geoip"
	"github.com/teamgram/teamgram-server/pkg/langpack"
	"github.com/teamgram/teamgram-server/pkg/verify"
)

// Users loads the account that owns the passkey.
type Users interface {
	UserGetImmutableUser(ctx context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error)
}

// Sessions binds the permanent auth key and reads the device model.
type Sessions interface {
	AuthsessionBindAuthKeyUser(ctx context.Context, in *authsession.TLAuthsessionBindAuthKeyUser) (*mtproto.Int64, error)
	AuthsessionGetAuthorization(ctx context.Context, in *authsession.TLAuthsessionGetAuthorization) (*mtproto.Authorization, error)
}

// Updates sends the sign-in service notification to the account's other sessions.
type Updates interface {
	SyncUpdatesNotMe(ctx context.Context, in *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error)
}

// Deps are the clients Authorize uses. Tests replace them.
type Deps struct {
	Users    Users
	Sessions Sessions
	Updates  Updates
	Msg      loginnotice.Pusher
	Geo      *geoip.Resolver
}

var (
	depMu sync.RWMutex
	deps  Deps
)

// SetDeps installs the clients. Configure and tests call it.
func SetDeps(d Deps) {
	depMu.Lock()
	deps = d
	depMu.Unlock()
}

func current() Deps {
	depMu.RLock()
	defer depMu.RUnlock()
	return deps
}

// Authorize binds md's permanent auth key to userID and returns auth.authorization.
// A cloud password returns SESSION_PASSWORD_NEEDED after the bind.
func Authorize(ctx context.Context, md *metadata.RpcMetadata, userID int64) (*mtproto.Auth_Authorization, error) {
	if md == nil || md.GetPermAuthKeyId() == 0 || userID == 0 {
		return nil, mtproto.ErrAuthKeyPermEmpty
	}
	if ctx == nil {
		ctx = context.Background()
	}
	d := current()
	if d.Users == nil || d.Sessions == nil {
		return nil, mtproto.ErrInternalServerError
	}
	user, err := d.Users.UserGetImmutableUser(ctx, &userpb.TLUserGetImmutableUser{Id: userID})
	if err != nil {
		return nil, err
	}
	if user == nil || user.Id() == 0 {
		return nil, mtproto.ErrInternalServerError
	}
	has, err := password.HasPassword(ctx, user.Id())
	if err != nil {
		return nil, err
	}
	if has {
		if err = password.NoteLogin(ctx, md.GetPermAuthKeyId(), user.Id()); err != nil {
			return nil, err
		}
		if _, err = d.Sessions.AuthsessionBindAuthKeyUser(ctx, &authsession.TLAuthsessionBindAuthKeyUser{
			AuthKeyId: md.GetPermAuthKeyId(),
			UserId:    user.Id(),
		}); err != nil {
			return nil, err
		}
		return nil, mtproto.ErrSessionPasswordNeeded
	}
	identifier, kind := loginIdentifier(user)
	stage := enforcement.LoginStage(user.Id(), identifier, kind, true, time.Now())
	if !stage.BindOrdinary {
		if err = enforcement.RPC(stage.Code); err != nil {
			return nil, err
		}
	}
	if _, err = d.Sessions.AuthsessionBindAuthKeyUser(ctx, &authsession.TLAuthsessionBindAuthKeyUser{
		AuthKeyId: md.GetPermAuthKeyId(),
		UserId:    user.Id(),
	}); err != nil {
		return nil, err
	}
	device := loginDevice(ctx, d, md)
	region := locationLine(d.Geo, md.GetClientAddr())
	if d.Updates != nil {
		_, _ = d.Updates.SyncUpdatesNotMe(ctx, &syncpb.TLSyncUpdatesNotMe{
			UserId:        user.Id(),
			PermAuthKeyId: md.GetPermAuthKeyId(),
			Updates: mtproto.MakeUpdatesByUpdates(
				mtproto.MakeSignInServiceNotification(user.ToSelfUser(), md.GetPermAuthKeyId(), device, region, md.GetClientAddr()),
			),
		})
	}
	_ = deletion.Cancel(ctx, user.Id())
	lang := ""
	if md != nil {
		lang = langpack.ClientLang(md.GetLangCode(), md.GetSystemLangCode(), md.GetLangpack())
	}
	loginnotice.Push(d.Msg, user.Id(), md.GetPermAuthKeyId(), lang, device, time.Now())
	return mtproto.MakeTLAuthAuthorization(&mtproto.Auth_Authorization{
		User: user.ToSelfUser(),
	}).To_Auth_Authorization(), nil
}

// loginIdentifier matches auth.signIn: an email stored in the phone field
// is kind "email", otherwise kind "phone". IDENTIFIER_RESTRICTED is keyed
// that way.
func loginIdentifier(user *mtproto.ImmutableUser) (identifier, kind string) {
	if user == nil {
		return "", "phone"
	}
	identifier = strings.TrimSpace(user.Phone())
	kind = "phone"
	if verify.IsEmail(identifier) {
		kind = "email"
	}
	return identifier, kind
}

func loginDevice(ctx context.Context, d Deps, md *metadata.RpcMetadata) string {
	model := ""
	if d.Sessions != nil && md.GetPermAuthKeyId() != 0 {
		lookup, cancel := context.WithTimeout(ctx, 3*time.Second)
		auth, err := d.Sessions.AuthsessionGetAuthorization(lookup, &authsession.TLAuthsessionGetAuthorization{
			AuthKeyId: md.GetPermAuthKeyId(),
		})
		cancel()
		if err == nil && auth != nil {
			model = auth.GetDeviceModel()
		}
	}
	return loginnotice.Device(model, md.GetClient(), md.GetLangpack())
}

func locationLine(geo *geoip.Resolver, ip string) string {
	if geo != nil {
		country, region := geo.Lookup(ip)
		if line := geoip.LocationLine(country, region); line != "" {
			return line
		}
	}
	if parsed := geoip.ParseClientIP(ip); parsed != nil {
		return parsed.String()
	}
	return ""
}
