package readtime

import (
	"strings"
	"testing"
	"time"
)

func TestPickRejectsEpoch(t *testing.T) {
	if Pick(0) != 0 {
		t.Fatal("epoch must not be reported as a read time")
	}
	now := time.Now().Unix()
	if got := Pick(Combine(0, now)); got == 0 {
		t.Fatal("getOutboxReadDate timestamp is 0")
	} else if int64(got) != now {
		t.Fatalf("fallback date = %d, want %d", got, now)
	}
	stored := now - 30
	if got := Pick(Combine(stored, now)); int64(got) != stored {
		t.Fatalf("stored date = %d, want %d", got, stored)
	}
}

func TestStampNeverZero(t *testing.T) {
	if Stamp(0) == 0 {
		t.Fatal("stamp persisted 0")
	}
	now := time.Now().Unix()
	if Stamp(now) != now {
		t.Fatalf("stamp(%d) changed a real time", now)
	}
	if Stamp(now*1000) != now {
		t.Fatalf("stamp did not collapse milliseconds")
	}
}

func TestUpsertKeepsFirstPositiveDate(t *testing.T) {
	if PositionalUpsert() == "" || NamedUpsert() == "" {
		t.Fatal("empty upsert")
	}
	for _, q := range []string{PositionalUpsert(), NamedUpsert(), UpsertTail} {
		if !strings.Contains(q, "read_outbox_max_date > 0") {
			t.Fatalf("upsert can overwrite a real read time or keep a zero: %s", q)
		}
	}
}
