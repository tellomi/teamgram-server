package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestSenderChatDialogIsTheBasicGroupBeforeInbox(t *testing.T) {
	box := mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		MessageId: 7,
		Message:   &mtproto.Message{Date: 1700000000},
	}).To_MessageBox()
	in := SenderChatDialog(4, 88, box)
	if in.UserId != 4 || in.PeerType != mtproto.PEER_CHAT || in.PeerId != 88 {
		t.Fatalf("peer %+v", in)
	}
	if in.GetTopMessage().GetValue() != 7 || in.GetDate2().GetValue() != 1700000000 {
		t.Fatalf("top %+v date %+v", in.GetTopMessage(), in.GetDate2())
	}
	if in.GetUnreadCount().GetValue() != 0 {
		t.Fatalf("unread %+v", in.GetUnreadCount())
	}
	newest := NewestOutbox([]*mtproto.MessageBox{
		{MessageId: 3},
		nil,
		{MessageId: 9},
		{MessageId: 9},
	})
	if newest == nil || newest.MessageId != 9 {
		t.Fatalf("newest %+v", newest)
	}
}
