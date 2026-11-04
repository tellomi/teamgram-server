package invite

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
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
	if err := (Export{Usage: 0, UsageSet: true, Now: now}).Validate(); err == nil {
		t.Fatal("zero usage accepted")
	}
	if err := (Export{Title: strings.Repeat("名", TitleMax+1), TitleSet: true, Now: now}).Validate(); err == nil {
		t.Fatal("long title accepted")
	}
}
