package push

import (
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
)

// Incoming is one new message that may generate a push.
type Incoming struct {
	UserID     int64
	MsgID      int32
	SenderID   int64
	SenderName string
	ChatID     int64
	ChannelID  int64
	ChatTitle  string
	Text       string
	Sticker    string
	LocKey     string
	Mentioned  bool
	Silent     bool
	PeerType   int32
	PeerID     int64
}

// Messages walks updates and returns incoming messages for userID.
// Outgoing copies are skipped. Later items of a grouped album are skipped.
func Messages(userID int64, updates *mtproto.Updates) []Incoming {
	if updates == nil || userID == 0 {
		return nil
	}
	var out []Incoming
	seenGroup := map[int64]struct{}{}
	visit := func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
		msg := update.GetMessage_MESSAGE()
		if msg == nil || msg.GetOut() || msg.GetId() == 0 {
			return
		}
		if g := msg.GetGroupedId().GetValue(); g != 0 {
			if _, ok := seenGroup[g]; ok {
				return
			}
			seenGroup[g] = struct{}{}
		}
		inc, ok := incomingFromMessage(userID, msg, users, chats)
		if ok {
			out = append(out, inc)
		}
	}
	mtproto.VisitUpdates(userID, updates, map[string]mtproto.UpdateVisitedFunc{
		mtproto.Predicate_updateNewMessage:        visit,
		mtproto.Predicate_updateNewChannelMessage: visit,
	})
	return out
}

func incomingFromMessage(userID int64, msg *mtproto.Message, users []*mtproto.User, chats []*mtproto.Chat) (Incoming, bool) {
	peerType, peerID := peerOf(msg.GetPeerId())
	if peerType == 0 {
		return Incoming{}, false
	}
	senderID := msg.GetFromId().GetUserId()
	if senderID == 0 && peerType == mtproto.PEER_USER {
		senderID = peerID
	}
	if senderID == userID && peerType == mtproto.PEER_USER && peerID == userID {
		return Incoming{}, false
	}
	inc := Incoming{
		UserID:    userID,
		MsgID:     msg.GetId(),
		SenderID:  senderID,
		Mentioned: msg.GetMentioned(),
		Silent:    msg.GetSilent(),
		PeerType:  peerType,
		PeerID:    peerID,
		Text:      strings.TrimSpace(msg.GetMessage()),
	}
	inc.SenderName = displayUser(users, senderID)
	switch peerType {
	case mtproto.PEER_CHAT:
		inc.ChatID = peerID
		inc.ChatTitle = displayChat(chats, peerID)
	case mtproto.PEER_CHANNEL:
		inc.ChannelID = peerID
		inc.ChatTitle = displayChat(chats, peerID)
	}
	kind, extra := mediaKind(msg)
	inc.Text = clip(inc.Text, 120)
	if kind == kindSticker {
		inc.Sticker = extra
	}
	inc.LocKey = locKey(peerType, kind, inc.Text)
	return inc, true
}

func peerOf(p *mtproto.Peer) (int32, int64) {
	if p == nil {
		return 0, 0
	}
	switch {
	case p.GetUserId() != 0:
		return mtproto.PEER_USER, p.GetUserId()
	case p.GetChatId() != 0:
		return mtproto.PEER_CHAT, p.GetChatId()
	case p.GetChannelId() != 0:
		return mtproto.PEER_CHANNEL, p.GetChannelId()
	default:
		return 0, 0
	}
}

func displayUser(users []*mtproto.User, id int64) string {
	for _, u := range users {
		if u.GetId() != id {
			continue
		}
		return joinName(u.GetFirstName().GetValue(), u.GetLastName().GetValue())
	}
	return ""
}

func displayChat(chats []*mtproto.Chat, id int64) string {
	for _, ch := range chats {
		if ch.GetId() == id {
			return strings.TrimSpace(ch.GetTitle())
		}
	}
	return ""
}

type kind int

const (
	kindText kind = iota
	kindNoText
	kindPhoto
	kindVideo
	kindGIF
	kindRound
	kindVoice
	kindSticker
	kindDoc
	kindGeo
	kindGeoLive
	kindContact
	kindPoll
)

func mediaKind(msg *mtproto.Message) (kind, string) {
	media := msg.GetMedia()
	if media == nil || media.GetPredicateName() == "" || media.GetPredicateName() == mtproto.Predicate_messageMediaEmpty {
		if strings.TrimSpace(msg.GetMessage()) == "" {
			return kindNoText, ""
		}
		return kindText, ""
	}
	switch media.GetPredicateName() {
	case mtproto.Predicate_messageMediaPhoto:
		return kindPhoto, ""
	case mtproto.Predicate_messageMediaGeo:
		return kindGeo, ""
	case mtproto.Predicate_messageMediaGeoLive:
		return kindGeoLive, ""
	case mtproto.Predicate_messageMediaContact:
		return kindContact, ""
	case mtproto.Predicate_messageMediaPoll:
		return kindPoll, ""
	case mtproto.Predicate_messageMediaDocument:
		return documentKind(media.GetDocument())
	case mtproto.Predicate_messageMediaWebPage:
		if strings.TrimSpace(msg.GetMessage()) == "" {
			return kindNoText, ""
		}
		return kindText, ""
	default:
		if strings.TrimSpace(msg.GetMessage()) != "" {
			return kindText, ""
		}
		return kindNoText, ""
	}
}

func documentKind(doc *mtproto.Document) (kind, string) {
	if doc == nil {
		return kindDoc, ""
	}
	var sticker string
	video := false
	round := false
	voice := false
	animated := false
	for _, a := range doc.GetAttributes() {
		switch a.GetPredicateName() {
		case mtproto.Predicate_documentAttributeSticker:
			sticker = a.GetAlt()
		case mtproto.Predicate_documentAttributeVideo:
			video = true
			round = a.GetRoundMessage()
		case mtproto.Predicate_documentAttributeAudio:
			voice = a.GetVoice()
		case mtproto.Predicate_documentAttributeAnimated:
			animated = true
		}
	}
	switch {
	case sticker != "" || hasPredicate(doc, mtproto.Predicate_documentAttributeSticker):
		if sticker == "" {
			sticker = " "
		}
		return kindSticker, sticker
	case round:
		return kindRound, ""
	case animated:
		return kindGIF, ""
	case voice:
		return kindVoice, ""
	case video:
		return kindVideo, ""
	default:
		return kindDoc, ""
	}
}

func hasPredicate(doc *mtproto.Document, name string) bool {
	for _, a := range doc.GetAttributes() {
		if a.GetPredicateName() == name {
			return true
		}
	}
	return false
}

func locKey(peerType int32, k kind, text string) string {
	prefix := ""
	switch peerType {
	case mtproto.PEER_CHAT:
		prefix = "CHAT_"
	case mtproto.PEER_CHANNEL:
		prefix = "CHANNEL_"
	}
	name := "MESSAGE_NOTEXT"
	switch k {
	case kindText:
		if text != "" {
			name = "MESSAGE_TEXT"
		}
	case kindPhoto:
		name = "MESSAGE_PHOTO"
	case kindVideo:
		name = "MESSAGE_VIDEO"
	case kindGIF:
		name = "MESSAGE_GIF"
	case kindRound:
		name = "MESSAGE_ROUND"
	case kindVoice:
		name = "MESSAGE_AUDIO"
	case kindSticker:
		name = "MESSAGE_STICKER"
	case kindDoc:
		name = "MESSAGE_DOC"
	case kindGeo:
		name = "MESSAGE_GEO"
	case kindGeoLive:
		name = "MESSAGE_GEOLIVE"
	case kindContact:
		name = "MESSAGE_CONTACT"
	case kindPoll:
		name = "MESSAGE_POLL"
	}
	return prefix + name
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

// Args are the loc-args for key, in the order documented at
// https://core.telegram.org/api/push-updates
func (m Incoming) Args(preview bool) []string {
	author := m.SenderName
	if author == "" {
		author = "User"
	}
	title := m.ChatTitle
	if title == "" {
		title = "Chat"
	}
	body := m.Text
	if !preview {
		body = ""
	}
	switch {
	case m.ChannelID != 0:
		if strings.HasSuffix(m.LocKey, "MESSAGE_TEXT") && body != "" {
			return []string{title, body}
		}
		return []string{title}
	case m.ChatID != 0:
		if strings.HasSuffix(m.LocKey, "MESSAGE_TEXT") && body != "" {
			return []string{author, title, body}
		}
		if strings.HasSuffix(m.LocKey, "MESSAGE_STICKER") && m.Sticker != "" {
			return []string{author, title, m.Sticker}
		}
		return []string{author, title}
	default:
		if strings.HasSuffix(m.LocKey, "MESSAGE_TEXT") && body != "" {
			return []string{author, body}
		}
		if strings.HasSuffix(m.LocKey, "MESSAGE_STICKER") && m.Sticker != "" {
			return []string{author, m.Sticker}
		}
		return []string{author}
	}
}

// LocKeyFor returns the key actually sent. Hidden previews use the NOTEXT key
// so the message body is not placed in loc-args.
func (m Incoming) LocKeyFor(preview bool) string {
	if preview || !strings.HasSuffix(m.LocKey, "MESSAGE_TEXT") {
		return m.LocKey
	}
	return strings.TrimSuffix(m.LocKey, "MESSAGE_TEXT") + "MESSAGE_NOTEXT"
}

func globalPeerType(peerType int32) int32 {
	switch peerType {
	case mtproto.PEER_USER:
		return mtproto.PEER_USERS
	case mtproto.PEER_CHAT:
		return mtproto.PEER_CHATS
	case mtproto.PEER_CHANNEL:
		return mtproto.PEER_BROADCASTS
	default:
		return 0
	}
}
