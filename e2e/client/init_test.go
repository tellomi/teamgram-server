package e2e

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestInitConnectionInvokeWithLayer(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	for _, profile := range []Profile{ProfileIOS, ProfileAndroid} {
		t.Run(profile.LangPack, func(t *testing.T) {
			a.SetProfile(profile)
			obj := invokeOK(t, a, &mtproto.TLHelpGetConfig{Constructor: mtproto.CRC32_help_getConfig})
			cfg, ok := obj.(*mtproto.TLConfig)
			if !ok {
				t.Fatalf("help.getConfig returned %T", obj)
			}
			if cfg.GetThisDc() == 0 && cfg.To_Config().GetThisDc() == 0 {
				t.Fatal("config.this_dc is 0")
			}
		})
	}
	a.SetProfile(ProfileIOS)
}

func TestHelpGetConfig(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	a.SetProfile(ProfileIOS)
	obj := invokeOK(t, a, &mtproto.TLHelpGetConfig{Constructor: mtproto.CRC32_help_getConfig})
	cfg, ok := obj.(*mtproto.TLConfig)
	if !ok {
		t.Fatalf("returned %T", obj)
	}
	if cfg.GetMeUrlPrefix() == "" {
		t.Fatal("me_url_prefix is empty")
	}
	assertDCOptions(t, cfg)
}

// assertDCOptions checks the list the server was started with.
// TELLOMI_E2E_DC_OPTIONS empty (production smoke, local stacks) allows
// whatever the server has, including the checked-in empty list.
func assertDCOptions(t *testing.T, cfg *mtproto.TLConfig) {
	t.Helper()
	spec := strings.TrimSpace(os.Getenv("TELLOMI_E2E_DC_OPTIONS"))
	if spec == "" {
		return
	}
	want := parseDCSpec(t, spec)
	got := cfg.GetDcOptions()
	if len(got) != len(want) {
		t.Fatalf("dc_options len %d, want %d (a missing predicate_name is dropped on encode)", len(got), len(want))
	}
	for i, opt := range got {
		if opt.GetPredicateName() != mtproto.Predicate_dcOption {
			t.Fatalf("entry %d predicate %q", i, opt.GetPredicateName())
		}
		if opt.GetId() != want[i].id || opt.GetIpAddress() != want[i].ip || opt.GetPort() != want[i].port {
			t.Fatalf("entry %d id=%d ip=%s port=%d, want id=%d ip=%s port=%d",
				i, opt.GetId(), opt.GetIpAddress(), opt.GetPort(), want[i].id, want[i].ip, want[i].port)
		}
		if opt.GetMediaOnly() || opt.GetCdn() || opt.GetTcpoOnly() || opt.GetStatic() || opt.GetIpv6() || !opt.GetThisPortOnly() {
			t.Fatalf("entry %d flags media=%v cdn=%v tcpo=%v static=%v ipv6=%v thisPort=%v",
				i, opt.GetMediaOnly(), opt.GetCdn(), opt.GetTcpoOnly(), opt.GetStatic(), opt.GetIpv6(), opt.GetThisPortOnly())
		}
	}
	if cfg.GetWebfileDcId() != 1 {
		t.Fatalf("webfile_dc_id %d, want 1", cfg.GetWebfileDcId())
	}
	if cfg.GetDcTxtDomainName() != "apv2.stel.com" {
		t.Fatalf("dc_txt_domain_name %q", cfg.GetDcTxtDomainName())
	}
}

type dcSpecEntry struct {
	id   int32
	ip   string
	port int32
}

func parseDCSpec(t *testing.T, spec string) []dcSpecEntry {
	t.Helper()
	var out []dcSpecEntry
	for _, part := range strings.Split(spec, ",") {
		fields := strings.Split(strings.TrimSpace(part), ":")
		if len(fields) != 3 {
			t.Fatalf("TELLOMI_E2E_DC_OPTIONS entry %q", part)
		}
		id, err := strconv.Atoi(fields[0])
		if err != nil {
			t.Fatal(err)
		}
		port, err := strconv.Atoi(fields[2])
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, dcSpecEntry{id: int32(id), ip: fields[1], port: int32(port)})
	}
	return out
}

func TestHelpGetAppConfig(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	obj := invokeOK(t, a, &mtproto.TLHelpGetAppConfig61E3F854{Constructor: mtproto.CRC32_help_getAppConfig61E3F854})
	switch obj.(type) {
	case *mtproto.TLHelpAppConfig, *mtproto.TLHelpAppConfigNotModified:
	default:
		t.Fatalf("returned %T", obj)
	}
}

func TestUpdatesGetStateAndDifference(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	c, cancel := ctx()
	defer cancel()
	pts, qts, date, err := a.State(c)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := a.Difference(c, pts, qts, date)
	if err != nil {
		t.Fatal(err)
	}
	switch diff.(type) {
	case *mtproto.TLUpdatesDifference, *mtproto.TLUpdatesDifferenceSlice, *mtproto.TLUpdatesDifferenceEmpty, *mtproto.TLUpdatesDifferenceTooLong:
	default:
		t.Fatalf("updates.getDifference returned %T", diff)
	}
}
