package core

import "github.com/teamgram/proto/mtproto"

// replyHeaderFromInput copies inputReplyToMessage onto messageReplyHeader.
// reply_to_peer_id is a quote of a message in another chat. Dropping the
// whole header used to discard the quote as well.
func replyHeaderFromInput(self int64, in *mtproto.InputReplyTo) *mtproto.MessageReplyHeader {
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
	if top := in.GetTopMsgId(); top != nil {
		header.ReplyToTopId = top
	}
	if in.GetReplyToPeerId() != nil {
		other := mtproto.FromInputPeer2(self, in.GetReplyToPeerId())
		if other != nil && (other.IsUser() || other.IsChat() || other.IsChannel()) {
			header.ReplyToPeerId = other.ToPeer()
		}
	}
	return header
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
