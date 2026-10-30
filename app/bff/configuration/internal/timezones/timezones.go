package timezones

import (
	"bufio"
	"hash/fnv"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// List is help.getTimezonesList. A matching hash returns
// help.timezonesListNotModified so the client keeps its picker.
func List(hash int32) *mtproto.Help_TimezonesList {
	list := cached()
	if hash != 0 && hash == list.GetHash() {
		return mtproto.MakeTLHelpTimezonesListNotModified(&mtproto.Help_TimezonesList{}).To_Help_TimezonesList()
	}
	return list
}

var (
	once sync.Once
	list *mtproto.Help_TimezonesList
)

func cached() *mtproto.Help_TimezonesList {
	once.Do(func() {
		list = build(loadZones())
	})
	return list
}

type zone struct {
	id, name string
}

func loadZones() []zone {
	if zones := readZoneTab("/usr/share/zoneinfo/zone.tab"); len(zones) > 0 {
		return zones
	}
	return fallbackZones()
}

func readZoneTab(path string) []zone {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []zone
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
		out = append(out, zone{id: id, name: name})
	}
	if err := sc.Err(); err != nil || len(out) == 0 {
		return nil
	}
	return out
}

func build(zones []zone) *mtproto.Help_TimezonesList {
	zones = append([]zone{{id: "UTC", name: "UTC"}}, zones...)
	sort.Slice(zones, func(i, j int) bool { return zones[i].id < zones[j].id })
	now := time.Now()
	out := make([]*mtproto.Timezone, 0, len(zones))
	var hashParts []string
	seen := map[string]bool{}
	for _, z := range zones {
		if z.id == "" || seen[z.id] {
			continue
		}
		loc, err := time.LoadLocation(z.id)
		if err != nil {
			continue
		}
		_, offset := now.In(loc).Zone()
		seen[z.id] = true
		name := z.name
		if name == "" {
			name = z.id
		}
		out = append(out, mtproto.MakeTLTimezone(&mtproto.Timezone{
			Id:        z.id,
			Name:      name,
			UtcOffset: int32(offset),
		}).To_Timezone())
		hashParts = append(hashParts, z.id, strconv.Itoa(offset))
	}
	if out == nil {
		out = []*mtproto.Timezone{}
	}
	return mtproto.MakeTLHelpTimezonesList(&mtproto.Help_TimezonesList{
		Timezones: out,
		Hash:      hash32(hashParts),
	}).To_Help_TimezonesList()
}

func hash32(parts []string) int32 {
	h := fnv.New32a()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	v := int32(h.Sum32() & 0x7fffffff)
	if v == 0 {
		return 1
	}
	return v
}

func fallbackZones() []zone {
	ids := []string{
		"UTC",
		"Etc/GMT",
		"Etc/GMT+12",
		"Pacific/Honolulu",
		"America/Anchorage",
		"America/Los_Angeles",
		"America/Denver",
		"America/Chicago",
		"America/New_York",
		"America/Sao_Paulo",
		"Atlantic/Reykjavik",
		"Europe/London",
		"Europe/Lisbon",
		"Europe/Paris",
		"Europe/Berlin",
		"Europe/Athens",
		"Europe/Moscow",
		"Africa/Cairo",
		"Africa/Johannesburg",
		"Asia/Dubai",
		"Asia/Karachi",
		"Asia/Kolkata",
		"Asia/Dhaka",
		"Asia/Bangkok",
		"Asia/Shanghai",
		"Asia/Tokyo",
		"Asia/Seoul",
		"Australia/Sydney",
		"Pacific/Auckland",
		"Pacific/Fiji",
	}
	out := make([]zone, len(ids))
	for i, id := range ids {
		out[i] = zone{id: id, name: strings.ReplaceAll(id, "_", " ")}
	}
	return out
}
