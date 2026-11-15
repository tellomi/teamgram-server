// Package tz is the IANA timezone source for help.getTimezonesList and
// account.updateBusinessWorkHours. Both call Load, which uses the system
// zoneinfo when it is installed and the database embedded by time/tzdata
// when the runtime image has none.
package tz

import (
	"bufio"
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"
)

//go:embed zones.txt
var zoneIndex string

// Zone is one IANA time zone. Name is the zone.tab comment when that file
// is installed, otherwise the id with underscores written as spaces.
type Zone struct {
	ID   string
	Name string
}

// zoneTabPaths follows the directories time.LoadLocation searches, so the
// picker and Load read one installation.
var zoneTabPaths = []string{
	"/usr/share/zoneinfo/zone.tab",
	"/usr/share/lib/zoneinfo/zone.tab",
	"/usr/lib/locale/TZ/zone.tab",
	"/etc/zoneinfo/zone.tab",
}

// Load resolves an IANA id. "" and "UTC" succeed without zoneinfo files.
func Load(id string) (*time.Location, error) {
	return time.LoadLocation(id)
}

// Zones lists IANA zones. A system zone.tab wins because Load reads that
// same tree. Without it, the list is the file index of the embedded database.
func Zones() []Zone {
	paths := zoneTabPaths
	if env := strings.TrimSpace(os.Getenv("ZONEINFO")); env != "" && !strings.HasSuffix(env, ".zip") {
		paths = append([]string{filepath.Join(env, "zone.tab")}, paths...)
	}
	for _, path := range paths {
		if zones := readZoneTab(path); len(zones) > 0 {
			return zones
		}
	}
	return embeddedZones()
}

func embeddedZones() []Zone {
	var out []Zone
	sc := bufio.NewScanner(strings.NewReader(zoneIndex))
	for sc.Scan() {
		id := strings.TrimSpace(sc.Text())
		if id == "" || strings.HasPrefix(id, "#") || strings.Contains(id, "..") {
			continue
		}
		out = append(out, Zone{ID: id, Name: strings.ReplaceAll(id, "_", " ")})
	}
	return out
}

func readZoneTab(path string) []Zone {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []Zone
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		id := fields[2]
		name := id
		if len(fields) >= 4 && strings.TrimSpace(fields[3]) != "" {
			name = strings.TrimSpace(fields[3])
		}
		out = append(out, Zone{ID: id, Name: name})
	}
	if err := sc.Err(); err != nil || len(out) == 0 {
		return nil
	}
	return out
}
