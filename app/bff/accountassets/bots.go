package accountassets

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type botLink struct {
	id         int64
	access     int64
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
		link = &botLink{id: botID, access: access, date: int32(time.Now().Unix())}
		u.bots[botID] = link
		u.botOrder = append(u.botOrder, botID)
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
func ToggleConnectedBotPaused(self int64, peer *mtproto.InputPeer, paused bool) (*mtproto.Bool, error) {
	key, err := peerKeyOf(self, peer)
	if err != nil {
		return nil, err
	}
	s := shared()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.user(self).paused[key] = paused
	return mtproto.BoolTrue, nil
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
		ExcludeSelected: in.GetExcludeSelected() || len(exclude) > 0,
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
			r.ExcludeSelected = true
			return
		}
	}
	r.ExcludeUsers = append(r.ExcludeUsers, id)
	r.ExcludeSelected = true
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
	return mtproto.MakeTLUser(&mtproto.User{
		Bot:         true,
		BotBusiness: true,
		Id:          id,
		AccessHash:  wrapperspb.Int64(access),
		FirstName:   wrapperspb.String("Bot"),
	}).To_User()
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
