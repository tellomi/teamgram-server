package invite

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestExportValidate(t *testing.T) {
	now := time.Now().Unix()
	if err := (Export{Legacy: true, Now: now}).Validate(); err != nil {
		t.Fatal(err)
	}
	err := (Export{Legacy: true, RequestNeeded: true, Title: "周末", TitleSet: true, Now: now}).Validate()
	if !errors.Is(err, mtproto.ErrChatInvitePermanent) && (err == nil || !strings.Contains(err.Error(), "CHAT_INVITE_PERMANENT")) {
		t.Fatalf("legacy with options: %v", err)
	}
	if err := (Export{
		Title: "周末局", TitleSet: true, RequestNeeded: true,
		Expire: now + 3600, Usage: 10, UsageSet: true, Now: now,
	}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Export{Expire: now - 1, Now: now}).Validate(); err == nil {
		t.Fatal("past expiry accepted")
	}
	zero := Export{Usage: 0, UsageSet: true, Now: now}
	if err := zero.Validate(); err != nil {
		t.Fatalf("usage_limit 0 is unlimited: %v", err)
	}
	if zero.StoredUsage() != 0 {
		t.Fatal("usage_limit 0 must be stored as unset")
	}
	if err := (Export{Legacy: true, Usage: 0, UsageSet: true, Now: now}).Validate(); err == nil || !strings.Contains(err.Error(), "CHAT_INVITE_PERMANENT") {
		t.Fatalf("legacy with usage 0: %v", err)
	}
	if err := (Export{Usage: -1, UsageSet: true, Now: now}).Validate(); err == nil || !strings.Contains(err.Error(), "USAGE_LIMIT_INVALID") {
		t.Fatalf("negative usage: %v", err)
	}
	if err := (Export{Usage: UsageMax, UsageSet: true, Now: now}).Validate(); err != nil {
		t.Fatalf("usage %d: %v", UsageMax, err)
	}
	if got := (Export{Usage: UsageMax, UsageSet: true}).StoredUsage(); got != UsageMax {
		t.Fatalf("stored max %d", got)
	}
	if err := (Export{Usage: UsageMax + 1, UsageSet: true, Now: now}).Validate(); err == nil || !strings.Contains(err.Error(), "USAGE_LIMIT_INVALID") {
		t.Fatalf("usage above %d: %v", UsageMax, err)
	}
	absent := Export{Usage: 12, Now: now}
	if err := absent.Validate(); err != nil || absent.StoredUsage() != 0 {
		t.Fatalf("absent flag: stored %d err %v", absent.StoredUsage(), err)
	}
	if err := (Export{Title: strings.Repeat("名", TitleMax+1), TitleSet: true, Now: now}).Validate(); err == nil {
		t.Fatal("long title accepted")
	}
}

func TestNormalizeUsage(t *testing.T) {
	got, err := NormalizeUsage(true, 0)
	if err != nil || got != 0 {
		t.Fatalf("flagged 0: %d %v", got, err)
	}
	got, err = NormalizeUsage(false, -5)
	if err != nil || got != 0 {
		t.Fatalf("absent: %d %v", got, err)
	}
	if _, err := NormalizeUsage(true, -1); err == nil || !strings.Contains(err.Error(), "USAGE_LIMIT_INVALID") {
		t.Fatalf("negative: %v", err)
	}
	if _, err := NormalizeUsage(true, UsageMax+1); err == nil || !strings.Contains(err.Error(), "USAGE_LIMIT_INVALID") {
		t.Fatalf("over max: %v", err)
	}
	got, err = NormalizeUsage(true, UsageMax)
	if err != nil || got != UsageMax {
		t.Fatalf("max: %d %v", got, err)
	}
}

func TestUsageExhausted(t *testing.T) {
	if UsageExhausted(0, 0) || UsageExhausted(0, 100) || UsageExhausted(-1, 1) {
		t.Fatal("0 and negative limits are unlimited")
	}
	if UsageExhausted(1, 0) || UsageExhausted(10, 9) {
		t.Fatal("a link with joins left is not exhausted")
	}
	if !UsageExhausted(1, 1) || !UsageExhausted(10, 10) || !UsageExhausted(10, 11) {
		t.Fatal("used >= limit must be exhausted")
	}
}

func TestChatInviteExportedOmitsUnlimitedUsage(t *testing.T) {
	unlimited := mtproto.MakeTLChatInviteExported(&mtproto.ExportedChatInvite{
		Link:       "https://tell.cc/+abc",
		AdminId:    1,
		Date:       1,
		UsageLimit: mtproto.MakeFlagsInt32(0),
	}).To_ExportedChatInvite()
	if unlimited.GetUsageLimit() != nil {
		t.Fatal("MakeFlagsInt32(0) must leave usage_limit unset")
	}
	if bit := usageLimitFlag(t, unlimited); bit != 0 {
		t.Fatalf("unlimited usage_limit flag %#x", bit)
	}
	limited := mtproto.MakeTLChatInviteExported(&mtproto.ExportedChatInvite{
		Link:       "https://tell.cc/+abc",
		AdminId:    1,
		Date:       1,
		UsageLimit: &wrapperspb.Int32Value{Value: 10},
	}).To_ExportedChatInvite()
	if bit := usageLimitFlag(t, limited); bit == 0 {
		t.Fatal("a positive usage_limit must set the flag")
	}
}

func usageLimitFlag(t *testing.T, inv *mtproto.ExportedChatInvite) uint32 {
	t.Helper()
	buf := mtproto.NewEncodeBuf(128)
	if err := inv.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if len(raw) < 8 {
		t.Fatalf("short encoding %d", len(raw))
	}
	ctor := binary.LittleEndian.Uint32(raw[:4])
	if ctor != 0xa22cbd96 {
		t.Fatalf("constructor %#x", ctor)
	}
	return binary.LittleEndian.Uint32(raw[4:8]) & (1 << 2)
}

func TestInviteNotStarted(t *testing.T) {
	if NotStarted(0, 100) || NotStarted(50, 50) || NotStarted(40, 50) {
		t.Fatal("a link with no future start_date is usable")
	}
	if !NotStarted(80, 50) {
		t.Fatal("start_date in the future must be refused")
	}
}
