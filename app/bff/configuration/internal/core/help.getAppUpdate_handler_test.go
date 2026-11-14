package core

import (
	"context"
	"os"
	"path/filepath"
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

func TestHelpGetAppUpdateReadsManifest(t *testing.T) {
	body := []byte(`{"versionName":"14.0.6","versionCode":1802599,"url":"https://www.tellomi.cn/dl/android/Tellomi.apk","fallbackUrl":"https://dl.tellomi.cn/android/Tellomi.apk","sha256sum":"abc","size":3,"changelog":"Bug fixes"}`)
	path := filepath.Join(t.TempDir(), "latest.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	clearAppUpdateEnv(t)
	t.Setenv("TELLOMI_APP_UPDATE_MANIFEST", path)
	android := &metadata.RpcMetadata{Langpack: "android", PermAuthKeyId: 9}
	auth := authUpdateStub{auth: authorization("14.0.4", "Pixel 8", "14")}

	got := callAppUpdate(t, android, auth, "")
	if got.GetPredicateName() != mtproto.Predicate_help_appUpdate || got.GetVersion() != "14.0.6" || got.GetCanNotSkip() || got.GetText() != "Bug fixes" {
		t.Fatalf("predicate %s version %q skip %v text %q", got.GetPredicateName(), got.GetVersion(), got.GetCanNotSkip(), got.GetText())
	}
	if got.GetUrl().GetValue() != "https://www.tellomi.cn/dl/android/Tellomi.apk" {
		t.Fatal(got.GetUrl())
	}

	current := callAppUpdate(t, android, authUpdateStub{auth: authorization("14.0.6", "Pixel 8", "14")}, "")
	if current.GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
		t.Fatal(current.GetPredicateName())
	}
	ios := callAppUpdate(t, &metadata.RpcMetadata{Langpack: "ios", PermAuthKeyId: 9}, auth, "")
	if ios.GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
		t.Fatal(ios.GetPredicateName())
	}

	t.Setenv("TELLOMI_APP_UPDATE_TEXT", "Operator note")
	t.Setenv("TELLOMI_APP_UPDATE_URL", "https://tellomi.cn/get")
	overFields := callAppUpdate(t, android, auth, "")
	if overFields.GetVersion() != "14.0.6" || overFields.GetText() != "Operator note" || overFields.GetUrl().GetValue() != "https://tellomi.cn/get" || overFields.GetCanNotSkip() {
		t.Fatalf("version %q text %q url %v skip %v", overFields.GetVersion(), overFields.GetText(), overFields.GetUrl(), overFields.GetCanNotSkip())
	}

	t.Setenv("TELLOMI_APP_UPDATE_CANNOT_SKIP", "1")
	forcedEnv := callAppUpdate(t, android, auth, "")
	if !forcedEnv.GetCanNotSkip() || forcedEnv.GetVersion() != "14.0.6" {
		t.Fatalf("skip %v version %q", forcedEnv.GetCanNotSkip(), forcedEnv.GetVersion())
	}

	mandatoryPath := filepath.Join(t.TempDir(), "latest.json")
	if err := os.WriteFile(mandatoryPath, []byte(`{"versionName":"14.0.6","url":"https://www.tellomi.cn/dl/android/Tellomi.apk","mandatory":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TELLOMI_APP_UPDATE_CANNOT_SKIP", "0")
	t.Setenv("TELLOMI_APP_UPDATE_MANIFEST", mandatoryPath)
	fromManifest := callAppUpdate(t, android, auth, "")
	if !fromManifest.GetCanNotSkip() || fromManifest.GetVersion() != "14.0.6" {
		t.Fatalf("skip %v version %q", fromManifest.GetCanNotSkip(), fromManifest.GetVersion())
	}

	t.Setenv("TELLOMI_APP_UPDATE_VERSION", "14.0.5")
	t.Setenv("TELLOMI_APP_UPDATE_TEXT", "From env")
	t.Setenv("TELLOMI_APP_UPDATE_URL", "https://tellomi.cn/env")
	t.Setenv("TELLOMI_APP_UPDATE_CANNOT_SKIP", "")
	t.Setenv("TELLOMI_APP_UPDATE_MANIFEST", filepath.Join(t.TempDir(), "missing.json"))
	fromEnv := callAppUpdate(t, android, auth, "")
	if fromEnv.GetVersion() != "14.0.5" || fromEnv.GetText() != "From env" || fromEnv.GetCanNotSkip() || fromEnv.GetUrl().GetValue() != "https://tellomi.cn/env" {
		t.Fatalf("version %q text %q skip %v url %v", fromEnv.GetVersion(), fromEnv.GetText(), fromEnv.GetCanNotSkip(), fromEnv.GetUrl())
	}

	clearAppUpdateEnv(t)
	t.Setenv("TELLOMI_APP_UPDATE_MANIFEST", filepath.Join(t.TempDir(), "missing.json"))
	if callAppUpdate(t, android, auth, "").GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
		t.Fatal("missing manifest")
	}
}

func clearAppUpdateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TELLOMI_APP_UPDATE_VERSION", "")
	t.Setenv("TELLOMI_APP_UPDATE_TEXT", "")
	t.Setenv("TELLOMI_APP_UPDATE_URL", "")
	t.Setenv("TELLOMI_APP_UPDATE_SOURCE", "")
	t.Setenv("TELLOMI_APP_UPDATE_CANNOT_SKIP", "")
}

func TestHelpGetAppUpdateLocalizesText(t *testing.T) {
	android := &metadata.RpcMetadata{Langpack: "android", PermAuthKeyId: 9, LangCode: "zh-hans"}
	auth := authUpdateStub{auth: authorization("14.0.4", "Pixel 8", "14")}

	clearAppUpdateEnv(t)
	path := filepath.Join(t.TempDir(), "latest.json")
	if err := os.WriteFile(path, []byte(`{"versionName":"14.0.6","url":"https://www.tellomi.cn/dl/android/Tellomi.apk"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TELLOMI_APP_UPDATE_MANIFEST", path)
	got := callAppUpdate(t, android, auth, "")
	if got.GetText() != "发现新版本 Tellomi，建议更新。" {
		t.Fatalf("text %q", got.GetText())
	}

	zhCN := callAppUpdate(t, &metadata.RpcMetadata{Langpack: "android", PermAuthKeyId: 9, LangCode: "zh-CN"}, auth, "")
	if zhCN.GetText() != "发现新版本 Tellomi，建议更新。" {
		t.Fatalf("zh-CN %q", zhCN.GetText())
	}
	system := callAppUpdate(t, &metadata.RpcMetadata{Langpack: "android", PermAuthKeyId: 9, SystemLangCode: "zh-Hant-HK"}, auth, "")
	if system.GetText() != "發現新版本 Tellomi，建議更新。" {
		t.Fatalf("system %q", system.GetText())
	}
	chosenEN := callAppUpdate(t, &metadata.RpcMetadata{Langpack: "android", PermAuthKeyId: 9, LangCode: "en", SystemLangCode: "zh-Hans"}, auth, "")
	if chosenEN.GetText() != "A new version of Tellomi is available." {
		t.Fatalf("app language wins %q", chosenEN.GetText())
	}
	fromSession := callAppUpdate(t, &metadata.RpcMetadata{Langpack: "android", PermAuthKeyId: 9}, authUpdateStub{
		auth:     authorization("14.0.4", "Pixel 8", "14"),
		langCode: "ja",
	}, "")
	if fromSession.GetText() != "Tellomi の新しいバージョンがあります。" {
		t.Fatalf("session lang %q", fromSession.GetText())
	}

	notes := filepath.Join(t.TempDir(), "notes.json")
	body := []byte(`{"versionName":"14.0.6","url":"https://www.tellomi.cn/dl/android/Tellomi.apk","changelog":"Shared","notes":{"zh-Hans":"简体说明","en":"English note","zz":1}}`)
	if err := os.WriteFile(notes, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TELLOMI_APP_UPDATE_MANIFEST", notes)
	if got := callAppUpdate(t, android, auth, ""); got.GetText() != "简体说明" {
		t.Fatalf("note %q", got.GetText())
	}
	ja := callAppUpdate(t, &metadata.RpcMetadata{Langpack: "android", PermAuthKeyId: 9, LangCode: "ja"}, auth, "")
	if ja.GetText() != "English note" {
		t.Fatalf("notes.en %q", ja.GetText())
	}

	t.Setenv("TELLOMI_APP_UPDATE_TEXT", "Operator note")
	if got := callAppUpdate(t, android, auth, ""); got.GetText() != "Operator note" {
		t.Fatalf("env text %q", got.GetText())
	}
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
	auth     *mtproto.Authorization
	lang     string
	langCode string
}

func (s authUpdateStub) AuthsessionGetAuthorization(context.Context, *authsession.TLAuthsessionGetAuthorization) (*mtproto.Authorization, error) {
	return s.auth, nil
}

func (s authUpdateStub) AuthsessionGetLangPack(context.Context, *authsession.TLAuthsessionGetLangPack) (*mtproto.String, error) {
	return mtproto.MakeTLString(&mtproto.String{V: s.lang}).To_String(), nil
}

func (s authUpdateStub) AuthsessionGetLangCode(context.Context, *authsession.TLAuthsessionGetLangCode) (*mtproto.String, error) {
	return mtproto.MakeTLString(&mtproto.String{V: s.langCode}).To_String(), nil
}
