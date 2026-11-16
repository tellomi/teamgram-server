package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestAutoarchivedFlag is the new-noncontact archive path. The dialog is
// archived and peerSettings.autoarchived is set, while the stranger bar
// flags stay.
//
// The recipient must not import the sender first. contacts.importContacts
// saves a contact, and the inbox then treats the message as coming from a
// contact, so neither the stranger bar nor auto-archive runs.
func TestAutoarchivedFlag(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610775", "ArchA")
	b, _ := dialPhone(t, ctx, cfg, "9996610861", "ArchB")
	setPrivacy(t, ctx, b, true, false, false)
	bID, bHash, err := a.ImportPhone(ctx, "9996610861", "ArchB")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.SendText(ctx, inputPeerUser(bID, bHash), "hello archived", nil); err != nil {
		t.Fatal(err)
	}
	var auto, spam bool
	if err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		hash, ok, err := folderUserHash(ctx, b, aSelf.ID)
		if err != nil || !ok {
			return false, err
		}
		got, err := loadPeerSettings(ctx, b, aSelf.ID, hash)
		if err != nil {
			return false, err
		}
		auto, spam = got.GetAutoarchived(), got.GetReportSpam()
		return auto && spam, nil
	}); err != nil {
		t.Fatalf("autoarchived=%v report_spam=%v err=%v", auto, spam, err)
	}
}

// folderUserHash is the access hash of a user already filed in the archive.
func folderUserHash(ctx context.Context, c *Client, id int64) (int64, bool, error) {
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetDialogs{
		Constructor: mtproto.CRC32_messages_getDialogs,
		OffsetPeer:  inputPeerEmpty(),
		Limit:       100,
		FolderId:    &wrapperspb.Int32Value{Value: 1},
	})
	if err != nil {
		return 0, false, err
	}
	for _, u := range archiveUsers(obj) {
		if u != nil && u.GetId() == id && u.GetAccessHash().GetValue() != 0 {
			return u.GetAccessHash().GetValue(), true, nil
		}
	}
	return 0, false, nil
}
