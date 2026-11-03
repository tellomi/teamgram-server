package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/jsonx"
)

func TestHelpGetConfigDCOptionsFromEnv(t *testing.T) {
	restoreConfig(t)
	loadCheckedInConfig(t)
	t.Setenv("TELLOMI_DC_OPTIONS", "1:8.137.39.82:10443,1:8.137.39.82:5222")
	t.Setenv("TELLOMI_DC_TXT_DOMAIN", "")

	got := callGetConfig(t)
	opts := got.GetDcOptions()
	if len(opts) != 2 {
		t.Fatalf("dc_options len %d", len(opts))
	}
	wantPorts := []int32{10443, 5222}
	for i, opt := range opts {
		if opt.GetPredicateName() != mtproto.Predicate_dcOption {
			t.Fatalf("predicate %q", opt.GetPredicateName())
		}
		if opt.GetId() != 1 || opt.GetIpAddress() != "8.137.39.82" || opt.GetPort() != wantPorts[i] {
			t.Fatalf("entry %d id=%d ip=%s port=%d", i, opt.GetId(), opt.GetIpAddress(), opt.GetPort())
		}
		if opt.GetMediaOnly() || opt.GetCdn() || opt.GetTcpoOnly() || opt.GetStatic() || opt.GetIpv6() || !opt.GetThisPortOnly() {
			t.Fatalf("flags media=%v cdn=%v tcpo=%v static=%v ipv6=%v thisPort=%v",
				opt.GetMediaOnly(), opt.GetCdn(), opt.GetTcpoOnly(), opt.GetStatic(), opt.GetIpv6(), opt.GetThisPortOnly())
		}
	}
	if got.GetWebfileDcId() != 1 {
		t.Fatalf("webfile_dc_id %d", got.GetWebfileDcId())
	}
	if got.GetDcTxtDomainName() != "apv2.stel.com" {
		t.Fatalf("domain %q", got.GetDcTxtDomainName())
	}

	buf := mtproto.NewEncodeBuf(8192)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded := &mtproto.Config{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatalf("decode: %v", err)
	}
	wire := decoded.GetDcOptions()
	if len(wire) != 2 {
		t.Fatalf("wire dc_options len %d (an entry without predicate_name is dropped)", len(wire))
	}
	if wire[0].GetIpAddress() != "8.137.39.82" || wire[0].GetPort() != 10443 || !wire[0].GetThisPortOnly() {
		t.Fatalf("wire[0] ip=%s port=%d thisPort=%v", wire[0].GetIpAddress(), wire[0].GetPort(), wire[0].GetThisPortOnly())
	}
	if wire[1].GetPort() != 5222 || wire[1].GetMediaOnly() || wire[1].GetStatic() {
		t.Fatalf("wire[1] port=%d media=%v static=%v", wire[1].GetPort(), wire[1].GetMediaOnly(), wire[1].GetStatic())
	}
	if decoded.GetWebfileDcId() != 1 {
		t.Fatalf("wire webfile_dc_id %d", decoded.GetWebfileDcId())
	}
}

func TestHelpGetConfigDCOptionsUnsetMatchesFile(t *testing.T) {
	restoreConfig(t)
	loadCheckedInConfig(t)
	t.Setenv("TELLOMI_DC_OPTIONS", "")
	t.Setenv("TELLOMI_DC_TXT_DOMAIN", "")

	if n := len(config.GetDcOptions()); n != 0 {
		t.Fatalf("checked-in config.json dc_options len %d", n)
	}
	got := callGetConfig(t)
	if len(got.GetDcOptions()) != 0 {
		t.Fatalf("dc_options %+v", got.GetDcOptions())
	}
	if got.GetWebfileDcId() != 4 {
		t.Fatalf("webfile_dc_id %d", got.GetWebfileDcId())
	}
	if got.GetDcTxtDomainName() != "apv2.stel.com" {
		t.Fatalf("domain %q", got.GetDcTxtDomainName())
	}
}

func TestHelpGetConfigDCOptionsInvalidDoesNotCrash(t *testing.T) {
	restoreConfig(t)
	loadCheckedInConfig(t)
	t.Setenv("TELLOMI_DC_OPTIONS", "1:api.tellomi.cn:10443")
	t.Setenv("TELLOMI_DC_TXT_DOMAIN", "")

	got := callGetConfig(t)
	if len(got.GetDcOptions()) != 0 {
		t.Fatalf("invalid spec replaced dc_options: %+v", got.GetDcOptions())
	}
	if got.GetWebfileDcId() != 4 {
		t.Fatalf("webfile_dc_id %d", got.GetWebfileDcId())
	}
}

func TestHelpGetConfigDCTxtDomainBlank(t *testing.T) {
	restoreConfig(t)
	loadCheckedInConfig(t)
	t.Setenv("TELLOMI_DC_OPTIONS", "1:127.0.0.1:10443")
	t.Setenv("TELLOMI_DC_TXT_DOMAIN", "-")

	got := callGetConfig(t)
	if len(got.GetDcOptions()) != 1 || got.GetDcOptions()[0].GetIpAddress() != "127.0.0.1" {
		t.Fatalf("options %+v", got.GetDcOptions())
	}
	if got.GetDcTxtDomainName() != "" {
		t.Fatalf("domain %q", got.GetDcTxtDomainName())
	}
	if got.GetWebfileDcId() != 1 {
		t.Fatalf("webfile_dc_id %d", got.GetWebfileDcId())
	}
}

func callGetConfig(t *testing.T) *mtproto.Config {
	t.Helper()
	c := &ConfigurationCore{ctx: context.Background()}
	got, err := c.HelpGetConfig(&mtproto.TLHelpGetConfig{Constructor: mtproto.CRC32_help_getConfig})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("nil config")
	}
	return got
}

func restoreConfig(t *testing.T) {
	t.Helper()
	saved := config
	t.Cleanup(func() { config = saved })
}

func loadCheckedInConfig(t *testing.T) {
	t.Helper()
	path := findConfigJSON(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	config = mtproto.TLConfig{}
	if err := jsonx.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if config.GetData2() == nil {
		t.Fatal("config.json did not fill data2")
	}
}

func findConfigJSON(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		candidate := filepath.Join(dir, "teamgramd", "bin", "config.json")
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("teamgramd/bin/config.json not found")
		}
		dir = parent
	}
}
