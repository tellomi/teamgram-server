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
	cfg.SetThisDc(7)
	cfg.SetDcOptions([]*mtproto.DcOption{origin, cdnOpt, domainOpt})
	Apply(cfg)
	if origin.GetIpAddress() != "8.156.40.105" || cdnOpt.GetIpAddress() != "10.20.0.218" {
		t.Fatal("Apply wrote into the caller's dcOption")
	}
	opts := cfg.GetDcOptions()
	if len(opts) != 3 {
		t.Fatalf("len %d", len(opts))
	}
	if opts[0].GetIpAddress() != "8.137.198.240" || opts[0].GetPort() != 10443 || !opts[0].GetMediaOnly() || opts[0].GetCdn() {
		t.Fatalf("media %+v", opts[0])
	}
	if opts[1].GetIpAddress() != "8.137.198.240" || opts[1].GetPort() != 443 || !opts[1].GetCdn() || opts[1].GetId() != 2 || opts[1].GetMediaOnly() {
		t.Fatalf("cdn %+v", opts[1])
	}
	if opts[2].GetIpAddress() != "api.tellomi.cn" || opts[2].GetPort() != 10443 {
		t.Fatalf("domain %+v", opts[2])
	}
	if cfg.GetDcTxtDomainName() != "apv2.stel.com" || cfg.GetThisDc() != 7 || cfg.GetWebfileDcId() != 1 {
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

func TestApplyPublicHostInvalidLeavesList(t *testing.T) {
	for _, host := range []string{"api.tellomi.cn", "8.137.198.240:10443", "2001:db8::1", "08.137.198.240"} {
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
		DcOptions: []*mtproto.DcOption{{
			PredicateName: mtproto.Predicate_dcOption,
			Id:            1,
			IpAddress:     "203.0.113.9",
			Port:          9,
		}},
	}}
}
