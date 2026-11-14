package timezones

import (
	"hash/fnv"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/tz"
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
	src := tz.Zones()
	out := make([]zone, len(src))
	for i, z := range src {
		out[i] = zone{id: z.ID, name: z.Name}
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
		loc, err := tz.Load(z.id)
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
