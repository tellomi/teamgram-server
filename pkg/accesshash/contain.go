package accesshash

import "github.com/teamgram/proto/mtproto"

// MessageContainsUser reports whether msg shows userID to a client.
// senderUserID is the private-chat or basic-group sender stored beside the
// message. Channel posts pass 0: an anonymous post's internal sender is not
// something the client was shown, and treating it as proof would let a
// caller confirm guessed user ids against a public post.
func MessageContainsUser(msg *mtproto.Message, senderUserID, userID int64) bool {
	if userID == 0 || messageEmpty(msg) {
		return false
	}
	if senderUserID == userID {
		return true
	}
	if msg == nil {
		return false
	}
	if peerUserID(msg.GetFromId()) == userID {
		return true
	}
	if msg.GetViaBotId().GetValue() == userID || msg.GetViaBusinessBotId().GetValue() == userID {
		return true
	}
	if fwd := msg.GetFwdFrom(); fwd != nil {
		if peerUserID(fwd.GetFromId()) == userID || peerUserID(fwd.GetSavedFromId()) == userID {
			return true
		}
	}
	if entityHasUser(msg.GetEntities(), userID) {
		return true
	}
	if media := msg.GetMedia(); media != nil && media.GetUserId() == userID {
		return true
	}
	if actionHasUser(msg.GetAction(), userID) {
		return true
	}
	if reply := msg.GetReplyTo(); reply != nil {
		if reply.GetUserId() == userID || peerUserID(reply.GetPeer()) == userID {
			return true
		}
		if fwd := reply.GetReplyFrom(); fwd != nil && peerUserID(fwd.GetFromId()) == userID {
			return true
		}
		if entityHasUser(reply.GetQuoteEntities(), userID) {
			return true
		}
	}
	return false
}

// MessageContainsChannel reports whether msg refers to channelID.
// The dialog the message lives in is not itself a reference. The caller
// already checked that the viewer can see that dialog.
func MessageContainsChannel(msg *mtproto.Message, channelID int64) bool {
	if channelID == 0 || msg == nil || messageEmpty(msg) {
		return false
	}
	if peerChannelID(msg.GetFromId()) == channelID {
		return true
	}
	if fwd := msg.GetFwdFrom(); fwd != nil {
		if peerChannelID(fwd.GetFromId()) == channelID || peerChannelID(fwd.GetSavedFromPeer()) == channelID {
			return true
		}
	}
	if media := msg.GetMedia(); media != nil {
		if media.GetChannelId() == channelID || containsInt(media.GetChannels(), channelID) {
			return true
		}
	}
	if action := msg.GetAction(); action != nil && action.GetChannelId() == channelID {
		return true
	}
	if reply := msg.GetReplyTo(); reply != nil && peerChannelID(reply.GetReplyToPeerId()) == channelID {
		return true
	}
	return false
}

func messageEmpty(msg *mtproto.Message) bool {
	return msg != nil && msg.GetPredicateName() == mtproto.Predicate_messageEmpty
}

func entityHasUser(entities []*mtproto.MessageEntity, userID int64) bool {
	for _, e := range entities {
		if e.GetUserId_INT64() == userID {
			return true
		}
		if u := e.GetUserId_INPUTUSER(); u != nil && u.GetUserId() == userID {
			return true
		}
	}
	return false
}

func actionHasUser(a *mtproto.MessageAction, userID int64) bool {
	if a == nil {
		return false
	}
	if a.GetUserId() == userID || a.GetInviterId() == userID {
		return true
	}
	if containsInt(a.GetUsers(), userID) {
		return true
	}
	return peerUserID(a.GetFromId()) == userID || peerUserID(a.GetToId()) == userID
}

func peerUserID(p *mtproto.Peer) int64 {
	if p == nil {
		return 0
	}
	if p.GetPredicateName() == mtproto.Predicate_peerUser || p.GetConstructor() == mtproto.CRC32_peerUser {
		return p.GetUserId()
	}
	return 0
}

func peerChannelID(p *mtproto.Peer) int64 {
	if p == nil {
		return 0
	}
	if p.GetPredicateName() == mtproto.Predicate_peerChannel || p.GetConstructor() == mtproto.CRC32_peerChannel {
		return p.GetChannelId()
	}
	return 0
}

func containsInt(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
