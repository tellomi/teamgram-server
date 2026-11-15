package e2e

import (
	"testing"
	"time"
)

// TestAutoarchivedFlag is the new-noncontact archive path. The dialog is
// archived and peerSettings.autoarchived is set, while the stranger bar
// flags stay.
func TestAutoarchivedFlag(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610775", "ArchA")
	b, _ := dialPhone(t, ctx, cfg, "9996610776", "ArchB")
	setPrivacy(t, ctx, b, true, false, false)
	bID, bHash, err := a.ImportPhone(ctx, "9996610776", "ArchB")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.SendText(ctx, inputPeerUser(bID, bHash), "hello archived", nil); err != nil {
		t.Fatal(err)
	}
	aID, aHash, err := b.ImportPhone(ctx, "9996610775", "ArchA")
	if err != nil || aID != aSelf.ID || aHash == 0 {
		t.Fatalf("import sender %d %d %v", aID, aHash, err)
	}
	var auto, spam bool
	if err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		got, err := loadPeerSettings(ctx, b, aSelf.ID, aHash)
		if err != nil {
			return false, err
		}
		auto, spam = got.GetAutoarchived(), got.GetReportSpam()
		return auto && spam, nil
	}); err != nil {
		t.Fatalf("autoarchived=%v report_spam=%v err=%v", auto, spam, err)
	}
}
