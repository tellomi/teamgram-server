package book

import (
	"encoding/binary"
	"testing"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const terms = "https://example.com/sms-jobs-terms"

func codeOf(err error) (codes.Code, string) {
	if err == nil {
		return codes.OK, ""
	}
	st := status.Convert(err)
	return st.Code(), st.Message()
}

func errOnly(_ any, err error) error { return err }

func ctor(t *testing.T, obj interface {
	Encode(*mtproto.EncodeBuf, int32) error
}) uint32 {
	t.Helper()
	buf := mtproto.NewEncodeBuf(512)
	if err := obj.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if len(raw) < 4 {
		t.Fatalf("short encode %d", len(raw))
	}
	return binary.LittleEndian.Uint32(raw)
}

func TestClosedProgram(t *testing.T) {
	b := New()
	if c, msg := codeOf(errOnly(b.Eligible(0))); c != mtproto.ErrUnauthorized || msg != "AUTH_KEY_UNREGISTERED" {
		t.Fatalf("auth %d %s", c, msg)
	}
	if c, msg := codeOf(errOnly(b.Eligible(4))); c != mtproto.ErrForbidden || msg != "NOT_ELIGIBLE" {
		t.Fatalf("eligible %d %s", c, msg)
	}
	if c, msg := codeOf(errOnly(b.Join(4))); c != mtproto.ErrBadRequest || msg != "NOT_ELIGIBLE" {
		t.Fatalf("join %d %s", c, msg)
	}
	for _, call := range []struct {
		name string
		err  error
	}{
		{"leave", func() error { _, err := b.Leave(4); return err }()},
		{"settings", func() error { _, err := b.UpdateSettings(4, true); return err }()},
		{"status", func() error { _, err := b.Status(4); return err }()},
	} {
		if c, msg := codeOf(call.err); c != mtproto.ErrBadRequest || msg != "NOT_JOINED" {
			t.Fatalf("%s %d %s", call.name, c, msg)
		}
	}
	if c, msg := codeOf(func() error { _, err := b.Job(4, ""); return err }()); c != mtproto.ErrBadRequest || msg != "SMSJOB_ID_INVALID" {
		t.Fatalf("empty job %d %s", c, msg)
	}
	if c, msg := codeOf(func() error { _, err := b.Job(4, "job-1"); return err }()); c != mtproto.ErrBadRequest || msg != "SMSJOB_ID_INVALID" {
		t.Fatalf("missing job %d %s", c, msg)
	}
	if c, msg := codeOf(func() error { _, err := b.Finish(4, "job-1", true); return err }()); c != mtproto.ErrBadRequest || msg != "SMSJOB_ID_INVALID" {
		t.Fatalf("finish %d %s", c, msg)
	}
	if err := b.assign(4, "job-1", "+15551212", "code"); status.Convert(err).Message() != "SMSJOB_ID_INVALID" {
		t.Fatal(err)
	}
}

func TestLedger(t *testing.T) {
	b := New()
	b.offer(terms, 3)
	elig, err := b.Eligible(7)
	if err != nil {
		t.Fatal(err)
	}
	if elig.GetPredicateName() != mtproto.Predicate_smsjobs_eligibleToJoin || elig.GetTermsUrl() != terms || elig.GetMonthlySentSms() != 3 {
		t.Fatalf("eligibility %+v", elig)
	}
	if got := ctor(t, elig); got != 0xdc8b44cf {
		t.Fatalf("eligibleToJoin ctor %x", got)
	}
	if _, err := b.Join(7); err != nil {
		t.Fatal(err)
	}
	if _, msg := codeOf(errOnly(b.Eligible(7))); msg != "NOT_ELIGIBLE" {
		t.Fatalf("joined eligible %s", msg)
	}
	if _, err := b.Join(7); err != nil {
		t.Fatal("second join")
	}
	if _, msg := codeOf(errOnly(b.UpdateSettings(8, true))); msg != "NOT_JOINED" {
		t.Fatal(msg)
	}
	if _, err := b.UpdateSettings(7, true); err != nil {
		t.Fatal(err)
	}
	st, err := b.Status(7)
	if err != nil {
		t.Fatal(err)
	}
	if st.GetPredicateName() != mtproto.Predicate_smsjobs_status || !st.GetAllowInternational() || st.GetRecentRemains() != 3 || st.GetRecentSent() != 0 || st.GetTotalSent() != 0 || st.GetTermsUrl() != terms || st.GetRecentSince() == 0 || st.GetTotalSince() != st.GetRecentSince() {
		t.Fatalf("status %+v", st)
	}
	if got := ctor(t, st); got != 0x2aee9191 {
		t.Fatalf("status ctor %x", got)
	}
	b.mu.Lock()
	b.members[7].lastGift = "star-gift"
	b.mu.Unlock()
	st, err = b.Status(7)
	if err != nil || st.GetLastGiftSlug().GetValue() != "star-gift" {
		t.Fatalf("gift %+v %v", st, err)
	}
	if got := ctor(t, st); got != 0x2aee9191 {
		t.Fatalf("status ctor %x", got)
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := st.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	flags := binary.LittleEndian.Uint32(buf.GetBuf()[4:8])
	if flags != 1<<0|1<<1 {
		t.Fatalf("flags %x", flags)
	}
	raw := append([]byte(nil), buf.GetBuf()...)
	decoded := &mtproto.Smsjobs_Status{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if !decoded.GetAllowInternational() || decoded.GetLastGiftSlug().GetValue() != "star-gift" || decoded.GetTermsUrl() != terms || decoded.GetRecentRemains() != 3 {
		t.Fatalf("round trip %+v", decoded)
	}

	if _, msg := codeOf(errOnly(b.Job(7, ""))); msg != "SMSJOB_ID_INVALID" {
		t.Fatal(msg)
	}
	if err := b.assign(7, "job-1", "15551212", "login code"); err != nil {
		t.Fatal(err)
	}
	if err := b.assign(9, "job-1", "1999", "nope"); status.Convert(err).Message() != "NOT_JOINED" {
		t.Fatal(err)
	}
	if _, err := b.Join(9); err != nil {
		t.Fatal(err)
	}
	if err := b.assign(9, "job-1", "1999", "nope"); status.Convert(err).Message() != "SMSJOB_ID_INVALID" {
		t.Fatal(err)
	}
	got, err := b.Job(7, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_smsJob || got.GetJobId() != "job-1" || got.GetPhoneNumber() != "15551212" || got.GetText() != "login code" {
		t.Fatalf("job %+v", got)
	}
	if c := ctor(t, got); c != 0xe6a1eeb8 {
		t.Fatalf("smsJob ctor %x", c)
	}
	if _, msg := codeOf(errOnly(b.Job(9, "job-1"))); msg != "SMSJOB_ID_INVALID" {
		t.Fatalf("other owner %s", msg)
	}
	if _, err := b.Finish(7, "job-1", true); err != nil {
		t.Fatal(err)
	}
	st, err = b.Status(7)
	if err != nil || st.GetRecentSent() != 0 || st.GetTotalSent() != 0 || st.GetRecentRemains() != 3 {
		t.Fatalf("failed finish %+v %v", st, err)
	}
	if _, msg := codeOf(errOnly(b.Finish(7, "job-1", false))); msg != "SMSJOB_ID_INVALID" {
		t.Fatal(msg)
	}
	if err := b.assign(7, "job-2", "15550000", "second"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Finish(7, "job-2", false); err != nil {
		t.Fatal(err)
	}
	st, err = b.Status(7)
	if err != nil || st.GetRecentSent() != 1 || st.GetTotalSent() != 1 || st.GetRecentRemains() != 2 {
		t.Fatalf("sent %+v %v", st, err)
	}
	if _, msg := codeOf(errOnly(b.Job(7, "job-2"))); msg != "SMSJOB_ID_INVALID" {
		t.Fatal(msg)
	}
	if _, err := b.Leave(7); err != nil {
		t.Fatal(err)
	}
	if _, msg := codeOf(errOnly(b.Status(7))); msg != "NOT_JOINED" {
		t.Fatal(msg)
	}
	if _, msg := codeOf(errOnly(b.Leave(7))); msg != "NOT_JOINED" {
		t.Fatal(msg)
	}
	elig, err = b.Eligible(7)
	if err != nil || elig.GetMonthlySentSms() != 3 {
		t.Fatalf("rejoin eligibility %+v %v", elig, err)
	}
}
