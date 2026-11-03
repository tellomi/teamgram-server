package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsessionclient "github.com/teamgram/teamgram-server/app/service/authsession/client"
)

func TestHelpGetAppUpdateSessions(t *testing.T) {
	const clientBuild = "14.0.4 (1801699) "

	t.Run("ios lang_pack", func(t *testing.T) {
		setAppUpdateEnv(t, "14.0.5")
		got := callAppUpdate(t, &metadata.RpcMetadata{Langpack: "ios", PermAuthKeyId: 9}, authUpdateStub{
			auth: authorization("14.0.1", "iPhone", "18.0"),
		}, "")
		if got.GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
			t.Fatalf("predicate %s", got.GetPredicateName())
		}
	})

	t.Run("android newer marketing version", func(t *testing.T) {
		setAppUpdateEnv(t, "14.0.5")
		got := callAppUpdate(t, &metadata.RpcMetadata{Langpack: "android", PermAuthKeyId: 9}, authUpdateStub{
			auth: authorization(clientBuild, "Pixel 8", "14"),
		}, "")
		if got.GetPredicateName() != mtproto.Predicate_help_appUpdate || got.GetVersion() != "14.0.5" {
			t.Fatalf("predicate %s version %q", got.GetPredicateName(), got.GetVersion())
		}
	})

	t.Run("bare build number", func(t *testing.T) {
		setAppUpdateEnv(t, "1800979")
		got := callAppUpdate(t, &metadata.RpcMetadata{Langpack: "android", PermAuthKeyId: 9}, authUpdateStub{
			auth: authorization(clientBuild, "Pixel 8", "14"),
		}, "")
		if got.GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
			t.Fatalf("predicate %s", got.GetPredicateName())
		}
	})

	t.Run("ios lang_pack from auths", func(t *testing.T) {
		setAppUpdateEnv(t, "14.0.5")
		got := callAppUpdate(t, &metadata.RpcMetadata{PermAuthKeyId: 9}, authUpdateStub{
			lang: "ios",
			auth: authorization(clientBuild, "Pixel 8", "14"),
		}, "")
		if got.GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
			t.Fatalf("predicate %s", got.GetPredicateName())
		}
	})

	t.Run("iphone when lang_pack empty", func(t *testing.T) {
		setAppUpdateEnv(t, "14.0.5")
		got := callAppUpdate(t, &metadata.RpcMetadata{PermAuthKeyId: 9}, authUpdateStub{
			auth: authorization("14.0.1", "iPhone16,2", "18.1"),
		}, "")
		if got.GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
			t.Fatalf("predicate %s", got.GetPredicateName())
		}
	})

	t.Run("ios system version when lang_pack empty", func(t *testing.T) {
		setAppUpdateEnv(t, "14.0.5")
		got := callAppUpdate(t, &metadata.RpcMetadata{PermAuthKeyId: 9}, authUpdateStub{
			auth: authorization("14.0.1", "Phone", "iOS 18.0"),
		}, "")
		if got.GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
			t.Fatalf("predicate %s", got.GetPredicateName())
		}
	})

	t.Run("android lang_pack wins over iphone model", func(t *testing.T) {
		setAppUpdateEnv(t, "14.0.5")
		got := callAppUpdate(t, &metadata.RpcMetadata{Langpack: "android", PermAuthKeyId: 9}, authUpdateStub{
			auth: authorization("14.0.1", "iPhone", "18.0"),
		}, "")
		if got.GetPredicateName() != mtproto.Predicate_help_appUpdate || got.GetVersion() != "14.0.5" {
			t.Fatalf("predicate %s version %q", got.GetPredicateName(), got.GetVersion())
		}
	})

	t.Run("no session keeps the published update", func(t *testing.T) {
		setAppUpdateEnv(t, "14.0.5")
		c := &ConfigurationCore{ctx: context.Background()}
		got, err := c.HelpGetAppUpdate(&mtproto.TLHelpGetAppUpdate{})
		if err != nil {
			t.Fatal(err)
		}
		if got.GetPredicateName() != mtproto.Predicate_help_appUpdate || got.GetVersion() != "14.0.5" {
			t.Fatalf("predicate %s version %q", got.GetPredicateName(), got.GetVersion())
		}
	})
}

func setAppUpdateEnv(t *testing.T, version string) {
	t.Helper()
	t.Setenv("TELLOMI_APP_UPDATE_VERSION", version)
	t.Setenv("TELLOMI_APP_UPDATE_TEXT", "Fixes")
	t.Setenv("TELLOMI_APP_UPDATE_URL", "https://tellomi.cn/get")
	t.Setenv("TELLOMI_APP_UPDATE_SOURCE", "")
	t.Setenv("TELLOMI_APP_UPDATE_CANNOT_SKIP", "")
}

func callAppUpdate(t *testing.T, md *metadata.RpcMetadata, auth authUpdateStub, source string) *mtproto.Help_AppUpdate {
	t.Helper()
	c := &ConfigurationCore{
		ctx: context.Background(),
		MD:  md,
		svcCtx: &svc.ServiceContext{
			AuthSession: auth,
		},
	}
	got, err := c.HelpGetAppUpdate(&mtproto.TLHelpGetAppUpdate{Source: source})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func authorization(appVersion, device, system string) *mtproto.Authorization {
	return mtproto.MakeTLAuthorization(&mtproto.Authorization{
		AppVersion:    appVersion,
		DeviceModel:   device,
		SystemVersion: system,
	}).To_Authorization()
}

type authUpdateStub struct {
	authsessionclient.AuthsessionClient
	auth *mtproto.Authorization
	lang string
}

func (s authUpdateStub) AuthsessionGetAuthorization(context.Context, *authsession.TLAuthsessionGetAuthorization) (*mtproto.Authorization, error) {
	return s.auth, nil
}

func (s authUpdateStub) AuthsessionGetLangPack(context.Context, *authsession.TLAuthsessionGetLangPack) (*mtproto.String, error) {
	return mtproto.MakeTLString(&mtproto.String{V: s.lang}).To_String(), nil
}
