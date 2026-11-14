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
func dropForwardAuthor(m *mtproto.Message) {
	if m == nil {
		return
	}
	m.FwdFrom = nil
	m.SavedPeerId = nil
	m.PostAuthor = nil
}
