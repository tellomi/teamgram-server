package tz

import (
	"testing"
	"time"
)

func TestZonesFromEmbeddedIndex(t *testing.T) {
	orig := zoneTabPaths
	zoneTabPaths = []string{"/nonexistent/zone.tab"}
	t.Cleanup(func() { zoneTabPaths = orig })
	t.Setenv("ZONEINFO", "")

	zones := Zones()
	if len(zones) < 100 {
		t.Fatalf("embedded zones %d", len(zones))
	}
	var saw bool
	for _, z := range zones {
		if z.ID == "Asia/Shanghai" {
			saw = true
			if z.Name == "" {
				t.Fatal("empty name")
			}
		}
	}
	if !saw {
		t.Fatal("Asia/Shanghai missing from embedded index")
	}
	loc, err := Load("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	_, offset := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC).In(loc).Zone()
	if offset != 8*3600 {
		t.Fatalf("shanghai offset %d", offset)
	}
	if _, err := Load("Not/AZone"); err == nil {
		t.Fatal("invalid zone accepted")
	}
}

func TestSystemZoneTabWhenInstalled(t *testing.T) {
	zones := Zones()
	if len(zones) < 20 {
		t.Fatalf("zones %d", len(zones))
	}
}
