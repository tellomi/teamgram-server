// Copyright 2026 Tellomi Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0

package dcoptions

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestFileDC(t *testing.T) {
	t.Setenv(envOptions, "")
	t.Setenv(envPublic, "8.137.198.240")
	if FileDC() != 1 {
		t.Fatalf("default dc %d", FileDC())
	}
	t.Setenv(envOptions, "2:8.137.39.82:10443")
	if FileDC() != 2 {
		t.Fatalf("configured dc %d", FileDC())
	}
	t.Setenv(envOptions, "not-a-spec")
	if FileDC() != 1 {
		t.Fatalf("invalid spec dc %d", FileDC())
	}
}

func TestParseProductionSpec(t *testing.T) {
	opts, err := Parse("1:8.137.39.82:10443,1:8.137.39.82:5222")
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 2 {
		t.Fatalf("len %d", len(opts))
	}
	for i, port := range []int32{10443, 5222} {
		opt := opts[i]
		if opt.GetPredicateName() != mtproto.Predicate_dcOption {
			t.Fatalf("predicate %q", opt.GetPredicateName())
		}
		if opt.GetId() != 1 || opt.GetIpAddress() != "8.137.39.82" || opt.GetPort() != port {
			t.Fatalf("entry %d: id %d ip %s port %d", i, opt.GetId(), opt.GetIpAddress(), opt.GetPort())
		}
		if opt.GetIpv6() || opt.GetMediaOnly() || opt.GetTcpoOnly() || opt.GetCdn() || opt.GetStatic() || !opt.GetThisPortOnly() {
			t.Fatalf("flags ipv6=%v media=%v tcpo=%v cdn=%v static=%v thisPort=%v",
				opt.GetIpv6(), opt.GetMediaOnly(), opt.GetTcpoOnly(), opt.GetCdn(), opt.GetStatic(), opt.GetThisPortOnly())
		}
		if len(opt.GetSecret()) != 0 {
			t.Fatalf("secret %d bytes", len(opt.GetSecret()))
		}
	}
}

func TestParseTrimsEntries(t *testing.T) {
	opts, err := Parse(" 1:127.0.0.1:10443, 1:127.0.0.1:5222 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 2 || opts[1].GetPort() != 5222 || opts[0].GetIpAddress() != "127.0.0.1" {
		t.Fatalf("%+v", opts)
	}
}

func TestParseRejects(t *testing.T) {
	cases := []string{
		"1:api.tellomi.cn:10443",
		"1:8.137.39.82",
		"8.137.39.82:10443",
		"1:8.137.39.82:10443:1",
		"1:2001:db8::1:10443",
		"1:::1:10443",
		"0:8.137.39.82:10443",
		"1:8.137.39.82:0",
		"1:8.137.39.82:65536",
		"1:8.137.39.82:10443,",
		"1:8.137.039.82:10443",
		"1:8.137.39.82:010443",
		"01:8.137.39.82:10443",
		"+1:8.137.39.82:10443",
		"1:8.137.39.82:10443, 1:8.137.39.82:5222 extra",
		"",
		"   ",
		strings.Repeat("1:8.137.39.82:1,", maxEntries),
		strings.Repeat("x", maxSpecLen+1),
	}
	for _, spec := range cases {
		if _, err := Parse(spec); err == nil {
			t.Errorf("accepted %q", spec)
		}
	}
}

func TestApplyUnsetKeepsFile(t *testing.T) {
	t.Setenv(envOptions, "")
	t.Setenv(envDomain, "")
	t.Setenv(envPublic, "")
	cfg := fileShape()
	Apply(cfg)
	if got := cfg.GetDcOptions(); len(got) != 1 || got[0].GetIpAddress() != "203.0.113.9" {
		t.Fatalf("options changed: %+v", got)
	}
	if cfg.GetWebfileDcId() != 4 || cfg.GetDcTxtDomainName() != "apv2.stel.com" {
		t.Fatalf("webfile %d domain %q", cfg.GetWebfileDcId(), cfg.GetDcTxtDomainName())
	}
}

func TestApplyInvalidKeepsFile(t *testing.T) {
	t.Setenv(envOptions, "1:api.tellomi.cn:10443")
	t.Setenv(envDomain, "")
	t.Setenv(envPublic, "")
	cfg := fileShape()
	Apply(cfg)
	if got := cfg.GetDcOptions(); len(got) != 1 || got[0].GetPort() != 9 {
		t.Fatalf("options changed: %+v", got)
	}
	if cfg.GetWebfileDcId() != 4 {
		t.Fatalf("webfile %d", cfg.GetWebfileDcId())
	}
}

func TestApplyReplacesAndFixesWebfile(t *testing.T) {
	t.Setenv(envOptions, "1:8.137.39.82:10443,1:8.137.39.82:5222")
	t.Setenv(envDomain, "")
	t.Setenv(envPublic, "")
	cfg := fileShape()
	Apply(cfg)
	opts := cfg.GetDcOptions()
	if len(opts) != 2 || opts[0].GetPort() != 10443 || opts[1].GetPort() != 5222 {
		t.Fatalf("%+v", opts)
	}
	if opts[0].GetIpAddress() == "203.0.113.9" {
		t.Fatal("file entry was kept")
	}
	if cfg.GetWebfileDcId() != 1 {
		t.Fatalf("webfile %d", cfg.GetWebfileDcId())
	}
	if cfg.GetDcTxtDomainName() != "apv2.stel.com" {
		t.Fatalf("domain %q", cfg.GetDcTxtDomainName())
	}
}

func TestApplyDomain(t *testing.T) {
	t.Setenv(envOptions, "")
	t.Setenv(envPublic, "")
	cfg := fileShape()

	t.Setenv(envDomain, "dc.example")
	Apply(cfg)
	if cfg.GetDcTxtDomainName() != "dc.example" {
		t.Fatalf("domain %q", cfg.GetDcTxtDomainName())
	}

	t.Setenv(envDomain, " - ")
	Apply(cfg)
	if cfg.GetDcTxtDomainName() != "" {
		t.Fatalf("domain %q", cfg.GetDcTxtDomainName())
	}

	cfg.SetDcTxtDomainName("apv2.stel.com")
	t.Setenv(envDomain, "")
	Apply(cfg)
	if cfg.GetDcTxtDomainName() != "apv2.stel.com" {
		t.Fatalf("empty domain overwrote %q", cfg.GetDcTxtDomainName())
	}
}

func TestApplyInvalidSpecStillAppliesDomain(t *testing.T) {
	t.Setenv(envOptions, "not-an-address")
	t.Setenv(envDomain, "-")
	t.Setenv(envPublic, "")
	cfg := fileShape()
	Apply(cfg)
	if cfg.GetWebfileDcId() != 4 || len(cfg.GetDcOptions()) != 1 {
		t.Fatalf("options changed webfile=%d len=%d", cfg.GetWebfileDcId(), len(cfg.GetDcOptions()))
	}
	if cfg.GetDcTxtDomainName() != "" {
		t.Fatalf("domain %q", cfg.GetDcTxtDomainName())
	}
}

func TestApplyNil(t *testing.T) {
	Apply(nil)
	Apply(&mtproto.TLConfig{})
}

func TestParseRelayPorts(t *testing.T) {
	opts, err := Parse("1:8.137.198.240:10443,1:8.137.198.240:443")
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 2 {
		t.Fatalf("len %d", len(opts))
	}
	for i, port := range []int32{10443, 443} {
		opt := opts[i]
		if opt.GetId() != 1 || opt.GetIpAddress() != "8.137.198.240" || opt.GetPort() != port {
			t.Fatalf("entry %d id=%d ip=%s port=%d", i, opt.GetId(), opt.GetIpAddress(), opt.GetPort())
		}
		if opt.GetIpv6() || opt.GetMediaOnly() || opt.GetCdn() || !opt.GetThisPortOnly() {
			t.Fatalf("flags ipv6=%v media=%v cdn=%v thisPort=%v", opt.GetIpv6(), opt.GetMediaOnly(), opt.GetCdn(), opt.GetThisPortOnly())
		}
	}
}

func TestApplyPublicHostRewritesOriginAndKeepsFlags(t *testing.T) {
	t.Setenv(envOptions, "")
	t.Setenv(envDomain, "")
	t.Setenv(envPublic, "8.137.198.240")
	origin := &mtproto.DcOption{
		PredicateName: mtproto.Predicate_dcOption,
		Constructor:   mtproto.TLConstructor_CRC32_dcOption,
		Id:            1,
		IpAddress:     "8.156.40.105",
		Port:          10443,
		MediaOnly:     true,
		ThisPortOnly:  true,
	}
	cdnOpt := &mtproto.DcOption{
		PredicateName: mtproto.Predicate_dcOption,
		Constructor:   mtproto.TLConstructor_CRC32_dcOption,
		Id:            2,
		IpAddress:     "10.20.0.218",
		Port:          443,
		Cdn:           true,
		ThisPortOnly:  true,
	}
	domainOpt := &mtproto.DcOption{
		PredicateName: mtproto.Predicate_dcOption,
		Constructor:   mtproto.TLConstructor_CRC32_dcOption,
		Id:            1,
		IpAddress:     "api.tellomi.cn",
		Port:          10443,
		ThisPortOnly:  true,
	}
	cfg := fileShape()
	cfg.SetThisDc(1)
	cfg.SetDcOptions([]*mtproto.DcOption{origin, cdnOpt, domainOpt})
	Apply(cfg)
	if origin.GetIpAddress() != "8.156.40.105" || cdnOpt.GetIpAddress() != "10.20.0.218" {
		t.Fatal("Apply wrote into the caller's dcOption")
	}
	opts := cfg.GetDcOptions()
	if len(opts) != 3 {
		t.Fatalf("len %d", len(opts))
	}
	var media, cdnEntry, domain *mtproto.DcOption
	for _, opt := range opts {
		switch {
		case opt.GetMediaOnly():
			media = opt
		case opt.GetCdn():
			cdnEntry = opt
		default:
			domain = opt
		}
	}
	if media == nil || media.GetIpAddress() != "8.137.198.240" || media.GetPort() != 10443 || media.GetCdn() || media.GetId() != 1 {
		t.Fatalf("media %+v", media)
	}
	if cdnEntry == nil || cdnEntry.GetIpAddress() != "8.137.198.240" || cdnEntry.GetPort() != 443 || cdnEntry.GetId() != 2 || cdnEntry.GetMediaOnly() {
		t.Fatalf("cdn %+v", cdnEntry)
	}
	if domain == nil || domain.GetIpAddress() != "api.tellomi.cn" || domain.GetPort() != 10443 || domain.GetId() != 1 {
		t.Fatalf("domain %+v", domain)
	}
	if cfg.GetDcTxtDomainName() != "apv2.stel.com" || cfg.GetThisDc() != 1 || cfg.GetWebfileDcId() != 1 {
		t.Fatalf("domain %q this %d webfile %d", cfg.GetDcTxtDomainName(), cfg.GetThisDc(), cfg.GetWebfileDcId())
	}
	if FileDC() != 1 {
		t.Fatalf("file dc %d", FileDC())
	}
}

func TestApplyPublicHostFillsEmptyList(t *testing.T) {
	t.Setenv(envOptions, "")
	t.Setenv(envDomain, "dc.example")
	t.Setenv(envPublic, " 8.137.198.240 ")
	cfg := &mtproto.TLConfig{Data2: &mtproto.Config{
		PredicateName:   mtproto.Predicate_config,
		DcTxtDomainName: "apv2.stel.com",
		WebfileDcId:     4,
		ThisDc:          1,
	}}
	Apply(cfg)
	opts := cfg.GetDcOptions()
	if len(opts) != 2 {
		t.Fatalf("len %d", len(opts))
	}
	for i, port := range []int32{10443, 443} {
		opt := opts[i]
		if opt.GetPredicateName() != mtproto.Predicate_dcOption || opt.GetId() != 1 || opt.GetIpAddress() != "8.137.198.240" || opt.GetPort() != port {
			t.Fatalf("entry %d id=%d ip=%s port=%d predicate=%s", i, opt.GetId(), opt.GetIpAddress(), opt.GetPort(), opt.GetPredicateName())
		}
		if opt.GetIpv6() || opt.GetMediaOnly() || opt.GetCdn() || opt.GetStatic() || opt.GetTcpoOnly() || !opt.GetThisPortOnly() || len(opt.GetSecret()) != 0 {
			t.Fatalf("entry %d flags", i)
		}
	}
	if cfg.GetWebfileDcId() != 1 || cfg.GetDcTxtDomainName() != "dc.example" || cfg.GetThisDc() != 1 {
		t.Fatalf("webfile %d domain %q this %d", cfg.GetWebfileDcId(), cfg.GetDcTxtDomainName(), cfg.GetThisDc())
	}
}

func TestApplyPublicHostRewritesOptionsList(t *testing.T) {
	t.Setenv(envOptions, "1:8.156.40.105:10443,1:8.156.40.105:443")
	t.Setenv(envDomain, "")
	t.Setenv(envPublic, "8.137.198.240")
	cfg := fileShape()
	Apply(cfg)
	opts := cfg.GetDcOptions()
	if len(opts) != 2 || opts[0].GetPort() != 10443 || opts[1].GetPort() != 443 {
		t.Fatalf("%+v", opts)
	}
	for _, opt := range opts {
		if opt.GetIpAddress() != "8.137.198.240" || opt.GetMediaOnly() || opt.GetCdn() || !opt.GetThisPortOnly() {
			t.Fatalf("%+v", opt)
		}
	}
}

func TestApplyPublicHostKeepsProductionDC2(t *testing.T) {
	spec := "1:8.137.198.240:10443,1:8.137.198.240:443,2:8.137.198.240:10443,2:8.137.198.240:443"
	t.Setenv(envOptions, spec)
	t.Setenv(envDomain, "")
	t.Setenv(envPublic, "8.137.198.240")
	cfg := fileShape()
	Apply(cfg)
	opts := cfg.GetDcOptions()
	wantID := []int32{1, 1, 2, 2}
	wantPort := []int32{10443, 443, 10443, 443}
	if len(opts) != len(wantID) {
		t.Fatalf("len %d", len(opts))
	}
	for i, opt := range opts {
		if opt.GetId() != wantID[i] || opt.GetPort() != wantPort[i] || opt.GetIpAddress() != "8.137.198.240" {
			t.Fatalf("entry %d id=%d ip=%s port=%d", i, opt.GetId(), opt.GetIpAddress(), opt.GetPort())
		}
	}
}

func TestApplyPublicHostDropsUnforwardedPort(t *testing.T) {
	t.Setenv(envOptions, "1:8.156.40.105:10443,1:8.156.40.105:5222,2:8.156.40.105:443,2:8.156.40.105:80")
	t.Setenv(envDomain, "")
	t.Setenv(envPublic, "8.137.198.240")
	cfg := fileShape()
	Apply(cfg)
	opts := cfg.GetDcOptions()
	if len(opts) != 2 {
		t.Fatalf("len %d %+v", len(opts), opts)
	}
	if opts[0].GetId() != 1 || opts[0].GetPort() != 10443 || opts[0].GetIpAddress() != "8.137.198.240" {
		t.Fatalf("first %+v", opts[0])
	}
	if opts[1].GetId() != 2 || opts[1].GetPort() != 443 || opts[1].GetIpAddress() != "8.137.198.240" {
		t.Fatalf("second %+v", opts[1])
	}
	for _, opt := range opts {
		if opt.GetPort() == 5222 || opt.GetPort() == 80 || opt.GetIpAddress() == "8.156.40.105" {
			t.Fatalf("published %s:%d", opt.GetIpAddress(), opt.GetPort())
		}
	}

	t.Setenv(envOptions, "1:8.156.40.105:5222,2:8.156.40.105:443")
	mixed := fileShape()
	mixed.SetThisDc(1)
	Apply(mixed)
	got := mixed.GetDcOptions()
	if len(got) != 3 {
		t.Fatalf("mixed len %d %+v", len(got), got)
	}
	if got[0].GetId() != 1 || got[0].GetPort() != 10443 || got[0].GetIpAddress() != "8.137.198.240" {
		t.Fatalf("dc1 10443 %+v", got[0])
	}
	if got[1].GetId() != 1 || got[1].GetPort() != 443 || got[1].GetIpAddress() != "8.137.198.240" {
		t.Fatalf("dc1 443 %+v", got[1])
	}
	if got[2].GetId() != 2 || got[2].GetPort() != 443 || got[2].GetIpAddress() != "8.137.198.240" {
		t.Fatalf("dc2 %+v", got[2])
	}
	if err := Check(mixed); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPublicHostFallsBackPerDC(t *testing.T) {
	t.Setenv(envOptions, "2:8.156.40.105:5222")
	t.Setenv(envDomain, "")
	t.Setenv(envPublic, "8.137.198.240")
	cfg := fileShape()
	cfg.SetThisDc(1)
	before := rewriteRefusedLogCount.Load()
	Apply(cfg)
	if rewriteRefusedLogCount.Load() == before {
		t.Fatal("refused rewrite was not logged")
	}
	opts := cfg.GetDcOptions()
	if len(opts) != 1 || opts[0].GetId() != 2 || opts[0].GetPort() != 5222 || opts[0].GetIpAddress() != "8.156.40.105" {
		t.Fatalf("rewrite ran despite this_dc: %+v", opts)
	}
	for _, opt := range opts {
		if opt.GetId() == 1 {
			t.Fatalf("synthesized DC1: %+v", opt)
		}
	}
	if err := Check(cfg); err == nil {
		t.Fatal("this_dc 1 was left without an address")
	}

	okCfg := fileShape()
	okCfg.SetThisDc(2)
	Apply(okCfg)
	got := okCfg.GetDcOptions()
	if len(got) != 2 || got[0].GetId() != 2 || got[0].GetPort() != 10443 || got[1].GetId() != 2 || got[1].GetPort() != 443 {
		t.Fatalf("per-dc fallback %+v", got)
	}
	for _, opt := range got {
		if opt.GetId() == 1 || opt.GetIpAddress() != "8.137.198.240" || opt.GetPort() == 5222 {
			t.Fatalf("cross-dc or 5222: %+v", opt)
		}
	}
}

func TestInvalidOptionSpecDoesNotPublishDC1(t *testing.T) {
	t.Setenv(envOptions, "0:8.137.198.240:10443,2:8.137.198.240:443")
	t.Setenv(envDomain, "")
	t.Setenv(envPublic, "8.137.198.240")
	cfg := fileShape()
	cfg.SetThisDc(1)
	cfg.SetDcOptions([]*mtproto.DcOption{{
		PredicateName: mtproto.Predicate_dcOption,
		Constructor:   mtproto.TLConstructor_CRC32_dcOption,
		Id:            2,
		IpAddress:     "203.0.113.9",
		Port:          10443,
		ThisPortOnly:  true,
	}})
	Apply(cfg)
	opts := cfg.GetDcOptions()
	if len(opts) != 1 || opts[0].GetId() != 2 || opts[0].GetIpAddress() != "203.0.113.9" || opts[0].GetPort() != 10443 {
		t.Fatalf("%+v", opts)
	}
}

func TestFileDCIDZeroIsNotPublished(t *testing.T) {
	t.Setenv(envOptions, "")
	t.Setenv(envDomain, "")
	t.Setenv(envPublic, "8.137.198.240")
	cfg := fileShape()
	cfg.SetThisDc(2)
	cfg.SetDcOptions([]*mtproto.DcOption{
		{
			PredicateName: mtproto.Predicate_dcOption,
			Constructor:   mtproto.TLConstructor_CRC32_dcOption,
			Id:            0,
			IpAddress:     "8.156.40.105",
			Port:          10443,
			ThisPortOnly:  true,
		},
		{
			PredicateName: mtproto.Predicate_dcOption,
			Constructor:   mtproto.TLConstructor_CRC32_dcOption,
			Id:            2,
			IpAddress:     "8.156.40.105",
			Port:          443,
			ThisPortOnly:  true,
		},
	})
	Apply(cfg)
	for _, opt := range cfg.GetDcOptions() {
		if opt.GetId() <= 0 {
			t.Fatalf("published dc %d", opt.GetId())
		}
	}
	opts := cfg.GetDcOptions()
	if len(opts) != 1 || opts[0].GetId() != 2 || opts[0].GetIpAddress() != "8.156.40.105" {
		t.Fatalf("rewrite ran or DC2 dropped: %+v", opts)
	}
}

func TestCheckRejectsFileDCWithoutAddress(t *testing.T) {
	t.Setenv(envOptions, "not-a-spec")
	t.Setenv(envDomain, "")
	t.Setenv(envPublic, "8.137.198.240")
	cfg := fileShape()
	cfg.SetThisDc(2)
	cfg.SetDcOptions([]*mtproto.DcOption{{
		PredicateName: mtproto.Predicate_dcOption,
		Constructor:   mtproto.TLConstructor_CRC32_dcOption,
		Id:            2,
		IpAddress:     "8.156.40.105",
		Port:          10443,
		ThisPortOnly:  true,
	}})
	if FileDC() != 1 {
		t.Fatalf("file dc %d", FileDC())
	}
	if err := Check(cfg); err == nil {
		t.Fatal("file dc 1 has no address")
	}
}

func TestDroppedPortWarningLogsEveryCall(t *testing.T) {
	t.Setenv(envOptions, "1:8.156.40.105:5222,1:8.156.40.105:10443")
	t.Setenv(envDomain, "")
	t.Setenv(envPublic, "8.137.198.240")
	before := droppedPortLogCount.Load()
	Apply(fileShape())
	Apply(fileShape())
	if got := droppedPortLogCount.Load() - before; got != 2 {
		t.Fatalf("drop logs %d, want 2", got)
	}
}

func TestApplyPublicHostLeavesMappedIPv6(t *testing.T) {
	t.Setenv(envOptions, "")
	t.Setenv(envDomain, "")
	t.Setenv(envPublic, "8.137.198.240")
	mapped := &mtproto.DcOption{
		PredicateName: mtproto.Predicate_dcOption,
		Constructor:   mtproto.TLConstructor_CRC32_dcOption,
		Id:            1,
		IpAddress:     "::ffff:8.156.40.105",
		Port:          10443,
		Ipv6:          true,
		ThisPortOnly:  true,
	}
	mappedFlagOff := &mtproto.DcOption{
		PredicateName: mtproto.Predicate_dcOption,
		Constructor:   mtproto.TLConstructor_CRC32_dcOption,
		Id:            2,
		IpAddress:     "::ffff:10.20.0.218",
		Port:          443,
		Ipv6:          false,
		ThisPortOnly:  true,
	}
	flaggedV4 := &mtproto.DcOption{
		PredicateName: mtproto.Predicate_dcOption,
		Constructor:   mtproto.TLConstructor_CRC32_dcOption,
		Id:            2,
		IpAddress:     "8.156.40.105",
		Port:          10443,
		Ipv6:          true,
		ThisPortOnly:  true,
	}
	cfg := fileShape()
	cfg.SetDcOptions([]*mtproto.DcOption{mapped, mappedFlagOff, flaggedV4})
	Apply(cfg)
	if mapped.GetIpAddress() != "::ffff:8.156.40.105" {
		t.Fatal("Apply wrote into the caller's dcOption")
	}
	opts := cfg.GetDcOptions()
	if len(opts) != 3 {
		t.Fatalf("len %d", len(opts))
	}
	if opts[0].GetIpAddress() != "::ffff:8.156.40.105" || !opts[0].GetIpv6() || opts[0].GetPort() != 10443 {
		t.Fatalf("mapped %+v", opts[0])
	}
	if opts[1].GetIpAddress() != "::ffff:10.20.0.218" || opts[1].GetIpv6() || opts[1].GetId() != 2 || opts[1].GetPort() != 443 {
		t.Fatalf("mapped flag off %+v", opts[1])
	}
	if opts[2].GetIpAddress() != "8.156.40.105" || !opts[2].GetIpv6() || opts[2].GetPort() != 10443 {
		t.Fatalf("ipv6 flag %+v", opts[2])
	}
}

func TestApplyPublicHostInvalidLeavesList(t *testing.T) {
	for _, host := range []string{"api.tellomi.cn", "8.137.198.240:10443", "2001:db8::1", "::ffff:8.137.198.240", "08.137.198.240"} {
		t.Run(host, func(t *testing.T) {
			t.Setenv(envOptions, "")
			t.Setenv(envDomain, "")
			t.Setenv(envPublic, host)
			cfg := fileShape()
			Apply(cfg)
			got := cfg.GetDcOptions()
			if len(got) != 1 || got[0].GetIpAddress() != "203.0.113.9" || got[0].GetPort() != 9 {
				t.Fatalf("%+v", got)
			}
			if cfg.GetWebfileDcId() != 4 {
				t.Fatalf("webfile %d", cfg.GetWebfileDcId())
			}
		})
	}
}

func fileShape() *mtproto.TLConfig {
	return &mtproto.TLConfig{Data2: &mtproto.Config{
		PredicateName:   mtproto.Predicate_config,
		DcTxtDomainName: "apv2.stel.com",
		WebfileDcId:     4,
		ThisDc:          1,
		DcOptions: []*mtproto.DcOption{{
			PredicateName: mtproto.Predicate_dcOption,
			Id:            1,
			IpAddress:     "203.0.113.9",
			Port:          9,
		}},
	}}
}
