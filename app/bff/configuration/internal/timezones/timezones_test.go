package timezones

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestListNotModified(t *testing.T) {
	list := List(0)
	if list.GetPredicateName() != mtproto.Predicate_help_timezonesList {
		t.Fatalf("predicate %s", list.GetPredicateName())
	}
	if len(list.GetTimezones()) < 20 {
		t.Fatalf("timezones %d", len(list.GetTimezones()))
	}
	var sawUTC, sawNewYork bool
	for _, zone := range list.GetTimezones() {
		if zone.GetPredicateName() != mtproto.Predicate_timezone || zone.GetId() == "" || zone.GetName() == "" {
			t.Fatalf("zone %#v", zone)
		}
		if zone.GetId() == "UTC" || zone.GetId() == "Etc/UTC" {
			sawUTC = true
		}
		if zone.GetId() == "America/New_York" {
			sawNewYork = true
		}
	}
	if !sawUTC || !sawNewYork {
		t.Fatalf("utc=%v new york=%v", sawUTC, sawNewYork)
	}
	again := List(list.GetHash())
	if again.GetPredicateName() != mtproto.Predicate_help_timezonesListNotModified {
		t.Fatalf("hash %d -> %s", list.GetHash(), again.GetPredicateName())
	}
}
