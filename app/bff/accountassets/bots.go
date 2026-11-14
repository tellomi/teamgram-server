package accountassets

import (
	"fmt"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type botLink struct {
	id         int64
	access     int64
	conn       string
	recipients *mtproto.BusinessBotRecipients
	rights     *mtproto.BusinessBotRights
	date       int32
}

// GetConnectedBots implements account.getConnectedBots.
// https://core.telegram.org/method/account.getConnectedBots
func GetConnectedBots(self int64) *mtproto.Account_ConnectedBots {
	return shared().connected(self)
}

func (s *store) connected(self int64) *mtproto.Account_ConnectedBots {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[self]
	bots := []*mtproto.ConnectedBot{}
	users := []*mtproto.User{}
	if u != nil {
		for _, id := range u.botOrder {
			link := u.bots[id]
			if link == nil {
				continue
			}
			bots = append(bots, link.view())
			users = append(users, botUser(link.id, link.access))
		}
	}
	return mtproto.MakeTLAccountConnectedBots(&mtproto.Account_ConnectedBots{
		ConnectedBots: bots,
		Users:         users,
	}).To_Account_ConnectedBots()
}

// UpdateConnectedBot implements account.updateConnectedBot.
// This process has no Telegram bot directory, so a positive inputUser other
// than the caller is the business bot the user asked to connect. inputUserSelf
// and an empty user are BOT_BUSINESS_MISSING.
// https://core.telegram.org/method/account.updateConnectedBot
func UpdateConnectedBot(self int64, in *mtproto.TLAccountUpdateConnectedBot) (*mtproto.Updates, error) {
	return shared().updateBot(self, in)
}

func (s *store) updateBot(self int64, in *mtproto.TLAccountUpdateConnectedBot) (*mtproto.Updates, error) {
	if in == nil {
		return nil, mtproto.ErrBotBusinessMissing
	}
	botID, access, err := botIDOf(self, in.GetBot())
	if err != nil {
		return nil, err
	}
	if IsBusinessBot != nil && !IsBusinessBot(botID) {
		return nil, mtproto.ErrBotBusinessMissing
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.user(self)
	if in.GetDeleted() {
		if _, ok := u.bots[botID]; ok {
			delete(u.bots, botID)
			u.botOrder = dropID(u.botOrder, botID)
		}
		return botUpdates(botID, access), nil
	}
	recipients, err := recipientsOf(self, in.GetRecipients_INPUTBUSINESSBOTRECIPIENTS())
	if err != nil {
		return nil, err
	}
	rights := copyRights(in.GetRights())
	link := u.bots[botID]
	if link == nil {
		link = &botLink{
			id: botID, access: access, date: int32(time.Now().Unix()),
			conn: connectionID(self, botID),
		}
		u.bots[botID] = link
		u.botOrder = append(u.botOrder, botID)
	}
	if link.conn == "" {
		link.conn = connectionID(self, botID)
	}
	if access != 0 {
		link.access = access
	}
	link.recipients = recipients
	link.rights = rights
	return botUpdates(botID, link.access), nil
}

// ToggleConnectedBotPaused implements account.toggleConnectedBotPaused.
// https://core.telegram.org/method/account.toggleConnectedBotPaused
// IsBusinessBot reports whether id is a business bot (user.bot_business).
// Nil accepts every connected bot. The bff installs a checker when the user
// service is available; tests leave it nil.
var IsBusinessBot func(id int64) bool

func ToggleConnectedBotPaused(self int64, peer *mtproto.InputPeer, paused bool) (*mtproto.Bool, error) {
	key, err := peerKeyOf(self, peer)
	if err != nil {
		return nil, err
	}
	// The connected bot is paused inside one private chat.
	if key.kind != "user" || key.id == 0 || key.id == self {
		return nil, mtproto.ErrPeerIdInvalid
	}
	s := shared()
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[self]
	if u == nil || len(u.bots) == 0 {
		return nil, mtproto.ErrBotNotConnectedYet
	}
	if !hasBusinessBot(u) {
		return nil, mtproto.ErrBotBusinessMissing
	}
	u.paused[key] = paused
	return mtproto.BoolTrue, nil
}

func hasBusinessBot(u *userState) bool {
	if u == nil {
		return false
	}
	for _, id := range u.botOrder {
		link := u.bots[id]
		if link == nil {
			continue
		}
		if IsBusinessBot != nil && !IsBusinessBot(link.id) {
			continue
		}
		return true
	}
	return false
}

// DisablePeerConnectedBot implements account.disablePeerConnectedBot.
// https://core.telegram.org/method/account.disablePeerConnectedBot
func DisablePeerConnectedBot(self int64, peer *mtproto.InputPeer) (*mtproto.Bool, error) {
	key, err := peerKeyOf(self, peer)
	if err != nil {
		return nil, err
	}
	s := shared()
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[self]
	if u == nil || len(u.bots) == 0 {
		return nil, mtproto.ErrBotNotConnectedYet
	}
	if _, ok := u.disabled[key]; ok {
		return nil, mtproto.ErrBotAlreadyDisabled
	}
	u.disabled[key] = struct{}{}
	if key.kind == "user" {
		for _, link := range u.bots {
			excludeUser(link.recipients, key.id)
		}
	}
	return mtproto.BoolTrue, nil
}

func botIDOf(self int64, in *mtproto.InputUser) (int64, int64, error) {
	if in == nil {
		return 0, 0, mtproto.ErrBotBusinessMissing
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputUser:
		if in.GetUserId() == 0 || in.GetUserId() == self {
			return 0, 0, mtproto.ErrBotBusinessMissing
		}
		access := in.GetAccessHash()
		if access == 0 {
			access = accessOf(in.GetUserId())
		}
		return in.GetUserId(), access, nil
	case mtproto.Predicate_inputUserFromMessage:
		if in.GetUserId() == 0 || in.GetUserId() == self {
			return 0, 0, mtproto.ErrBotBusinessMissing
		}
		return in.GetUserId(), accessOf(in.GetUserId()), nil
	default:
		return 0, 0, mtproto.ErrBotBusinessMissing
	}
}

func recipientsOf(self int64, in *mtproto.InputBusinessBotRecipients) (*mtproto.BusinessBotRecipients, error) {
	if in == nil || in.GetPredicateName() != mtproto.Predicate_inputBusinessBotRecipients {
		return nil, mtproto.ErrBusinessRecipientsEmpty
	}
	users := userIDs(self, in.GetUsers())
	exclude := userIDs(self, in.GetExcludeUsers())
	if !in.GetExistingChats() && !in.GetNewChats() && !in.GetContacts() && !in.GetNonContacts() && len(users) == 0 {
		return nil, mtproto.ErrBusinessRecipientsEmpty
	}
	return mtproto.MakeTLBusinessBotRecipients(&mtproto.BusinessBotRecipients{
		ExistingChats:   in.GetExistingChats(),
		NewChats:        in.GetNewChats(),
		Contacts:        in.GetContacts(),
		NonContacts:     in.GetNonContacts(),
		ExcludeSelected: in.GetExcludeSelected(),
		Users:           users,
		ExcludeUsers:    exclude,
	}).To_BusinessBotRecipients(), nil
}

func userIDs(self int64, in []*mtproto.InputUser) []int64 {
	var ids []int64
	seen := map[int64]bool{}
	for _, u := range in {
		if u == nil {
			continue
		}
		id := mtproto.ToUserIdByInput(self, u)
		if id <= 0 || id == self || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func copyRights(in *mtproto.BusinessBotRights) *mtproto.BusinessBotRights {
	if in == nil {
		return mtproto.MakeTLBusinessBotRights(&mtproto.BusinessBotRights{}).To_BusinessBotRights()
	}
	c := proto.Clone(in).(*mtproto.BusinessBotRights)
	return mtproto.MakeTLBusinessBotRights(c).To_BusinessBotRights()
}

func excludeUser(r *mtproto.BusinessBotRecipients, id int64) {
	if r == nil || id == 0 {
		return
	}
	kept := make([]int64, 0, len(r.Users))
	for _, u := range r.Users {
		if u != id {
			kept = append(kept, u)
		}
	}
	r.Users = kept
	for _, u := range r.ExcludeUsers {
		if u == id {
			return
		}
	}
	r.ExcludeUsers = append(r.ExcludeUsers, id)
}

func (b *botLink) view() *mtproto.ConnectedBot {
	return mtproto.MakeTLConnectedBot(&mtproto.ConnectedBot{
		BotId:                            b.id,
		Recipients_BUSINESSBOTRECIPIENTS: cloneRecipients(b.recipients),
		Rights:                           copyRights(b.rights),
		Date:                             wrapperspb.Int32(b.date),
	}).To_ConnectedBot()
}

func cloneRecipients(in *mtproto.BusinessBotRecipients) *mtproto.BusinessBotRecipients {
	if in == nil {
		return mtproto.MakeTLBusinessBotRecipients(&mtproto.BusinessBotRecipients{}).To_BusinessBotRecipients()
	}
	c := proto.Clone(in).(*mtproto.BusinessBotRecipients)
	return mtproto.MakeTLBusinessBotRecipients(c).To_BusinessBotRecipients()
}

func botUser(id, access int64) *mtproto.User {
	// bot and bot_info_version share flags.14. A bot user without the int
	// makes official clients fail to parse account.connectedBots.
	return mtproto.MakeTLUser(&mtproto.User{
		Bot:            true,
		BotBusiness:    true,
		Id:             id,
		AccessHash:     wrapperspb.Int64(access),
		FirstName:      wrapperspb.String("Bot"),
		BotInfoVersion: wrapperspb.Int32(0),
	}).To_User()
}

func connectionID(user, bot int64) string {
	return fmt.Sprintf("%x-%x", user, bot)
}

func manageURL(link *botLink) string {
	if link == nil {
		return ""
	}
	return fmt.Sprintf("https://t.me/bot%d?start=biz_%s", link.id, link.conn)
}

// ViewerFacts is the viewer's relationship to the business account.
// Contact means the viewer is in that account's contact list.
// ExistingChat means a private dialog already exists between them.
// ContactOK and ChatOK are false when that lookup failed. A selected
// category that cannot be evaluated does not match, so the bot id is not shown.
type ViewerFacts struct {
	Contact      bool
	ExistingChat bool
	ContactOK    bool
	ChatOK       bool
}

// ViewerFactsOf reports the relationship. Nil means the categories cannot
// be evaluated. The bff installs this when the user and dialog services exist.
var ViewerFactsOf func(owner, viewer int64) ViewerFacts

// ApplyBusinessBar fills the business-bot fields the viewer sees on owner's
// peerSettings. business_bot_id and business_bot_manage_url share flags.13,
// so both are set together. The returned user is the bot, for the users vector.
// https://core.telegram.org/constructor/peerSettings
// https://core.telegram.org/constructor/businessBotRecipients
func ApplyBusinessBar(owner, viewer int64, settings *mtproto.PeerSettings) *mtproto.User {
	if settings == nil || owner == 0 || viewer == 0 || owner == viewer {
		return nil
	}
	var facts ViewerFacts
	if ViewerFactsOf != nil {
		facts = ViewerFactsOf(owner, viewer)
	}
	s := shared()
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[owner]
	if u == nil {
		return nil
	}
	key := peerKey{kind: "user", id: viewer}
	if _, off := u.disabled[key]; off {
		return nil
	}
	for _, id := range u.botOrder {
		link := u.bots[id]
		if link == nil || !covers(link, viewer, facts) {
			continue
		}
		if IsBusinessBot != nil && !IsBusinessBot(link.id) {
			continue
		}
		paused := u.paused[key]
		settings.BusinessBotId = wrapperspb.Int64(link.id)
		settings.BusinessBotManageUrl = wrapperspb.String(manageURL(link))
		settings.BusinessBotPaused = paused
		settings.BusinessBotCanReply = link.rights.GetReply() && !paused
		return botUser(link.id, link.access)
	}
	return nil
}

// covers reports whether viewer is in the business bot's recipient set.
// Categories are OR-ed and use the viewer's real relationship: contacts,
// non-contacts, an existing private chat, or a new private chat.
// users adds people when exclude_selected is unset, and removes them when
// it is set. exclude_users never sees the bar.
func covers(link *botLink, viewer int64, facts ViewerFacts) bool {
	if link == nil || link.recipients == nil || viewer == 0 {
		return false
	}
	r := link.recipients
	for _, id := range r.ExcludeUsers {
		if id == viewer {
			return false
		}
	}
	listed := false
	for _, id := range r.Users {
		if id == viewer {
			listed = true
			break
		}
	}
	if r.ExcludeSelected {
		if listed {
			return false
		}
		return recipientCategory(r, facts)
	}
	if listed {
		return true
	}
	return recipientCategory(r, facts)
}

func recipientCategory(r *mtproto.BusinessBotRecipients, facts ViewerFacts) bool {
	if r.GetContacts() && facts.ContactOK && facts.Contact {
		return true
	}
	if r.GetNonContacts() && facts.ContactOK && !facts.Contact {
		return true
	}
	if r.GetExistingChats() && facts.ChatOK && facts.ExistingChat {
		return true
	}
	if r.GetNewChats() && facts.ChatOK && !facts.ExistingChat {
		return true
	}
	return false
}

// BotBusinessConnection implements account.getBotBusinessConnection.
// The caller may be the connected bot or the user who connected it.
// https://core.telegram.org/method/account.getBotBusinessConnection
func BotBusinessConnection(self int64, id string) (*mtproto.Updates, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	id = stringsTrim(id)
	if id == "" {
		return nil, mtproto.ErrConnectionIdInvalid
	}
	s := shared()
	s.mu.Lock()
	defer s.mu.Unlock()
	for userID, u := range s.users {
		if u == nil {
			continue
		}
		for _, link := range u.bots {
			if link == nil || link.conn != id {
				continue
			}
			if self != userID && self != link.id {
				return nil, mtproto.ErrConnectionIdInvalid
			}
			return businessUpdates(userID, link), nil
		}
	}
	return nil, mtproto.ErrConnectionIdInvalid
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func businessUpdates(userID int64, link *botLink) *mtproto.Updates {
	rights := copyRights(link.rights)
	conn := mtproto.MakeTLBotBusinessConnection(&mtproto.BotBusinessConnection{
		ConnectionId: link.conn,
		UserId:       userID,
		DcId:         1,
		Date:         link.date,
		Rights:       rights,
		CanReply:     rights.GetReply(),
	}).To_BotBusinessConnection()
	upd := mtproto.MakeTLUpdateBotBusinessConnect(&mtproto.Update{
		Connection: conn,
		Qts:        1,
	}).To_Update()
	user := mtproto.MakeTLUser(&mtproto.User{Id: userID, Min: true}).To_User()
	bot := botUser(link.id, link.access)
	return mtproto.MakeUpdatesByUpdatesUsers([]*mtproto.User{user, bot}, upd)
}

func botUpdates(id, access int64) *mtproto.Updates {
	user := botUser(id, access)
	upd := mtproto.MakeTLUpdateUser(&mtproto.Update{UserId: id}).To_Update()
	return mtproto.MakeUpdatesByUpdatesUsers([]*mtproto.User{user}, upd)
}

func dropID(ids []int64, id int64) []int64 {
	out := ids[:0]
	for _, v := range ids {
		if v != id {
			out = append(out, v)
		}
	}
	return out
}

func peerKeyOf(self int64, in *mtproto.InputPeer) (peerKey, error) {
	if in == nil {
		return peerKey{}, mtproto.ErrPeerIdInvalid
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		if self == 0 {
			return peerKey{}, mtproto.ErrPeerIdInvalid
		}
		return peerKey{kind: "user", id: self}, nil
	case mtproto.Predicate_inputPeerUser:
		if in.GetUserId() == 0 {
			return peerKey{}, mtproto.ErrPeerIdInvalid
		}
		return peerKey{kind: "user", id: in.GetUserId()}, nil
	case mtproto.Predicate_inputPeerChat:
		if in.GetChatId() == 0 {
			return peerKey{}, mtproto.ErrPeerIdInvalid
		}
		return peerKey{kind: "chat", id: in.GetChatId()}, nil
	case mtproto.Predicate_inputPeerChannel:
		if in.GetChannelId() == 0 {
			return peerKey{}, mtproto.ErrPeerIdInvalid
		}
		return peerKey{kind: "channel", id: in.GetChannelId()}, nil
	default:
		return peerKey{}, mtproto.ErrPeerIdInvalid
	}
}
