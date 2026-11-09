package core

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dao/mysql_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// A refused database must not look like a saved privacy update or like the
// empty default settings. A missing row is still the empty object; this case
// is a connection error, which Select does not convert into ErrNotFound.
func TestGlobalPrivacyStoreAndReadErrors(t *testing.T) {
	const addr = "127.0.0.1:1"
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatalf("%s accepted a connection; a refused store cannot be simulated", addr)
	}

	db := sqlx.NewMySQL(&sqlx.Config{
		DSN: "privacy:privacy@tcp(" + addr + ")/none?timeout=200ms&readTimeout=200ms&writeTimeout=200ms",
	})
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{
		Mysql: &dao.Mysql{
			UserGlobalPrivacySettingsDAO: mysql_dao.NewUserGlobalPrivacySettingsDAO(db),
		},
	}})

	settings := mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{
		HideReadMarks:                    true,
		NewNoncontactPeersRequirePremium: true,
	}).To_GlobalPrivacySettings()
	stored, err := c.UserSetGlobalPrivacySettings(&user.TLUserSetGlobalPrivacySettings{
		UserId:   42,
		Settings: settings,
	})
	if err == nil || mtproto.FromBool(stored) {
		t.Fatalf("store failure was reported as success: %v %v", stored, err)
	}

	read, err := c.UserGetGlobalPrivacySettings(&user.TLUserGetGlobalPrivacySettings{UserId: 42})
	if err == nil || read != nil {
		t.Fatalf("read failure was reported as default settings: %v %v", read, err)
	}
}
