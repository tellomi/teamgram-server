package channel

import "github.com/teamgram/proto/mtproto"

// channelFilterMatches is messages.search's filter against one channel post.
// An empty filter matches every message. Photo+video matches either media.
func channelFilterMatches(msg *mtproto.Message, filter *mtproto.MessagesFilter, pinned bool) bool {
	if filter == nil {
		return true
	}
	switch mtproto.FromMessagesFilter(filter) {
	case mtproto.FilterEmpty:
		return true
	case mtproto.FilterPinned:
		return pinned || msg.GetPinned()
	case mtproto.FilterPhotoVideo:
		t := mtproto.GetMediaType(msg)
		return t == mtproto.MEDIA_PHOTOS_ONLY || t == mtproto.MEDIA_VIDEOS_ONLY
	case mtproto.FilterPhotos:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_PHOTOS_ONLY
	case mtproto.FilterVideo:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_VIDEOS_ONLY
	case mtproto.FilterDocument:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_FILE
	case mtproto.FilterUrl:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_URL
	case mtproto.FilterGif:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_GIF
	case mtproto.FilterMusic:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_MUSIC
	case mtproto.FilterVoice, mtproto.FilterRoundVoice, mtproto.FilterRoundVideo:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_AUDIO
	case mtproto.FilterChatPhotos:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_CHAT_PHOTO
	case mtproto.FilterPhoneCalls:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_PHONE_CALL
	case mtproto.FilterPoll:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_POLL
	case mtproto.FilterGeo:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_GEO
	case mtproto.FilterContacts:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_CONTACT
	default:
		return false
	}
}

func storedInTopic(f *Features, stored *StoredMessage, top int32) bool {
	if stored == nil || top == 0 {
		return false
	}
	if stored.ID == top || sameTopic(f, stored.ID, top) {
		return true
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil || body.GetReplyTo() == nil {
		return false
	}
	h := body.GetReplyTo()
	return h.GetReplyToTopId().GetValue() == top || h.GetReplyToMsgId() == top
}
