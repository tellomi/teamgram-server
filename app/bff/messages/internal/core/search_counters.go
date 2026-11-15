package core

import "github.com/teamgram/proto/mtproto"

// searchCounterMedia is the message_filter_type messages.getSearchCounters
// asks the message service to count.
//
// Photos are stored as MEDIA_PHOTOS_ONLY and videos as MEDIA_VIDEOS_ONLY.
// MEDIA_PHOTOVIDEO is only the legacy combined bucket, so a photo+video
// counter has to sum all three. A new voice note is MEDIA_VOICE_FILE and a
// new round video is MEDIA_ROUND_FILE, counted on that column like a photo.
// MEDIA_AUDIO remains the legacy bucket and the round-voice total.
func searchCounterMedia(filter *mtproto.MessagesFilter) (int32, bool) {
	switch mtproto.FromMessagesFilter(filter) {
	case mtproto.FilterPhotos:
		return mtproto.MEDIA_PHOTOS_ONLY, true
	case mtproto.FilterVideo:
		return mtproto.MEDIA_VIDEOS_ONLY, true
	case mtproto.FilterPhotoVideo:
		return mtproto.MEDIA_PHOTOVIDEO, true
	case mtproto.FilterDocument:
		return mtproto.MEDIA_FILE, true
	case mtproto.FilterUrl:
		return mtproto.MEDIA_URL, true
	case mtproto.FilterGif:
		return mtproto.MEDIA_GIF, true
	case mtproto.FilterMusic:
		return mtproto.MEDIA_MUSIC, true
	case mtproto.FilterVoice:
		return mtproto.MEDIA_VOICE_FILE, true
	case mtproto.FilterRoundVideo:
		return mtproto.MEDIA_ROUND_FILE, true
	case mtproto.FilterRoundVoice:
		return mtproto.MEDIA_AUDIO, true
	case mtproto.FilterChatPhotos:
		return mtproto.MEDIA_CHAT_PHOTO, true
	case mtproto.FilterPhoneCalls:
		return mtproto.MEDIA_PHONE_CALL, true
	case mtproto.FilterPoll:
		return mtproto.MEDIA_POLL, true
	case mtproto.FilterGeo:
		return mtproto.MEDIA_GEO, true
	case mtproto.FilterContacts:
		return mtproto.MEDIA_CONTACT, true
	case mtproto.FilterPinned:
		return mtproto.MEDIA_PINNED, true
	default:
		return 0, false
	}
}
