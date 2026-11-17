package dialog

import (
	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// InputTopicID is the forum topic on inputReplyToMessage.
// Zero means the dialog composer, not a topic.
func InputTopicID(reply *mtproto.InputReplyTo) int32 {
	if reply == nil || reply.GetPredicateName() != mtproto.Predicate_inputReplyToMessage {
		return 0
	}
	top := reply.GetTopMsgId()
	if top == nil || top.GetValue() <= 0 {
		return 0
	}
	return top.GetValue()
}

// DraftTopicID is the forum topic stored on a draft.
// The dialog service keys topic rows by this value. Zero is the dialog draft.
func DraftTopicID(msg *mtproto.DraftMessage) int32 {
	if msg == nil {
		return 0
	}
	return InputTopicID(msg.GetReplyTo())
}

// ReplyCarriesDraftBody reports a reply that is more than the topic address.
// inputReplyToMessage with only top_msg_id (or reply_to_msg_id equal to that
// topic) is how a client names the topic. A quote, a reply to another
// message, or another reply constructor is draft content.
func ReplyCarriesDraftBody(reply *mtproto.InputReplyTo) bool {
	if reply == nil {
		return false
	}
	if name := reply.GetPredicateName(); name != "" && name != mtproto.Predicate_inputReplyToMessage {
		return true
	}
	if reply.GetQuoteText() != nil || len(reply.GetQuoteEntities()) > 0 || reply.GetReplyToPeerId() != nil {
		return true
	}
	if reply.GetTodoItemId() != nil || len(reply.GetPollOption()) > 0 {
		return true
	}
	top := int32(0)
	if reply.GetTopMsgId() != nil {
		top = reply.GetTopMsgId().GetValue()
	}
	rid := reply.GetReplyToMsgId()
	if rid != 0 && rid != top {
		return true
	}
	if top > 0 {
		return false
	}
	return rid != 0
}

// DraftIsEmpty reports a draft the composer should treat as cleared.
// draftMessageEmpty is empty even when it still carries the topic address.
func DraftIsEmpty(msg *mtproto.DraftMessage) bool {
	if msg == nil || msg.GetPredicateName() == mtproto.Predicate_draftMessageEmpty {
		return true
	}
	if msg.GetMessage() != "" || len(msg.GetEntities()) > 0 {
		return false
	}
	if msg.GetNoWebpage() || msg.GetInvertMedia() {
		return false
	}
	if msg.GetMedia() != nil || msg.GetEffect() != nil || msg.GetSuggestedPost() != nil || msg.GetRichMessage() != nil {
		return false
	}
	if ReplyCarriesDraftBody(msg.GetReplyTo()) {
		return false
	}
	if id := msg.GetReplyToMsgId(); id != nil && id.GetValue() != 0 && id.GetValue() != DraftTopicID(msg) {
		return false
	}
	return true
}

// VisibleTopicDraft is the draft forumTopic.draft should carry.
// An empty topic draft is omitted. A dialog draft (top_msg_id 0) is omitted.
func VisibleTopicDraft(msg *mtproto.DraftMessage) *mtproto.DraftMessage {
	if DraftIsEmpty(msg) || DraftTopicID(msg) <= 0 {
		return nil
	}
	return msg
}

// EnsureTopicReply stamps top_msg_id onto the draft so a later read can tell
// a topic draft from the dialog draft without a second column on the wire.
func EnsureTopicReply(msg *mtproto.DraftMessage, top int32) {
	if msg == nil || top <= 0 || InputTopicID(msg.GetReplyTo()) == top {
		return
	}
	reply := msg.GetReplyTo()
	if reply == nil || reply.GetPredicateName() != mtproto.Predicate_inputReplyToMessage {
		reply = mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{}).To_InputReplyTo()
		msg.ReplyTo = reply
	}
	reply.TopMsgId = wrapperspb.Int32(top)
}

// DraftUpdate is updateDraftMessage. Topic drafts set top_msg_id.
// A dialog draft leaves the flag unset.
func DraftUpdate(peer *mtproto.Peer, draft *mtproto.DraftMessage) *mtproto.Update {
	upd := mtproto.MakeTLUpdateDraftMessage(&mtproto.Update{
		Peer_PEER: peer,
		Draft:     draft,
	}).To_Update()
	if top := DraftTopicID(draft); top > 0 {
		upd.TopMsgId_FLAGINT32 = wrapperspb.Int32(top)
	}
	return upd
}
