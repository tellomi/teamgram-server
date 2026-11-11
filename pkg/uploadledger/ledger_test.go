package uploadledger

import (
	"sync"
	"testing"
	"time"
)

func TestSamePartRetryIsIdempotent(t *testing.T) {
	l := New(Limits{Network: 100, Window: time.Hour})
	fresh, err := l.ChargePart(7, 9, 0, 40)
	if err != nil || !fresh {
		t.Fatal(err, fresh)
	}
	fresh, err = l.ChargePart(7, 9, 0, 40)
	if err != nil || fresh {
		t.Fatal(err, fresh)
	}
	if l.Book(7).Network != 40 {
		t.Fatalf("%+v", l.Book(7))
	}
}

func TestChangedBytesChargeAgain(t *testing.T) {
	l := New(Limits{Network: 100, Window: time.Hour})
	if _, err := l.ChargePart(7, 9, 0, 40); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ChargePart(7, 9, 0, 50); err != nil {
		t.Fatal(err)
	}
	if l.Book(7).Network != 90 {
		t.Fatalf("%+v", l.Book(7))
	}
}

func TestOverLimitDoesNotRecord(t *testing.T) {
	l := New(Limits{Network: 30, Window: time.Hour})
	if _, err := l.ChargePart(7, 1, 0, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ChargePart(7, 1, 1, 20); err != ErrLimited {
		t.Fatalf("%v", err)
	}
	if l.Book(7).Network != 20 {
		t.Fatalf("%+v", l.Book(7))
	}
}

func TestUploadWithoutSendDoesNotChargeEntity(t *testing.T) {
	l := New(Limits{Network: 100, Entity: 100, Hold: 100, Window: time.Hour})
	if _, err := l.ChargePart(7, 1, 0, 40); err != nil {
		t.Fatal(err)
	}
	if b := l.Book(7); b.Network != 40 || b.Entity != 0 || b.Refs != 0 {
		t.Fatalf("%+v", b)
	}
}

func TestCancelRefundsHoldOnly(t *testing.T) {
	l := New(Limits{Network: 100, Entity: 100, Hold: 100, Window: time.Hour})
	if _, err := l.ChargePart(7, 1, 0, 40); err != nil {
		t.Fatal(err)
	}
	if err := l.Reserve(7, 40); err != nil {
		t.Fatal(err)
	}
	if err := l.CommitNew(7, 10); err != nil {
		t.Fatal(err)
	}
	l.Cancel(7, 40)
	b := l.Book(7)
	if b.Network != 40 || b.Entity != 10 || b.Hold != 0 {
		t.Fatalf("cancel refunded stored bytes %+v", b)
	}
	if err := l.Reserve(7, 40); err != nil {
		t.Fatal(err)
	}
}

func TestReuseDoesNotChargeVictimOrEntity(t *testing.T) {
	l := New(Limits{Entity: 100, Refs: 10, Window: time.Hour})
	if err := l.CommitNew(7, 50); err != nil {
		t.Fatal(err)
	}
	if err := l.CommitReuse(7, 99); err != nil {
		t.Fatal(err)
	}
	if b := l.Book(99); b.Entity != 0 || b.Refs != 0 || b.Network != 0 {
		t.Fatalf("victim charged %+v", b)
	}
	if b := l.Book(7); b.Entity != 50 || b.Refs != 1 {
		t.Fatalf("uploader %+v", b)
	}
}

func TestConcurrentDevicesStayInsideCap(t *testing.T) {
	l := New(Limits{Network: 1000, Window: time.Hour})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(part int) {
			defer wg.Done()
			_, _ = l.ChargePart(7, 3, part, 100)
		}(i)
	}
	wg.Wait()
	if b := l.Book(7); b.Network != 1000 {
		t.Fatalf("%+v", b)
	}
}

func TestWindowDropsOldCharges(t *testing.T) {
	l := New(Limits{Network: 50, Window: time.Hour})
	now := time.Unix(1_700_000_000, 0)
	l.now = func() time.Time { return now }
	if _, err := l.ChargePart(7, 1, 0, 40); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if _, err := l.ChargePart(7, 1, 1, 40); err != nil {
		t.Fatal(err)
	}
	if l.Book(7).Network != 40 {
		t.Fatalf("%+v", l.Book(7))
	}
}

func TestFailedStoreRefundLetsRetryThrough(t *testing.T) {
	l := New(Limits{Network: 40, Window: time.Hour})
	fresh, err := l.ChargePart(7, 1, 0, 40)
	if err != nil || !fresh {
		t.Fatal(err, fresh)
	}
	l.RefundPart(7, 1, 0, 40)
	fresh, err = l.ChargePart(7, 1, 0, 40)
	if err != nil || !fresh {
		t.Fatalf("retry after refund %v fresh=%v book=%+v", err, fresh, l.Book(7))
	}
}
