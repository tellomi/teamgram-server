package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/jsonx"
)

func TestHelpGetConfigDCOptionsFromEnv(t *testing.T) {
	restoreConfig(t)
	loadCheckedInConfig(t)
	t.Setenv("TELLOMI_DC_OPTIONS", "1:8.137.39.82:10443,1:8.137.39.82:5222")
	t.Setenv("TELLOMI_DC_TXT_DOMAIN", "")
	t.Setenv("TELLOMI_PUBLIC_DC_HOST", "")

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
	t.Setenv("TELLOMI_PUBLIC_DC_HOST", "")

	raw, err := os.ReadFile(findConfigJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"8.156.40.105", "8.137.198.240", "10.20.0.218"} {
		if strings.Contains(string(raw), ip) {
			t.Fatalf("checked-in config.json contains %s", ip)
		}
	}
	if n := len(config.GetDcOptions()); n != 0 {
		t.Fatalf("checked-in config.json dc_options len %d", n)
	}
	got := callGetConfig(t)
	if len(got.GetDcOptions()) != 0 {
		t.Fatalf("dc_options %+v", got.GetDcOptions())
	}
	if got.GetWebfileDcId() != 1 {
		t.Fatalf("webfile_dc_id %d", got.GetWebfileDcId())
	}
	if got.GetVenueSearchUsername().GetValue() != "venue" {
		t.Fatalf("venue_search_username %q", got.GetVenueSearchUsername().GetValue())
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
	t.Setenv("TELLOMI_PUBLIC_DC_HOST", "")

	got := callGetConfig(t)
	if len(got.GetDcOptions()) != 0 {
		t.Fatalf("invalid spec replaced dc_options: %+v", got.GetDcOptions())
	}
	if got.GetWebfileDcId() != 1 {
		t.Fatalf("webfile_dc_id %d", got.GetWebfileDcId())
	}
}

func TestHelpGetConfigDCTxtDomainBlank(t *testing.T) {
	restoreConfig(t)
	loadCheckedInConfig(t)
	t.Setenv("TELLOMI_DC_OPTIONS", "1:127.0.0.1:10443")
	t.Setenv("TELLOMI_DC_TXT_DOMAIN", "-")
	t.Setenv("TELLOMI_PUBLIC_DC_HOST", "")

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

func TestHelpGetConfigPublicHostFromEmptyFile(t *testing.T) {
	restoreConfig(t)
	loadCheckedInConfig(t)
	t.Setenv("TELLOMI_DC_OPTIONS", "")
	t.Setenv("TELLOMI_DC_TXT_DOMAIN", "")
	t.Setenv("TELLOMI_PUBLIC_DC_HOST", "8.137.198.240")

	got := callGetConfig(t)
	assertRelayOptions(t, got.GetDcOptions())
	if got.GetWebfileDcId() != 1 {
		t.Fatalf("webfile_dc_id %d", got.GetWebfileDcId())
	}
	if got.GetDcTxtDomainName() != "apv2.stel.com" {
		t.Fatalf("domain %q", got.GetDcTxtDomainName())
	}
	wire := roundTripConfig(t, got)
	assertRelayOptions(t, wire.GetDcOptions())
	if wire.GetWebfileDcId() != 1 {
		t.Fatalf("wire webfile_dc_id %d", wire.GetWebfileDcId())
	}
}

func TestHelpGetConfigPublicHostReplacesOrigin(t *testing.T) {
	restoreConfig(t)
	loadCheckedInConfig(t)
	t.Setenv("TELLOMI_DC_OPTIONS", "1:8.156.40.105:10443,1:8.156.40.105:443")
	t.Setenv("TELLOMI_DC_TXT_DOMAIN", "-")
	t.Setenv("TELLOMI_PUBLIC_DC_HOST", "8.137.198.240")

	got := callGetConfig(t)
	assertRelayOptions(t, got.GetDcOptions())
	if got.GetDcTxtDomainName() != "" {
		t.Fatalf("domain %q", got.GetDcTxtDomainName())
	}
	wire := roundTripConfig(t, got)
	assertRelayOptions(t, wire.GetDcOptions())
	for _, opt := range wire.GetDcOptions() {
		if opt.GetIpAddress() == "8.156.40.105" || opt.GetIpAddress() == "10.20.0.218" {
			t.Fatalf("origin leaked: %s", opt.GetIpAddress())
		}
	}
}

func TestHelpGetConfigKeepsMediaAndCDNFlags(t *testing.T) {
	restoreConfig(t)
	loadCheckedInConfig(t)
	t.Setenv("TELLOMI_DC_OPTIONS", "")
	t.Setenv("TELLOMI_DC_TXT_DOMAIN", "")
	t.Setenv("TELLOMI_PUBLIC_DC_HOST", "8.137.198.240")
	config.SetDcOptions([]*mtproto.DcOption{
		{
			PredicateName: mtproto.Predicate_dcOption,
			Constructor:   mtproto.TLConstructor_CRC32_dcOption,
			Id:            1,
			IpAddress:     "8.156.40.105",
			Port:          10443,
			MediaOnly:     true,
			ThisPortOnly:  true,
		},
		{
			PredicateName: mtproto.Predicate_dcOption,
			Constructor:   mtproto.TLConstructor_CRC32_dcOption,
			Id:            4,
			IpAddress:     "8.156.40.105",
			Port:          443,
			Cdn:           true,
			ThisPortOnly:  true,
		},
	})

	got := callGetConfig(t)
	opts := got.GetDcOptions()
	if len(opts) != 2 {
		t.Fatalf("len %d", len(opts))
	}
	if opts[0].GetIpAddress() != "8.137.198.240" || opts[0].GetPort() != 10443 || !opts[0].GetMediaOnly() || opts[0].GetCdn() {
		t.Fatalf("media ip=%s port=%d media=%v cdn=%v", opts[0].GetIpAddress(), opts[0].GetPort(), opts[0].GetMediaOnly(), opts[0].GetCdn())
	}
	if opts[1].GetIpAddress() != "8.137.198.240" || opts[1].GetPort() != 443 || opts[1].GetId() != 4 || !opts[1].GetCdn() || opts[1].GetMediaOnly() {
		t.Fatalf("cdn ip=%s port=%d id=%d cdn=%v media=%v", opts[1].GetIpAddress(), opts[1].GetPort(), opts[1].GetId(), opts[1].GetCdn(), opts[1].GetMediaOnly())
	}
	wire := roundTripConfig(t, got)
	if len(wire.GetDcOptions()) != 2 || !wire.GetDcOptions()[0].GetMediaOnly() || !wire.GetDcOptions()[1].GetCdn() {
		t.Fatalf("wire lost flags")
	}
	if wire.GetDcOptions()[0].GetIpAddress() != "8.137.198.240" || wire.GetDcOptions()[1].GetIpAddress() != "8.137.198.240" {
		t.Fatal("wire kept the origin address")
	}
}

func TestHelpGetConfigPublicHostDrops5222(t *testing.T) {
	restoreConfig(t)
	loadCheckedInConfig(t)
	t.Setenv("TELLOMI_DC_OPTIONS", "1:8.156.40.105:10443,1:8.156.40.105:5222,2:8.156.40.105:443")
	t.Setenv("TELLOMI_DC_TXT_DOMAIN", "")
	t.Setenv("TELLOMI_PUBLIC_DC_HOST", "8.137.198.240")

	got := callGetConfig(t)
	opts := got.GetDcOptions()
	if len(opts) != 2 {
		t.Fatalf("len %d", len(opts))
	}
	if opts[0].GetId() != 1 || opts[0].GetPort() != 10443 || opts[0].GetIpAddress() != "8.137.198.240" {
		t.Fatalf("first id=%d ip=%s port=%d", opts[0].GetId(), opts[0].GetIpAddress(), opts[0].GetPort())
	}
	if opts[1].GetId() != 2 || opts[1].GetPort() != 443 || opts[1].GetIpAddress() != "8.137.198.240" {
		t.Fatalf("second id=%d ip=%s port=%d", opts[1].GetId(), opts[1].GetIpAddress(), opts[1].GetPort())
	}
	wire := roundTripConfig(t, got)
	for _, opt := range wire.GetDcOptions() {
		if opt.GetPort() == 5222 || opt.GetIpAddress() == "8.156.40.105" {
			t.Fatalf("wire published %s:%d", opt.GetIpAddress(), opt.GetPort())
		}
	}
}

func TestHelpGetConfigPublicHostKeepsMappedIPv6(t *testing.T) {
	restoreConfig(t)
	loadCheckedInConfig(t)
	t.Setenv("TELLOMI_DC_OPTIONS", "")
	t.Setenv("TELLOMI_DC_TXT_DOMAIN", "")
	t.Setenv("TELLOMI_PUBLIC_DC_HOST", "8.137.198.240")
	config.SetDcOptions([]*mtproto.DcOption{{
		PredicateName: mtproto.Predicate_dcOption,
		Constructor:   mtproto.TLConstructor_CRC32_dcOption,
		Id:            2,
		IpAddress:     "::ffff:8.156.40.105",
		Port:          443,
		Ipv6:          true,
		ThisPortOnly:  true,
	}})

	got := callGetConfig(t)
	opts := got.GetDcOptions()
	if len(opts) != 1 {
		t.Fatalf("len %d", len(opts))
	}
	if opts[0].GetIpAddress() != "::ffff:8.156.40.105" || !opts[0].GetIpv6() || opts[0].GetId() != 2 || opts[0].GetPort() != 443 {
		t.Fatalf("ip=%s ipv6=%v id=%d port=%d", opts[0].GetIpAddress(), opts[0].GetIpv6(), opts[0].GetId(), opts[0].GetPort())
	}
	wire := roundTripConfig(t, got)
	w := wire.GetDcOptions()
	if len(w) != 1 {
		t.Fatalf("wire len %d", len(w))
	}
	if w[0].GetIpAddress() != "::ffff:8.156.40.105" || !w[0].GetIpv6() {
		t.Fatalf("wire ip=%s ipv6=%v", w[0].GetIpAddress(), w[0].GetIpv6())
	}
}

func assertRelayOptions(t *testing.T, opts []*mtproto.DcOption) {
	t.Helper()
	if len(opts) != 2 {
		t.Fatalf("dc_options len %d", len(opts))
	}
	for i, port := range []int32{10443, 443} {
		opt := opts[i]
		if opt.GetPredicateName() != mtproto.Predicate_dcOption {
			t.Fatalf("entry %d predicate %q", i, opt.GetPredicateName())
		}
		if opt.GetId() != 1 || opt.GetIpAddress() != "8.137.198.240" || opt.GetPort() != port {
			t.Fatalf("entry %d id=%d ip=%s port=%d", i, opt.GetId(), opt.GetIpAddress(), opt.GetPort())
		}
		if opt.GetMediaOnly() || opt.GetCdn() || opt.GetTcpoOnly() || opt.GetStatic() || opt.GetIpv6() || !opt.GetThisPortOnly() {
			t.Fatalf("entry %d flags media=%v cdn=%v tcpo=%v static=%v ipv6=%v thisPort=%v",
				i, opt.GetMediaOnly(), opt.GetCdn(), opt.GetTcpoOnly(), opt.GetStatic(), opt.GetIpv6(), opt.GetThisPortOnly())
		}
	}
}

func roundTripConfig(t *testing.T, got *mtproto.Config) *mtproto.Config {
	t.Helper()
	buf := mtproto.NewEncodeBuf(8192)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded := &mtproto.Config{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return decoded
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
