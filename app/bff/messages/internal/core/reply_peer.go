package core

import "github.com/teamgram/proto/mtproto"

// replyHeaderFromInput copies inputReplyToMessage onto messageReplyHeader.
// reply_to_peer_id is a quote of a message in another chat. Dropping the
// whole header used to discard the quote as well.
func replyHeaderFromInput(self int64, dest *mtproto.PeerUtil, in *mtproto.InputReplyTo) *mtproto.MessageReplyHeader {
	if in == nil || in.GetPredicateName() != mtproto.Predicate_inputReplyToMessage {
		return nil
	}
	header := mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId:           in.GetReplyToMsgId(),
		ReplyToMsgId_INT32:     in.GetReplyToMsgId(),
		ReplyToMsgId_FLAGINT32: mtproto.MakeFlagsInt32(in.GetReplyToMsgId()),
	}).To_MessageReplyHeader()
	if in.GetQuoteText() != nil {
		header.Quote = true
		header.QuoteText = in.GetQuoteText()
		header.QuoteEntities = in.GetQuoteEntities()
		header.QuoteOffset = in.GetQuoteOffset()
	}
	if in.GetTodoItemId() != nil {
		header.TodoItemId = in.GetTodoItemId()
	}
	if top := in.GetTopMsgId(); top != nil {
		header.ReplyToTopId = top
	}
	if in.GetReplyToPeerId() != nil {
		other := mtproto.FromInputPeer2(self, in.GetReplyToPeerId())
		// The same dialog is an ordinary reply. reply_to_peer_id is only for
		// a quote that lives in a different chat.
		if other != nil && !sameDialog(self, dest, other) && (other.IsUser() || other.IsChat() || other.IsChannel()) {
			header.ReplyToPeerId = other.ToPeer()
		}
	}
	return header
}

func sameDialog(self int64, dest, other *mtproto.PeerUtil) bool {
	if dest == nil || other == nil {
		return false
	}
	dType, dID := dest.PeerType, dest.PeerId
	oType, oID := other.PeerType, other.PeerId
	if dType == mtproto.PEER_SELF {
		dType, dID = mtproto.PEER_USER, self
	}
	if oType == mtproto.PEER_SELF {
		oType, oID = mtproto.PEER_USER, self
	}
	return dType == oType && dID != 0 && dID == oID
}

// dropForwardAuthor removes fwd_from. messages.forwardMessages drop_author
// sends the content as the caller's own message.
// https://core.telegram.org/method/messages.forwardMessages
func dropForwardAuthor(m *mtproto.Message) {
	if m == nil {
		return
	}
	m.FwdFrom = nil
	m.SavedPeerId = nil
	m.PostAuthor = nil
}

// dropMediaCaptions strips the caption from a photo or a document
// (video, file, gif, audio). The media stays. A text message and a link
// preview keep their text: that text is the message, not a media caption.
// https://core.telegram.org/method/messages.forwardMessages
func dropMediaCaptions(m *mtproto.Message) {
	if m == nil || m.Media == nil {
		return
	}
	switch m.Media.GetPredicateName() {
	case mtproto.Predicate_messageMediaPhoto, mtproto.Predicate_messageMediaDocument:
		m.Message = ""
		m.Entities = nil
	}
}
