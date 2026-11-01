package geoip

import (
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/lionsoul2014/ip2region/binding/golang/service"
	"github.com/oschwald/geoip2-golang"
	"github.com/zeromicro/go-zero/core/logx"
)

// Options selects the databases used for authorization.country and
// authorization.region. A missing file does not stop the process.
type Options struct {
	// V4Path and V6Path are ip2region xdb files. Empty paths search
	// /app/data/ip2region and data/ip2region.
	V4Path string
	V6Path string
	// MMDBPath is the DB-IP Lite (or GeoLite2) city database. Empty uses
	// MMDB, GEOIP_MMDB, TELLOMI_GEOIP_MMDB, then the -mmdb flag. The deploy
	// mount is /app/bin/GeoLite2-City.mmdb.
	MMDBPath string
}

// Resolver answers country and region for a client address. The zero value
// resolves nothing and never panics.
type Resolver struct {
	ip2  *service.Ip2Region
	mmdb *geoip2.Reader
}

// MaxMindPath is the city mmdb. MMDB, then GEOIP_MMDB, then
// TELLOMI_GEOIP_MMDB, then the -mmdb flag. The flag default is
// ./GeoLite2-City.mmdb, which is /app/bin/GeoLite2-City.mmdb when
// authsession and bff are started from /app/bin.
func MaxMindPath(flagPath string) string {
	if p := strings.TrimSpace(os.Getenv("MMDB")); p != "" {
		return p
	}
	if p := strings.TrimSpace(os.Getenv("GEOIP_MMDB")); p != "" {
		return p
	}
	if p := strings.TrimSpace(os.Getenv("TELLOMI_GEOIP_MMDB")); p != "" {
		return p
	}
	return flagPath
}

// Load reads IP2REGION_V4_XDB, IP2REGION_V6_XDB, and the city mmdb path.
// Both databases are optional. A missing file is logged and lookups still
// return.
func Load(mmdbFlag string) *Resolver {
	return Open(Options{
		V4Path:   os.Getenv("IP2REGION_V4_XDB"),
		V6Path:   os.Getenv("IP2REGION_V6_XDB"),
		MMDBPath: MaxMindPath(mmdbFlag),
	})
}

// Open loads ip2region into memory and opens the city mmdb when the file
// exists. Mainland China comes from ip2region. Other addresses come from
// the mmdb. Open never panics and never returns nil.
func Open(opt Options) *Resolver {
	r := &Resolver{
		ip2:  openIP2Region(opt),
		mmdb: openMaxMind(opt.MMDBPath),
	}
	if r.ip2 == nil && r.mmdb == nil {
		logx.Errorf("geoip loaded neither ip2region nor a city mmdb. account.getAuthorizations still succeeds; country falls back to the client IP. See docs/ip2region.md.")
	}
	return r
}

// Lookup returns authorization.country and authorization.region. An unknown
// address, a private range, or a missing database yields two empty strings.
// It does not fail the caller.
func (r *Resolver) Lookup(ip string) (country, region string) {
	if r == nil {
		return "", ""
	}
	defer func() {
		if rec := recover(); rec != nil {
			logx.Errorf("geoip lookup panic: %v", rec)
			country, region = "", ""
		}
	}()
	parsed := ParseClientIP(ip)
	if parsed == nil {
		return "", ""
	}
	ip2Raw := ""
	if r.ip2 != nil {
		raw, err := r.ip2.Search(parsed.String())
		if err != nil {
			logx.Errorf("geoip ip2region search %s: %v", parsed.String(), err)
		} else {
			ip2Raw = raw
		}
	}
	mmdbCountry, mmdbRegion := "", ""
	if r.mmdb != nil {
		mmdbCountry, mmdbRegion = lookupMaxMind(r.mmdb, parsed)
	}
	return ChoosePlace(ip2Raw, mmdbCountry, mmdbRegion)
}

func openIP2Region(opt Options) *service.Ip2Region {
	v4Path, v4OK := resolveXDB(opt.V4Path, "ip2region_v4.xdb", true)
	v6Path, v6OK := resolveXDB(opt.V6Path, "ip2region_v6.xdb", false)
	var v4Cfg, v6Cfg *service.Config
	if v4OK {
		cfg, err := service.NewV4Config(service.BufferCache, v4Path, 1)
		if err != nil {
			logx.Errorf("geoip ip2region v4 not loaded from %q: %v. account.getAuthorizations still succeeds.", v4Path, err)
		} else {
			v4Cfg = cfg
			logx.Infof("geoip ip2region v4 loaded into memory from %q (%d bytes)", v4Path, len(cfg.CBuffer()))
		}
	}
	if v6OK {
		cfg, err := service.NewV6Config(service.BufferCache, v6Path, 1)
		if err != nil {
			logx.Errorf("geoip ip2region v6 not loaded from %q: %v. IPv6 lookups use the city mmdb when it is loaded.", v6Path, err)
		} else {
			v6Cfg = cfg
			logx.Infof("geoip ip2region v6 loaded into memory from %q (%d bytes)", v6Path, len(cfg.CBuffer()))
		}
	}
	if v4Cfg == nil && v6Cfg == nil {
		return nil
	}
	searcher, err := service.NewIp2Region(v4Cfg, v6Cfg)
	if err != nil {
		logx.Errorf("geoip ip2region searcher not started: %v. account.getAuthorizations still succeeds.", err)
		return nil
	}
	return searcher
}

// resolveXDB returns a readable xdb path. required is the IPv4 file: its
// absence is an error log. IPv6 is optional and logs at info when the
// default locations are empty.
func resolveXDB(explicit, filename string, required bool) (string, bool) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		if fileExists(explicit) {
			return explicit, true
		}
		if required {
			logx.Infof("geoip ip2region %s not found at %q. Mainland China uses the city mmdb when that file is present. RPCs still succeed.", filename, explicit)
		} else {
			logx.Infof("geoip ip2region %s not found at %q. That IP version uses the city mmdb when it is loaded.", filename, explicit)
		}
		return "", false
	}
	candidates := []string{
		filepath.Join("/app/data/ip2region", filename),
		filepath.Join("data", "ip2region", filename),
		filename,
	}
	for _, path := range candidates {
		if fileExists(path) {
			return path, true
		}
	}
	if required {
		logx.Infof("geoip ip2region %s not found under /app/data/ip2region or data/ip2region. Mainland China uses the city mmdb when that file is present. RPCs still succeed.", filename)
	} else {
		logx.Infof("geoip ip2region %s is not bundled. IPv6 lookups use the city mmdb when it is loaded.", filename)
	}
	return "", false
}

func openMaxMind(path string) *geoip2.Reader {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "./GeoLite2-City.mmdb"
	}
	candidates := []string{path}
	if path == "./GeoLite2-City.mmdb" {
		candidates = append(candidates, "/app/bin/GeoLite2-City.mmdb")
	}
	var missing []string
	for _, candidate := range candidates {
		if !fileExists(candidate) {
			missing = append(missing, candidate)
			continue
		}
		reader, err := geoip2.Open(candidate)
		if err != nil {
			logx.Errorf("geoip city mmdb not loaded from %q: %v. account.getAuthorizations still succeeds.", candidate, err)
			continue
		}
		logx.Infof("geoip city mmdb loaded from %q", candidate)
		return reader
	}
	logx.Infof("geoip city mmdb not found (%s). Addresses outside mainland China use ip2region when that database is loaded. RPCs still succeed.", strings.Join(missing, ", "))
	return nil
}

func lookupMaxMind(reader *geoip2.Reader, ip net.IP) (string, string) {
	if reader == nil || ip == nil {
		return "", ""
	}
	record, err := reader.City(ip)
	if err != nil || record == nil {
		return "", ""
	}
	regionName := ""
	if len(record.Subdivisions) > 0 {
		regionName = record.Subdivisions[0].Names["en"]
		if regionName == "" {
			regionName = record.Subdivisions[0].IsoCode
		}
	}
	return FormatMaxMind(record.Country.Names["en"], record.Country.IsoCode, regionName, record.City.Names["en"])
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
