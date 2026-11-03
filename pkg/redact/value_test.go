package redact

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/zeromicro/go-zero/core/discov"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

func TestDSN(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"0.0.0.0:10443", "0.0.0.0:10443"},
		{"https://tellomi.cn/legal/terms/", "https://tellomi.cn/legal/terms/"},
		{"rtmp://127.0.0.1:1935/live", "rtmp://127.0.0.1:1935/live"},
		{"redis:6379", "redis:6379"},
		{
			"teamgram:s3cret-db@tcp(mysql:3306)/teamgram?charset=utf8mb4&parseTime=true",
			"teamgram:***@tcp(mysql:3306)/teamgram?charset=utf8mb4&parseTime=true",
		},
		{
			"root:@tcp(127.0.0.1:3306)/teamgram?charset=utf8mb4",
			"root:@tcp(127.0.0.1:3306)/teamgram?charset=utf8mb4",
		},
		{
			"root:@tcp(localhost:3306)/gozero",
			"root:@tcp(localhost:3306)/gozero",
		},
		{
			"teamgram:p%40ss@tcp(mysql:3306)/teamgram",
			"teamgram:***@tcp(mysql:3306)/teamgram",
		},
		{
			"teamgram:p@ss:word@tcp(mysql:3306)/teamgram",
			"teamgram:***@tcp(mysql:3306)/teamgram",
		},
		{
			"teamgram:pw@unix(/var/run/mysqld.sock)/teamgram",
			"teamgram:***@unix(/var/run/mysqld.sock)/teamgram",
		},
		{
			"teamgram:pw@/teamgram",
			"teamgram:***@/teamgram",
		},
		{
			"teamgram:***@tcp(mysql:3306)/teamgram",
			"teamgram:***@tcp(mysql:3306)/teamgram",
		},
		{
			"https://ak:sk@account.r2.cloudflarestorage.com",
			"https://ak:***@account.r2.cloudflarestorage.com",
		},
		{
			"postgres://teamgram:pw@postgres:5432/teamgram",
			"postgres://teamgram:***@postgres:5432/teamgram",
		},
		{
			"https://user:@host/db",
			"https://user:@host/db",
		},
		{
			"a:b@tcp(h1:3306)/db a:c@tcp(h2:3306)/db2",
			"a:***@tcp(h1:3306)/db a:***@tcp(h2:3306)/db2",
		},
	}
	for _, tc := range cases {
		if got := DSN(tc.in); got != tc.want {
			t.Errorf("DSN(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValueMasksStartupConfig(t *testing.T) {
	const (
		dbPass    = "s3cret-db"
		readPass  = "read-secret"
		redisPass = "redis-secret"
		etcdPass  = "etcd-secret"
		chanPass  = "chan-secret"
		accessKey = "minio-access-ABCDEF"
		secretKey = "miniostorage-SECRET"
		turn      = "tellomi-turn-secret"
		rtmpKey   = "tellomi"
		pem       = "-----BEGIN RSA PRIVATE KEY-----\nMIIE-secret\n-----END RSA PRIVATE KEY-----"
	)
	type minio struct {
		Endpoint        string
		AccessKeyID     string
		SecretAccessKey string
		UseSSL          bool
	}
	type sample struct {
		Name          string
		Mysql         sqlx.Config
		Cache         []redis.RedisConf
		Etcd          discov.EtcdConf
		ChannelMySQL  string
		DataSource    string
		Replicas      []string
		Minio         minio
		TurnSecret    string
		RTMPKey       string
		RTMPURL       string
		TermsURL      string
		ListenOn      string
		AuthKeyCacheM int
		KeyFile       string
		KeyPEM        []byte
		FixedCode     string
		IDMap         map[string]string
	}
	in := sample{
		Name: "bff.bff",
		Mysql: sqlx.Config{
			DSN:     "teamgram:" + dbPass + "@tcp(mysql:3306)/teamgram?charset=utf8mb4",
			ReadDSN: []string{"teamgram:" + readPass + "@tcp(mysql-ro:3306)/teamgram"},
		},
		Cache: []redis.RedisConf{{
			Host: "redis:6379",
			Pass: redisPass,
			Type: "node",
		}},
		Etcd: discov.EtcdConf{
			Hosts: []string{"etcd:2379"},
			Key:   "service.biz_service",
			Pass:  etcdPass,
		},
		ChannelMySQL:  "teamgram:" + chanPass + "@tcp(mysql:3306)/teamgram",
		DataSource:    "root:@tcp(localhost:3306)/gozero",
		Replicas:      []string{"teamgram:" + readPass + "@tcp(mysql-ro:3306)/teamgram"},
		Minio:         minio{Endpoint: "minio:9000", AccessKeyID: accessKey, SecretAccessKey: secretKey, UseSSL: false},
		TurnSecret:    turn,
		RTMPKey:       rtmpKey,
		RTMPURL:       "rtmp://127.0.0.1:1935/live",
		TermsURL:      "https://tellomi.cn/legal/terms/",
		ListenOn:      "0.0.0.0:20010",
		AuthKeyCacheM: 10,
		KeyFile:       "./server_pkcs1.key",
		KeyPEM:        []byte(pem),
		FixedCode:     "12345",
		IDMap: map[string]string{
			"/mtproto.RPCPasskey":     "bff.bff",
			"/mtproto.RPCSecretChats": "bff.bff",
			"/mtproto.RPCPassport":    "bff.bff",
		},
	}

	got := Value(in).(sample)

	if in.Mysql.DSN != "teamgram:"+dbPass+"@tcp(mysql:3306)/teamgram?charset=utf8mb4" {
		t.Fatal("Value mutated Mysql.DSN")
	}
	if in.Cache[0].Pass != redisPass || in.Etcd.Pass != etcdPass || in.Minio.SecretAccessKey != secretKey {
		t.Fatal("Value mutated a secret field")
	}
	if string(in.KeyPEM) != pem {
		t.Fatal("Value mutated KeyPEM")
	}
	if got.Name != "bff.bff" || got.ListenOn != "0.0.0.0:20010" || got.AuthKeyCacheM != 10 {
		t.Fatalf("non-secret fields changed: %+v", got)
	}
	if got.Mysql.DSN != "teamgram:***@tcp(mysql:3306)/teamgram?charset=utf8mb4" {
		t.Fatalf("Mysql.DSN = %q", got.Mysql.DSN)
	}
	if got.Mysql.ReadDSN[0] != "teamgram:***@tcp(mysql-ro:3306)/teamgram" {
		t.Fatalf("ReadDSN = %q", got.Mysql.ReadDSN[0])
	}
	if got.ChannelMySQL != "teamgram:***@tcp(mysql:3306)/teamgram" {
		t.Fatalf("ChannelMySQL = %q", got.ChannelMySQL)
	}
	if got.DataSource != in.DataSource {
		t.Fatalf("empty-password DataSource changed: %q", got.DataSource)
	}
	if got.Replicas[0] != "teamgram:***@tcp(mysql-ro:3306)/teamgram" {
		t.Fatalf("Replicas = %q", got.Replicas[0])
	}
	if got.Cache[0].Host != "redis:6379" || got.Cache[0].Pass != mask || got.Cache[0].Type != "node" {
		t.Fatalf("redis: %+v", got.Cache[0])
	}
	if len(got.Etcd.Hosts) != 1 || got.Etcd.Hosts[0] != "etcd:2379" || got.Etcd.Pass != mask || got.Etcd.Key != mask {
		t.Fatalf("etcd: %+v", got.Etcd)
	}
	if got.Minio.Endpoint != "minio:9000" || got.Minio.UseSSL || got.Minio.AccessKeyID != mask || got.Minio.SecretAccessKey != mask {
		t.Fatalf("minio: %+v", got.Minio)
	}
	if got.TurnSecret != mask || got.RTMPKey != mask || got.KeyFile != mask {
		t.Fatalf("named secrets: turn %q rtmp %q keyfile %q", got.TurnSecret, got.RTMPKey, got.KeyFile)
	}
	if string(got.KeyPEM) != mask {
		t.Fatalf("KeyPEM = %q", got.KeyPEM)
	}
	if got.RTMPURL != in.RTMPURL || got.TermsURL != in.TermsURL || got.FixedCode != "12345" {
		t.Fatalf("plain strings changed: %+v", got)
	}
	if got.IDMap["/mtproto.RPCPasskey"] != "bff.bff" || got.IDMap["/mtproto.RPCSecretChats"] != "bff.bff" || got.IDMap["/mtproto.RPCPassport"] != "bff.bff" {
		t.Fatalf("IDMap redacted route targets: %+v", got.IDMap)
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, secret := range []string{dbPass, readPass, redisPass, etcdPass, chanPass, accessKey, secretKey, turn, pem, "BEGIN RSA PRIVATE KEY"} {
		if strings.Contains(text, secret) {
			t.Fatalf("json log leaked %q: %s", secret, text)
		}
	}
	if !strings.Contains(text, "mysql:3306") || !strings.Contains(text, "bff.bff") || !strings.Contains(text, "redis:6379") {
		t.Fatalf("json log dropped non-secrets: %s", text)
	}
	if strings.Contains(text, rtmpKey) && strings.Contains(text, `"RTMPKey":"`+rtmpKey+`"`) {
		t.Fatalf("json log leaked RTMPKey: %s", text)
	}
}

func TestValueDoesNotAliasPointers(t *testing.T) {
	type inner struct {
		Host string
		Pass string
	}
	type outer struct {
		In   *inner
		Name string
	}
	in := outer{In: &inner{Host: "redis:6379", Pass: "redis-secret"}, Name: "session"}
	got := Value(&in).(*outer)
	got.In.Pass = "changed"
	got.Name = "other"
	if in.In.Pass != "redis-secret" || in.Name != "session" {
		t.Fatalf("pointer copy aliased the live config: %+v", in)
	}
	if got.In.Host != "redis:6379" {
		t.Fatalf("host = %q", got.In.Host)
	}
	again := Value(in).(outer)
	if again.In == in.In {
		t.Fatal("nested pointer was not copied")
	}
	if again.In.Pass != mask {
		t.Fatalf("nested pass = %q", again.In.Pass)
	}
}

func TestValueNilAndEmpty(t *testing.T) {
	if Value(nil) != nil {
		t.Fatal("nil")
	}
	var p *struct{ Pass string }
	gotp := Value(p).(*struct{ Pass string })
	if gotp != nil {
		t.Fatalf("typed nil became %#v", gotp)
	}
	type cfg struct {
		Pass            string
		SecretAccessKey string
		KeyPEM          []byte
	}
	got := Value(cfg{}).(cfg)
	if got.Pass != "" || got.SecretAccessKey != "" || got.KeyPEM != nil {
		t.Fatalf("empty secrets changed: %+v", got)
	}
}

func TestValueKeepsUnexportedFieldsUnmodified(t *testing.T) {
	type cfg struct {
		Endpoint        string
		SecretAccessKey string
		transport       http.RoundTripper
	}
	rt := http.DefaultTransport
	in := cfg{Endpoint: "minio:9000", SecretAccessKey: "miniostorage", transport: rt}
	got := Value(in).(cfg)
	if in.SecretAccessKey != "miniostorage" || in.transport != rt {
		t.Fatal("live config changed")
	}
	if got.Endpoint != "minio:9000" || got.SecretAccessKey != mask {
		t.Fatalf("exported fields: %+v", got)
	}
}

func TestValueJSONName(t *testing.T) {
	type cfg struct {
		Token string `json:"access_key"`
		Note  string `json:"note"`
	}
	got := Value(cfg{Token: "secret-token", Note: "visible"}).(cfg)
	if got.Token != mask || got.Note != "visible" {
		t.Fatalf("%+v", got)
	}
}
