package mtproto

import (
	"math"
	"strconv"
	"strings"

	"google.golang.org/grpc/status"
)

// ChatSendForbidden is the 403 a member receives for one outgoing message.
// admin skips member bans. rights are the default ban and any per-user ban;
// a flag applies while now is still before its until_date. A temporary ban
// appends that unix time, the same shape as FLOOD_WAIT, so the client can
// show when the restriction ends. A forever ban (until_date 0 or MaxInt32)
// is the plain CHAT_SEND_* / CHAT_WRITE_FORBIDDEN code.
func ChatSendForbidden(now int32, admin bool, rights []*ChatBannedRights, msg *Message) error {
	if admin || msg == nil || msg.GetPredicateName() == Predicate_messageService {
		return nil
	}
	if on, until := banActive(rights, now, func(r *ChatBannedRights) bool { return r.GetSendMessages() }); on {
		return forbidUntil(ErrChatWriteForbidden, "CHAT_WRITE_FORBIDDEN", until)
	}
	if msg.GetViaBotId().GetValue() != 0 {
		if on, until := banActive(rights, now, func(r *ChatBannedRights) bool { return r.GetSendInline() }); on {
			return forbidUntil(ErrForbiddenChatSendInlineForbidden, "CHAT_SEND_INLINE_FORBIDDEN", until)
		}
	}
	switch kindOf(msg) {
	case sendPhoto:
		return firstBan(now, rights,
			ban{func(r *ChatBannedRights) bool { return r.GetSendPhotos() }, ErrChatSendPhotosForbidden, "CHAT_SEND_PHOTOS_FORBIDDEN"},
			ban{func(r *ChatBannedRights) bool { return r.GetSendMedia() }, ErrChatSendMediaForbidden, "CHAT_SEND_MEDIA_FORBIDDEN"},
		)
	case sendVideo:
		return firstBan(now, rights,
			ban{func(r *ChatBannedRights) bool { return r.GetSendVideos() }, ErrChatSendVideosForbidden, "CHAT_SEND_VIDEOS_FORBIDDEN"},
			ban{func(r *ChatBannedRights) bool { return r.GetSendMedia() }, ErrChatSendMediaForbidden, "CHAT_SEND_MEDIA_FORBIDDEN"},
		)
	case sendRound:
		return firstBan(now, rights,
			ban{func(r *ChatBannedRights) bool { return r.GetSendRoundvideos() }, ErrChatSendRoundvideosForbidden, "CHAT_SEND_ROUNDVIDEOS_FORBIDDEN"},
			ban{func(r *ChatBannedRights) bool { return r.GetSendVideos() }, ErrChatSendVideosForbidden, "CHAT_SEND_VIDEOS_FORBIDDEN"},
			ban{func(r *ChatBannedRights) bool { return r.GetSendMedia() }, ErrChatSendMediaForbidden, "CHAT_SEND_MEDIA_FORBIDDEN"},
		)
	case sendAudio:
		return firstBan(now, rights,
			ban{func(r *ChatBannedRights) bool { return r.GetSendAudios() }, ErrChatSendAudiosForbidden, "CHAT_SEND_AUDIOS_FORBIDDEN"},
			ban{func(r *ChatBannedRights) bool { return r.GetSendMedia() }, ErrChatSendMediaForbidden, "CHAT_SEND_MEDIA_FORBIDDEN"},
		)
	case sendVoice:
		return firstBan(now, rights,
			ban{func(r *ChatBannedRights) bool { return r.GetSendVoices() }, ErrChatSendVoicesForbidden, "CHAT_SEND_VOICES_FORBIDDEN"},
			ban{func(r *ChatBannedRights) bool { return r.GetSendMedia() }, ErrChatSendMediaForbidden, "CHAT_SEND_MEDIA_FORBIDDEN"},
		)
	case sendDoc:
		return firstBan(now, rights,
			ban{func(r *ChatBannedRights) bool { return r.GetSendDocs() }, ErrChatSendDocsForbidden, "CHAT_SEND_DOCS_FORBIDDEN"},
			ban{func(r *ChatBannedRights) bool { return r.GetSendMedia() }, ErrChatSendMediaForbidden, "CHAT_SEND_MEDIA_FORBIDDEN"},
		)
	case sendSticker:
		return firstBan(now, rights,
			ban{func(r *ChatBannedRights) bool { return r.GetSendStickers() }, ErrChatSendStickersForbidden, "CHAT_SEND_STICKERS_FORBIDDEN"},
			ban{func(r *ChatBannedRights) bool { return r.GetSendMedia() }, ErrChatSendMediaForbidden, "CHAT_SEND_MEDIA_FORBIDDEN"},
		)
	case sendGif:
		return firstBan(now, rights,
			ban{func(r *ChatBannedRights) bool { return r.GetSendGifs() }, ErrChatSendGifsForbidden, "CHAT_SEND_GIFS_FORBIDDEN"},
			ban{func(r *ChatBannedRights) bool { return r.GetSendMedia() }, ErrChatSendMediaForbidden, "CHAT_SEND_MEDIA_FORBIDDEN"},
		)
	case sendGame:
		return firstBan(now, rights,
			ban{func(r *ChatBannedRights) bool { return r.GetSendGames() }, ErrChatSendGameForbidden, "CHAT_SEND_GAME_FORBIDDEN"},
			ban{func(r *ChatBannedRights) bool { return r.GetSendMedia() }, ErrChatSendMediaForbidden, "CHAT_SEND_MEDIA_FORBIDDEN"},
		)
	case sendPoll:
		if on, until := banActive(rights, now, func(r *ChatBannedRights) bool { return r.GetSendPolls() }); on {
			return forbidUntil(ErrChatSendPollForbidden, "CHAT_SEND_POLL_FORBIDDEN", until)
		}
		return nil
	case sendPlain:
		if on, until := banActive(rights, now, func(r *ChatBannedRights) bool { return r.GetSendPlain() }); on {
			return forbidUntil(ErrChatSendPlainForbidden, "CHAT_SEND_PLAIN_FORBIDDEN", until)
		}
		if messageEmbedsLink(msg) {
			if on, until := banActive(rights, now, func(r *ChatBannedRights) bool { return r.GetEmbedLinks() }); on {
				return forbidUntil(ErrChatSendWebpageForbidden, "CHAT_SEND_WEBPAGE_FORBIDDEN", until)
			}
		}
		return nil
	default:
		if on, until := banActive(rights, now, func(r *ChatBannedRights) bool { return r.GetSendMedia() }); on {
			return forbidUntil(ErrChatSendMediaForbidden, "CHAT_SEND_MEDIA_FORBIDDEN", until)
		}
		return nil
	}
}

// BasicGroupSendForbidden applies default_banned_rights on a basic group.
// The creator and admins are not restricted by those rights.
func BasicGroupSendForbidden(chat *MutableChat, userID int64, now int32, msgs []*Message) error {
	if chat == nil {
		return ErrPeerIdInvalid
	}
	p, ok := chat.GetImmutableChatParticipant(userID)
	if !ok || !p.IsChatMemberStateNormal() {
		return ErrChatWriteForbidden
	}
	admin := p.IsChatMemberCreator() || p.IsChatMemberAdmin()
	rights := []*ChatBannedRights{chat.DefaultBannedRights()}
	for _, msg := range msgs {
		if err := ChatSendForbidden(now, admin, rights, msg); err != nil {
			return err
		}
	}
	return nil
}

// MemberPinForbidden is CHAT_ADMIN_REQUIRED when a non-admin tries to pin
// and pin_messages is banned. Admins who already hold the pin right pass.
func MemberPinForbidden(now int32, canPinAsAdmin bool, rights []*ChatBannedRights) error {
	if canPinAsAdmin {
		return nil
	}
	if on, _ := banActive(rights, now, func(r *ChatBannedRights) bool { return r.GetPinMessages() }); on {
		return ErrChatAdminRequired
	}
	return nil
}

// MemberTopicsForbidden is CHAT_ADMIN_REQUIRED when a non-admin tries to
// create a forum topic and manage_topics is banned.
func MemberTopicsForbidden(now int32, canManageAsAdmin bool, rights []*ChatBannedRights) error {
	if canManageAsAdmin {
		return nil
	}
	if on, _ := banActive(rights, now, func(r *ChatBannedRights) bool { return r.GetManageTopics() }); on {
		return ErrChatAdminRequired
	}
	return nil
}

type sendKind int

const (
	sendPlain sendKind = iota
	sendPhoto
	sendVideo
	sendRound
	sendAudio
	sendVoice
	sendDoc
	sendSticker
	sendGif
	sendGame
	sendPoll
	sendOther
)

type ban struct {
	on   func(*ChatBannedRights) bool
	err  error
	code string
}

func firstBan(now int32, rights []*ChatBannedRights, checks ...ban) error {
	for _, c := range checks {
		if on, until := banActive(rights, now, c.on); on {
			return forbidUntil(c.err, c.code, until)
		}
	}
	return nil
}

func banActive(rights []*ChatBannedRights, now int32, flag func(*ChatBannedRights) bool) (bool, int32) {
	var until int32
	active := false
	for _, r := range rights {
		if r == nil || !flag(r) {
			continue
		}
		u := r.GetUntilDate()
		if u == 0 {
			u = math.MaxInt32
		}
		if now >= u {
			continue
		}
		if !active || u > until {
			until = u
			active = true
		}
	}
	return active, until
}

func forbidUntil(static error, code string, until int32) error {
	if until == 0 || until == math.MaxInt32 {
		return static
	}
	return status.Error(ErrForbidden, code+"_"+strconv.FormatInt(int64(until), 10))
}

func kindOf(msg *Message) sendKind {
	media := msg.GetMedia()
	switch media.GetPredicateName() {
	case "", Predicate_messageMediaEmpty:
		return sendPlain
	case Predicate_messageMediaWebPage:
		return sendPlain
	case Predicate_messageMediaPhoto:
		return sendPhoto
	case Predicate_messageMediaPoll:
		return sendPoll
	case Predicate_messageMediaGame:
		return sendGame
	case Predicate_messageMediaDocument:
		return documentKind(media.GetDocument())
	default:
		return sendOther
	}
}

func documentKind(doc *Document) sendKind {
	if doc == nil {
		return sendDoc
	}
	if IsStickerDocument(doc) {
		return sendSticker
	}
	if IsGifDocument(doc) {
		return sendGif
	}
	for _, a := range doc.GetAttributes() {
		switch a.GetPredicateName() {
		case Predicate_documentAttributeVideo:
			if a.GetRoundMessage() {
				return sendRound
			}
		case Predicate_documentAttributeAudio:
			if a.GetVoice() {
				return sendVoice
			}
			return sendAudio
		}
	}
	for _, a := range doc.GetAttributes() {
		if a.GetPredicateName() == Predicate_documentAttributeVideo {
			return sendVideo
		}
	}
	if IsMusicDocument(doc) {
		return sendAudio
	}
	return sendDoc
}

func messageEmbedsLink(msg *Message) bool {
	if msg == nil {
		return false
	}
	if msg.GetMedia().GetPredicateName() == Predicate_messageMediaWebPage {
		return true
	}
	for _, e := range msg.GetEntities() {
		switch e.GetPredicateName() {
		case Predicate_messageEntityUrl, Predicate_messageEntityTextUrl:
			return true
		}
	}
	text := strings.ToLower(msg.GetMessage())
	return strings.Contains(text, "http://") || strings.Contains(text, "https://") || strings.Contains(text, "www.")
}
