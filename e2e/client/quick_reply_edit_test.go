package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestEditQuickReplyMessage is ISSUE-S-QR-EDIT-001 (表格行 89).
// messages.editMessage with quick_reply_shortcut_id rewrites the shortcut
// message. Phone 9996610644, code 11111, layer 232.
func TestEditQuickReplyMessage(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610644", "QuickA")
	sent, err := a.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerSelf(),
		Message:     "qr-first",
		RandomId:    randomID(),
		QuickReplyShortcut: mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{
			Shortcut: "qredit",
		}).To_InputQuickReplyShortcut(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var msgID, shortcutID int32
	for _, u := range updatesOf(sent) {
		if m := u.GetMessage_MESSAGE(); m.GetMessage() == "qr-first" {
			msgID = m.GetId()
			shortcutID = m.GetQuickReplyShortcutId().GetValue()
		}
	}
	if msgID == 0 || shortcutID == 0 {
		t.Fatalf("capture %T id=%d shortcut=%d", sent, msgID, shortcutID)
	}
	edited, err := a.Invoke(ctx, &mtproto.TLMessagesEditMessage{
		// Layer 232. The older 48f71778 constructor does not carry
		// quick_reply_shortcut_id, so the edit falls through to Saved Messages.
		Constructor:          mtproto.CRC32_messages_editMessage_b106e66c,
		Peer:                 inputPeerSelf(),
		Id:                   msgID,
		Message:              &wrapperspb.StringValue{Value: "qr-rewritten"},
		QuickReplyShortcutId: &wrapperspb.Int32Value{Value: shortcutID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasText(allMessages(edited), "qr-rewritten") {
		listed, lerr := a.Invoke(ctx, &mtproto.TLMessagesGetQuickReplyMessages{
			Constructor: mtproto.CRC32_messages_getQuickReplyMessages,
			ShortcutId:  shortcutID,
		})
		if lerr != nil || !hasText(allMessages(listed), "qr-rewritten") {
			t.Fatalf("edit %T list %v", edited, lerr)
		}
	}
}
