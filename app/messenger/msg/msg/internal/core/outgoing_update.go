package core

import (
	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/proto"
)

// outgoingUpdate is one updateNewMessage for the sender's messages.sendMessage
// result. The inbox delivery clones its own copy and clears out; this clone
// stays out=true with the sender's message id and the client random_id.
//
// Android (SendMessagesHelper, layer 232) matches the pending bubble from
// updateMessageID.random_id, then applies updateNewMessage. pts_count must be
// the number of pts this update consumes: the client applies it only when
// localPts + pts_count == pts. A text message consumes one pts.
func outgoingUpdate(box *mtproto.MessageBox) *mtproto.Update {
	if box == nil || box.GetMessage() == nil {
		return nil
	}
	msg, _ := proto.Clone(box.GetMessage()).(*mtproto.Message)
	if msg == nil {
		return nil
	}
	msg.Out = true
	if box.MessageId != 0 {
		msg.Id = box.MessageId
	}
	ptsCount := box.PtsCount
	if box.Pts > 0 && ptsCount <= 0 {
		ptsCount = 1
	}
	return mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Pts_INT32:       box.Pts,
		PtsCount:        ptsCount,
		RandomId:        box.RandomId,
		Message_MESSAGE: msg,
	}).To_Update()
}
