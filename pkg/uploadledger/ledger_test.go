package uploadledger

import (
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRetryAndChangedBytesBothCount(t *testing.T) {
	l := New(Limits{Network: 200, Window: time.Hour})
	if err := l.ChargeNetwork(7, 40); err != nil {
		t.Fatal(err)
	}
	if err := l.ChargeNetwork(7, 40); err != nil {
		t.Fatal(err)
	}
	if err := l.ChargeNetwork(7, 50); err != nil {
		t.Fatal(err)
	}
	if l.Book(7).Network != 130 {
		t.Fatalf("%+v", l.Book(7))
	}
}

func TestReviewOldPartMustBeMeteredAfterWindow(t *testing.T) {
	l := New(Limits{Network: 50, Window: time.Hour})
	now := time.Unix(1_700_000_000, 0)
	l.now = func() time.Time { return now }
	if err := l.ChargeNetwork(7, 40); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if err := l.ChargeNetwork(7, 40); err != nil {
		t.Fatalf("old part was not metered again: %v book=%+v", err, l.Book(7))
	}
	if l.Book(7).Network != 40 {
		t.Fatalf("%+v", l.Book(7))
	}
}

func TestOverLimitDoesNotRecord(t *testing.T) {
	l := New(Limits{Network: 30, Window: time.Hour})
	if err := l.ChargeNetwork(7, 20); err != nil {
		t.Fatal(err)
	}
	err := l.ChargeNetwork(7, 20)
	if err == nil {
		t.Fatal("over limit stored")
	}
	if l.Book(7).Network != 20 {
		t.Fatalf("%+v", l.Book(7))
	}
	if WaitOf(err) < 1 {
		t.Fatal(err)
	}
}

func TestUploadWithoutSendDoesNotChargeEntity(t *testing.T) {
	l := New(Limits{Network: 100, Entity: 100, Hold: 100, Window: time.Hour})
	if err := l.ChargeNetwork(7, 40); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Reserve(7, 1, 40); err != nil {
		t.Fatal(err)
	}
	if b := l.Book(7); b.Network != 40 || b.Entity != 0 || b.Refs != 0 || b.Hold != 40 {
		t.Fatalf("%+v", b)
	}
}

func TestCancelRefundsOnlyThatHold(t *testing.T) {
	l := New(Limits{Network: 200, Entity: 100, Hold: 200, Window: time.Hour})
	if err := l.ChargeNetwork(7, 40); err != nil {
		t.Fatal(err)
	}
	if err := l.CommitNew(7, 10); err != nil {
		t.Fatal(err)
	}
	first, err := l.Reserve(7, 1, 40)
	if err != nil {
		t.Fatal(err)
	}
	second, err := l.Reserve(7, 1, 40)
	if err != nil {
		t.Fatal(err)
	}
	l.Cancel(first)
	b := l.Book(7)
	if b.Network != 40 || b.Entity != 10 || b.Hold != 40 {
		t.Fatalf("cancel touched stored bytes %+v", b)
	}
	l.Cancel(second)
	if l.Book(7).Hold != 0 {
		t.Fatalf("%+v", l.Book(7))
	}
}

func TestReviewFailureCannotRefundSuccessfulConcurrentDuplicate(t *testing.T) {
	l := New(Limits{Network: 100, Hold: 100, Window: time.Hour})
	if err := l.ChargeNetwork(7, 40); err != nil {
		t.Fatal(err)
	}
	okID, err := l.Reserve(7, 1, 40)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.ChargeNetwork(7, 40); err != nil {
		t.Fatal(err)
	}
	failID, err := l.Reserve(7, 1, 40)
	if err != nil {
		t.Fatal(err)
	}
	l.Cancel(failID)
	b := l.Book(7)
	if b.Network != 80 || b.Hold != 40 {
		t.Fatalf("failure refunded the successful receive %+v", b)
	}
	if okID == failID {
		t.Fatal("hold ids collided")
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
		go func() {
			defer wg.Done()
			_ = l.ChargeNetwork(7, 100)
		}()
	}
	wg.Wait()
	if b := l.Book(7); b.Network != 1000 {
		t.Fatalf("%+v", b)
	}
}

func TestIndependentFinishedClaimReleasesHold(t *testing.T) {
	l := New(Limits{Network: 100, Entity: 80, Hold: 40, Refs: 10, Window: time.Hour})
	Use(l)
	t.Cleanup(func() { Use(nil) })
	if _, err := l.Reserve(7, 1, 40); err != nil {
		t.Fatal(err)
	}
	if err := NoteClaim(7, 1, false, 40); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Reserve(7, 1, 1); err != nil {
		t.Fatalf("finished upload kept the hold: %v %+v", err, l.Book(7))
	}
	if err := NoteClaim(7, 1, false, 80); err == nil {
		t.Fatal("entity cap was ignored")
	}
}

func TestIndependentRollingWindow(t *testing.T) {
	l := New(Limits{Network: 10, Window: time.Hour})
	now := time.Unix(1_700_000_000, 0)
	l.now = func() time.Time { return now }
	if err := l.ChargeNetwork(7, 1); err != nil {
		t.Fatal(err)
	}
	now = now.Add(59 * time.Minute)
	if err := l.ChargeNetwork(7, 8); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if err := l.ChargeNetwork(7, 8); err == nil {
		t.Fatalf("rolling hour allowed %+v", l.Book(7))
	}
}

func TestIndependentRefSubjectsAreCapped(t *testing.T) {
	l := New(Limits{Refs: 1 << 20, Window: time.Hour})
	for i := int64(1); i <= 8212; i++ {
		if err := l.CommitReuse(i, 0); err != nil {
			t.Fatal(err)
		}
	}
	if n := l.Users(); n > maxUsers+1 {
		t.Fatalf("ref-only subjects %d", n)
	}
}

func TestIndependentMeterEventsStayBounded(t *testing.T) {
	l := New(Limits{Network: 1 << 20, Window: time.Hour})
	for i := 0; i < 200; i++ {
		if err := l.ChargeNetwork(7, 1); err != nil {
			t.Fatal(err)
		}
	}
	if n := l.netEvents(7); n > maxMeterEvents {
		t.Fatalf("events %d", n)
	}
	if l.Book(7).Network != 200 {
		t.Fatalf("%+v", l.Book(7))
	}
}

func (l *Ledger) netEvents(user int64) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if c := l.net[user]; c != nil {
		return len(c.events)
	}
	return 0
}

func TestIndependentReleaseFileDoesNotClearAnother(t *testing.T) {
	l := New(Limits{Hold: 200, Entity: 200, Window: time.Hour})
	Use(l)
	t.Cleanup(func() { Use(nil) })
	if _, err := l.Reserve(7, 1, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Reserve(7, 2, 1); err != nil {
		t.Fatal(err)
	}
	if err := NoteClaim(7, 2, false, 1); err != nil {
		t.Fatal(err)
	}
	if got := l.Book(7).Hold; got != 100 {
		t.Fatalf("other file hold %d", got)
	}
}

func TestRedisCancelDoesNotClearTheNewHold(t *testing.T) {
	mr := miniredis.RunT(t)
	l, err := openRedis(mr.Addr(), Limits{Hold: 500, Window: time.Hour}, ModeEnforce)
	if err != nil {
		t.Fatal(err)
	}
	oldID, err := l.Reserve(7, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	newID, err := l.Reserve(7, 2, 40)
	if err != nil {
		t.Fatal(err)
	}
	l.Cancel(oldID)
	if got := l.Book(7).Hold; got != 40 {
		t.Fatalf("hold after canceling %d is %d, new id %d", oldID, got, newID)
	}
}

func TestRedisReserveMappingFailureIsClosed(t *testing.T) {
	mr := miniredis.RunT(t)
	l, err := openRedis(mr.Addr(), Limits{Hold: 500, Window: time.Hour}, ModeEnforce)
	if err != nil {
		t.Fatal(err)
	}
	mr.Close()
	if _, err = l.Reserve(7, 1, 20); err == nil {
		t.Fatal("reserve succeeded without a hold id mapping")
	}
}

func TestRedisWindowIsSharedByTwoProcesses(t *testing.T) {
	mr := miniredis.RunT(t)
	a, err := openRedis(mr.Addr(), Limits{Network: 100, Window: time.Hour}, ModeEnforce)
	if err != nil {
		t.Fatal(err)
	}
	b, err := openRedis(mr.Addr(), Limits{Network: 100, Window: time.Hour}, ModeEnforce)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.ChargeNetwork(7, 60); err != nil {
		t.Fatal(err)
	}
	if err = b.ChargeNetwork(7, 50); err == nil {
		t.Fatal("second process did not see the first charge")
	}
	if a.Book(7).Network != 60 || b.Book(7).Network != 60 {
		t.Fatalf("a %+v b %+v", a.Book(7), b.Book(7))
	}
	mr.FastForward(2 * time.Hour)
	if err = b.ChargeNetwork(7, 40); err != nil {
		t.Fatalf("after the window: %v", err)
	}
}
