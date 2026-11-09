// Copyright 2026 Tellomi Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0

// Package dcoptions fills help.getConfig dc_options from the environment.
// teamgramd/bin/config.json stays an empty list. Unset means that empty list.
package dcoptions

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	envOptions = "TELLOMI_DC_OPTIONS"
	envDomain  = "TELLOMI_DC_TXT_DOMAIN"

	// domainBlank is the only value that clears dc_txt_domain_name.
	// Compose always passes the variable through, so an empty value has to
	// mean "leave the file alone" or a deploy with the variable unset would
	// blank apv2.stel.com.
	domainBlank = "-"

	maxSpecLen = 2048
	maxEntries = 32
	maxDCID    = 65535
)

// Apply edits cfg from TELLOMI_DC_OPTIONS and TELLOMI_DC_TXT_DOMAIN.
// A valid options spec replaces dc_options and sets webfile_dc_id to 1.
// The checked-in file also ships 1 (DC 4 does not exist). An empty options
// variable leaves both fields as they were. An invalid spec is logged
// once and also leaves them as they were. The process keeps running.
func Apply(cfg *mtproto.TLConfig) {
	if cfg == nil || cfg.GetData2() == nil {
		return
	}
	if spec := strings.TrimSpace(os.Getenv(envOptions)); spec != "" {
		opts, err := Parse(spec)
		if err != nil {
			logInvalid(spec, err)
		} else {
			cfg.SetDcOptions(opts)
			cfg.SetWebfileDcId(1)
		}
	}
	applyDomain(cfg)
}

// Parse checks a compact list: dcId:ipv4:port,dcId:ipv4:port.
// IPv4 literals only. Hostnames and IPv6 are rejected.
func Parse(spec string) ([]*mtproto.DcOption, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty")
	}
	if len(spec) > maxSpecLen {
		return nil, fmt.Errorf("longer than %d bytes", maxSpecLen)
	}
	parts := strings.Split(spec, ",")
	if len(parts) > maxEntries {
		return nil, fmt.Errorf("%d entries exceeds %d", len(parts), maxEntries)
	}
	out := make([]*mtproto.DcOption, 0, len(parts))
	for _, part := range parts {
		opt, err := parseEntry(part)
		if err != nil {
			return nil, err
		}
		out = append(out, opt)
	}
	return out, nil
}

func parseEntry(part string) (*mtproto.DcOption, error) {
	part = strings.TrimSpace(part)
	if part == "" {
		return nil, fmt.Errorf("empty entry")
	}
	fields := strings.Split(part, ":")
	if len(fields) != 3 {
		return nil, fmt.Errorf("%q: want dc:ipv4:port", part)
	}
	id, err := parseCanonicalInt(fields[0], 1, maxDCID)
	if err != nil {
		return nil, fmt.Errorf("%q: dc id: %w", part, err)
	}
	ip := fields[1]
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil || parsed.To4().String() != ip {
		return nil, fmt.Errorf("%q: ipv4 literal required, got %q", part, ip)
	}
	port, err := parseCanonicalInt(fields[2], 1, 65535)
	if err != nil {
		return nil, fmt.Errorf("%q: port: %w", part, err)
	}
	// this_port_only makes Android dial the declared port. 10443 is not
	// one of the ports clients substitute. iOS ignores the flag and still
	// probes 80 and 5222. media_only would drop the address from the main
	// connection. tcpo_only is not required for gnetway. static stays false.
	return &mtproto.DcOption{
		PredicateName: mtproto.Predicate_dcOption,
		Constructor:   mtproto.TLConstructor_CRC32_dcOption,
		Id:            int32(id),
		IpAddress:     ip,
		Port:          int32(port),
		ThisPortOnly:  true,
	}, nil
}

func parseCanonicalInt(raw string, min, max int) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil || raw != strconv.Itoa(n) {
		return 0, fmt.Errorf("%q is not a canonical integer", raw)
	}
	if n < min || n > max {
		return 0, fmt.Errorf("%d is outside %d-%d", n, min, max)
	}
	return n, nil
}

// FileDC is the datacenter id clients should use when downloading a file
// this process stored. It is the first id in TELLOMI_DC_OPTIONS, or 1 when
// that variable is unset or invalid. Documents must not hardcode a different id.
func FileDC() int32 {
	spec := strings.TrimSpace(os.Getenv(envOptions))
	if spec == "" {
		return 1
	}
	opts, err := Parse(spec)
	if err != nil || len(opts) == 0 || opts[0].GetId() == 0 {
		return 1
	}
	return opts[0].GetId()
}

func applyDomain(cfg *mtproto.TLConfig) {
	raw, ok := os.LookupEnv(envDomain)
	if !ok {
		return
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	if raw == domainBlank {
		cfg.SetDcTxtDomainName("")
		return
	}
	cfg.SetDcTxtDomainName(raw)
}

var invalidLog struct {
	sync.Mutex
	spec string
}

func logInvalid(spec string, err error) {
	invalidLog.Lock()
	defer invalidLog.Unlock()
	if spec == invalidLog.spec {
		return
	}
	invalidLog.spec = spec
	shown := spec
	if len(shown) > 200 {
		shown = shown[:200] + "..."
	}
	logx.Errorf("help.getConfig: TELLOMI_DC_OPTIONS %q is invalid (%v); leaving dc_options unchanged", shown, err)
}
