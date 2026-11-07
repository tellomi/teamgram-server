package msgexpiry

import "testing"

func TestLedgerBoundariesDevicesAndIdempotentSweep(t *testing.T) {
	t.Setenv(EnvEnabled, "1")
	t.Setenv(EnvSweep, "1")
	led := NewLedger()
	now := int32(1_000)

	led.Register(Record{Kind: KindPrivate, Scope: 7, ID: 1, Date: 100, TTL: 10, FileID: 11, Users: []int64{7, 8}})
	led.Register(Record{Kind: KindChannel, Scope: 3, ID: 2, Date: 100, TTL: 10, FileID: 12, Users: []int64{7, 9}})
	led.Register(Record{Kind: KindSecret, Scope: 4, ID: 3, Date: 100, TTL: 10, FileID: 13, Users: []int64{7, 8}})
	led.Register(Record{Kind: KindViewOnce, Scope: 7, ID: 4, Date: now, TTL: 0, FileID: 14, Users: []int64{7, 8}})
	led.Register(Record{Kind: KindPrivate, Scope: 7, ID: 5, Date: now, TTL: 0, FileID: 15, Users: []int64{7}})

	if !led.Readable(KindPrivate, 7, 5, now) {
		t.Fatal("ttl 0 private was hidden")
	}
	if led.Readable(KindPrivate, 7, 1, now) || !led.BlocksDownload(11, now) {
		t.Fatal("expired private still readable or downloadable")
	}
	if led.Readable(KindChannel, 3, 2, now) || !led.BlocksDownload(12, now) {
		t.Fatal("expired channel still readable")
	}
	if led.Readable(KindSecret, 4, 3, now) || !led.BlocksDownload(13, now) {
		t.Fatal("expired secret ciphertext still available")
	}
	if !led.Readable(KindViewOnce, 7, 4, now) || led.BlocksDownload(14, now) {
		t.Fatal("unopened view-once was blocked")
	}
	led.Open(7, 4)
	if led.Readable(KindViewOnce, 7, 4, now) || !led.BlocksDownload(14, now) {
		t.Fatal("opened view-once can still be fetched")
	}
	// A channel sweep must not be required to hide the private file, and the
	// private record must not make the live private file 15 disappear.
	if led.BlocksDownload(15, now) {
		t.Fatal("live private file blocked")
	}

	first := led.Sweep(now)
	if len(first) != 4 {
		t.Fatalf("sweep deleted %d, want 4", len(first))
	}
	users := UsersOf(first)
	if len(users) != 3 {
		t.Fatalf("devices %v", users)
	}
	// Reconnect: the same ledger already deleted them. A second device
	// reading again, and a second sweep, do not revive or delete twice.
	if led.Readable(KindPrivate, 7, 1, now+5) || !led.BlocksDownload(11, now+5) {
		t.Fatal("reconnect revived an expired private message")
	}
	if again := led.Sweep(now + 5); len(again) != 0 {
		t.Fatalf("second sweep %+v", again)
	}
}

func TestSwitchOffLeavesEveryBoundaryReadable(t *testing.T) {
	t.Setenv(EnvEnabled, "")
	t.Setenv(EnvSweep, "")
	led := NewLedger()
	led.Register(Record{Kind: KindPrivate, Scope: 1, ID: 1, Date: 10, TTL: 1, FileID: 9, Users: []int64{1, 2}})
	led.Open(1, 1)
	if !led.Readable(KindPrivate, 1, 1, 100) || led.BlocksDownload(9, 100) {
		t.Fatal("switch off still blocked")
	}
	if got := led.Sweep(100); len(got) != 0 {
		t.Fatalf("sweep ran while off: %+v", got)
	}
}

func TestGuardDoesNotStartWhenSweepIsOff(t *testing.T) {
	t.Setenv(EnvSweep, "")
	Guard(NewLedger())
}
