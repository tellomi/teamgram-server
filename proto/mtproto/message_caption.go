package mtproto

// CaptionEditable reports whether messages.getMessageEditData should set
// the caption flag.
//
// https://core.telegram.org/constructor/messages.messageEditData
//
// caption means the editor changes the media caption rather than a plain
// text body. Photos and ordinary documents (video, gif, file, music) have
// captions. Stickers, voice messages, and round video do not.
func CaptionEditable(message *Message) bool {
	if message == nil || message.GetMedia() == nil {
		return false
	}
	switch message.GetMedia().GetPredicateName() {
	case Predicate_messageMediaPhoto:
		return true
	case Predicate_messageMediaDocument:
		if IsStickerMessage(message) || IsVoiceMessage(message) || IsRoundVideoMessage(message) {
			return false
		}
		return true
	default:
		return false
	}
}
