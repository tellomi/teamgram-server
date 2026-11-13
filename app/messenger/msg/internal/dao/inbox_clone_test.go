package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestCloneForInboxLeavesSenderMessageOutgoing(t *testing.T) {
	original := &mtproto.Message{Out: true, Id: 7, Message: "hi"}
	cloned := cloneForInbox(original)
	if !original.GetOut() || original.GetId() != 7 || original.GetMessage() != "hi" {
		t.Fatalf("sender message mutated: %+v", original)
	}
	if cloned.GetOut() || cloned.GetId() != 7 || cloned.GetMessage() != "hi" {
		t.Fatalf("inbox copy = %+v", cloned)
	}
	if cloneForInbox(nil) != nil {
		t.Fatal("nil clone")
	}
}
