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
