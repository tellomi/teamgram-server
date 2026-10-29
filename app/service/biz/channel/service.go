package channel

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"regexp"
	"strings"
	"sync"
	"time"

	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/reaction"
)

// 3–32 characters, [A-Za-z0-9_], starting with a letter. A trailing
// underscore is rejected by validChannelUsername.
var usernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{2,31}$`)

func validChannelUsername(username string) bool {
	return usernamePattern.MatchString(username) && !strings.HasSuffix(username, "_")
}

func allAdminRights() int32 {
	return int32(mtproto.MakeChatAdminRightsHelper(&mtproto.ChatAdminRights{
		ChangeInfo:     true,
		PostMessages:   true,
		EditMessages:   true,
		DeleteMessages: true,
		BanUsers:       true,
		InviteUsers:    true,
		PinMessages:    true,
		AddAdmins:      true,
		ManageCall:     true,
		Other:          true,
		ManageTopics:   true,
		ManageRanks:    true,
		PostStories:    true,
		EditStories:    true,
		DeleteStories:  true,
	}))
}

func defaultChannelBanned() int64 {
	rights := mtproto.MakeDefaultBannedRights()
	rights.ChangeInfo = true
	rights.PinMessages = true
	return int64(rights.ToBannedRights())
}

// Hooks fan out channel updates and dialog rows. Both are optional.
type Hooks struct {
	Push   func(ctx context.Context, userID int64, excludeAuthKeys []int64, updates *mtproto.Updates)
	Dialog func(ctx context.Context, userID, channelID int64, top, readInbox, readOutbox, unread int32, date int64)
}

// Service is the channel and supergroup implementation.
type Service struct {
	store     Store
	reactions reaction.Store

	mu     sync.Mutex
	push   Hooks
	loaded bool
}

func New(store Store) *Service {
	return &Service{store: store, reactions: reaction.NewMemory()}
}

func (s *Service) SetHooks(h Hooks) {
	s.mu.Lock()
	s.push = h
	s.loaded = true
	s.mu.Unlock()
}

func (s *Service) hooks() Hooks {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.push
}

type CreateRequest struct {
	CreatorID int64
	ActorAuth int64
	Title     string
	About     string
	Broadcast bool
	Megagroup bool
	Forum     bool
	TtlPeriod int32
	Address   string
}

func (s *Service) Create(ctx context.Context, in CreateRequest) (*mtproto.Updates, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, mtproto.ErrChatTitleEmpty
	}
	if len(title) > 255 {
		title = title[:255]
	}
	about := in.About
	if len(about) > 255 {
		about = about[:255]
	}
	broadcast := in.Broadcast
	megagroup := in.Megagroup
	if !broadcast && !megagroup {
		broadcast = true
	}
	if broadcast && megagroup {
		megagroup = false
	}
	now := int32(time.Now().Unix())
	ch := &Channel{
		AccessHash:        randInt63(),
		CreatorID:         in.CreatorID,
		Title:             title,
		About:             about,
		Broadcast:         broadcast,
		Megagroup:         megagroup,
		Forum:             in.Forum && megagroup,
		Date:              now,
		TtlPeriod:         in.TtlPeriod,
		Address:           in.Address,
		DefaultBanned:     defaultChannelBanned(),
		ParticipantsCount: 1,
		Version:           1,
	}
	creator := &Participant{
		UserID:      in.CreatorID,
		Role:        RoleCreator,
		State:       StateNormal,
		AdminRights: allAdminRights(),
		Date:        now,
	}
	if err := s.store.CreateChannel(ctx, ch, creator); err != nil {
		return nil, err
	}
	msg := serviceMessage(in.CreatorID, ch.ID, mtproto.MakeMessageActionChannelCreate(title))
	if _, _, err := s.appendMessage(ctx, ch.ID, in.CreatorID, 0, msg); err != nil {
		return nil, err
	}
	return s.actorUpdates(ctx, in.CreatorID, in.ActorAuth, ch.ID, KindNew)
}

func (s *Service) GetChannels(ctx context.Context, self int64, ids []*mtproto.InputChannel) (*mtproto.Messages_Chats, error) {
	chats := make([]*mtproto.Chat, 0, len(ids))
	for _, in := range ids {
		ch, err := s.lookup(ctx, in)
		if err != nil || ch == nil {
			chats = append(chats, mtproto.MakeTLChannelForbidden(&mtproto.Chat{}).To_Chat())
			continue
		}
		chats = append(chats, s.ChatFor(ctx, self, ch))
	}
	return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: chats}).To_Messages_Chats(), nil
}

func (s *Service) GetFullChannel(ctx context.Context, self int64, in *mtproto.InputChannel) (*mtproto.Messages_ChatFull, error) {
	ch, p, err := s.requireView(ctx, self, in)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesChatFull(&mtproto.Messages_ChatFull{
		FullChat: s.fullChat(ctx, self, ch, p),
		Chats:    []*mtproto.Chat{s.chatFor(ctx, self, ch, p)},
		Users:    []*mtproto.User{},
	}).To_Messages_ChatFull(), nil
}

func (s *Service) GetParticipants(ctx context.Context, self int64, in *mtproto.InputChannel, filter *mtproto.ChannelParticipantsFilter, offset, limit int32, hash int64) (*mtproto.Channels_ChannelParticipants, error) {
	ch, p, err := s.requireView(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if p == nil || !p.InChannel() {
		if !ch.Public() {
			return nil, mtproto.ErrChannelPrivate
		}
	}
	if feat := s.features(ctx, ch.ID); feat.ParticipantsHidden && (p == nil || !p.IsAdmin()) {
		return mtproto.MakeTLChannelsChannelParticipants(&mtproto.Channels_ChannelParticipants{
			Count:        ch.ParticipantsCount,
			Participants: []*mtproto.ChannelParticipant{},
			Chats:        []*mtproto.Chat{s.chatFor(ctx, self, ch, p)},
			Users:        []*mtproto.User{},
		}).To_Channels_ChannelParticipants(), nil
	}
	all, err := s.store.ListParticipants(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	q := ""
	name := mtproto.Predicate_channelParticipantsRecent
	if filter != nil {
		name = filter.PredicateName
		q = filter.Q_STRING
		if filter.Q_FLAGSTRING != nil {
			q = filter.Q_FLAGSTRING.Value
		}
	}
	matched := filterParticipants(all, name, strings.ToLower(q))
	ids := make([]int64, len(matched))
	for i, part := range matched {
		ids[i] = part.UserID
	}
	if hash != 0 && offset == 0 && hash == hashParticipants(ids) {
		return mtproto.MakeTLChannelsChannelParticipantsNotModified(&mtproto.Channels_ChannelParticipants{}).To_Channels_ChannelParticipants(), nil
	}
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	if int(offset) > len(matched) {
		offset = int32(len(matched))
	}
	end := int(offset + limit)
	if end > len(matched) {
		end = len(matched)
	}
	page := matched[offset:end]
	parts := make([]*mtproto.ChannelParticipant, 0, len(page))
	for _, part := range page {
		parts = append(parts, participantTL(self, part))
	}
	return mtproto.MakeTLChannelsChannelParticipants(&mtproto.Channels_ChannelParticipants{
		Count:        int32(len(matched)),
		Participants: parts,
		Chats:        []*mtproto.Chat{s.chatFor(ctx, self, ch, p)},
		Users:        []*mtproto.User{},
	}).To_Channels_ChannelParticipants(), nil
}

func (s *Service) GetParticipant(ctx context.Context, self int64, in *mtproto.InputChannel, user *mtproto.InputPeer) (*mtproto.Channels_ChannelParticipant, error) {
	ch, actor, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	userID := self
	if user != nil && user.PredicateName == mtproto.Predicate_inputPeerUser {
		userID = user.UserId
	} else if user != nil && user.PredicateName == mtproto.Predicate_inputPeerSelf {
		userID = self
	}
	p, err := s.store.GetParticipant(ctx, ch.ID, userID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, mtproto.ErrUserNotParticipant
	}
	_ = actor
	return mtproto.MakeTLChannelsChannelParticipant(&mtproto.Channels_ChannelParticipant{
		Participant: participantTL(self, p),
		Chats:       []*mtproto.Chat{s.ChatFor(ctx, self, ch)},
		Users:       []*mtproto.User{},
	}).To_Channels_ChannelParticipant(), nil
}

func (s *Service) Invite(ctx context.Context, actor, actorAuth int64, in *mtproto.InputChannel, users []int64, bots map[int64]bool) (*mtproto.Messages_InvitedUsers, error) {
	ch, p, err := s.requireMember(ctx, actor, in)
	if err != nil {
		return nil, err
	}
	if !s.canInvite(ch, p) {
		return nil, mtproto.ErrChatAdminRequired
	}
	if len(users) > 200 {
		return nil, mtproto.ErrUsersTooMuch
	}
	var added []int64
	var missing []*mtproto.MissingInvitee
	now := int32(time.Now().Unix())
	for _, id := range users {
		if id == 0 {
			continue
		}
		cur, _ := s.store.GetParticipant(ctx, ch.ID, id)
		if cur != nil && cur.InChannel() {
			continue
		}
		if cur != nil && cur.State == StateKicked {
			missing = append(missing, missingInvitee(id))
			continue
		}
		np := &Participant{
			ChannelID: ch.ID,
			UserID:    id,
			Role:      RoleMember,
			State:     StateNormal,
			InviterID: actor,
			IsBot:     bots[id],
			Date:      now,
		}
		if cur != nil && cur.Role == RoleAdmin {
			np.Role = RoleAdmin
			np.AdminRights = cur.AdminRights
			np.Rank = cur.Rank
		}
		if err := s.store.PutParticipant(ctx, np); err != nil {
			return nil, err
		}
		added = append(added, id)
		// Joining sees history up to now. Keep any outbox cursor already
		// earned; do not pretend the current top was read by someone else.
		st, _ := s.store.GetRead(ctx, ch.ID, id)
		if st == nil {
			st = &ReadState{ChannelID: ch.ID, UserID: id}
		}
		st.ReadInboxMaxID = ch.TopMessage
		_ = s.store.SaveRead(ctx, st)
	}
	if len(added) > 0 {
		if _, err = s.refreshCount(ctx, ch.ID); err != nil {
			return nil, err
		}
		msg := serviceMessage(actor, ch.ID, mtproto.MakeMessageActionChatAddUser(added...))
		if _, _, err = s.appendMessage(ctx, ch.ID, actor, 0, msg); err != nil {
			return nil, err
		}
	}
	updates, err := s.actorUpdates(ctx, actor, actorAuth, ch.ID, KindNew)
	if err != nil {
		return nil, err
	}
	if len(added) == 0 {
		updates = s.pack(ctx, actor, ch.ID, channelUpdate(ch.ID))
	}
	if missing == nil {
		missing = []*mtproto.MissingInvitee{}
	}
	return mtproto.MakeTLMessagesInvitedUsers(&mtproto.Messages_InvitedUsers{
		Updates:         updates,
		MissingInvitees: missing,
	}).To_Messages_InvitedUsers(), nil
}

func (s *Service) Join(ctx context.Context, user, actorAuth int64, in *mtproto.InputChannel) (*mtproto.Updates, error) {
	ch, err := s.lookup(ctx, in)
	if err != nil {
		return nil, err
	}
	p, _ := s.store.GetParticipant(ctx, ch.ID, user)
	if p != nil && p.InChannel() {
		return s.pack(ctx, user, ch.ID, channelUpdate(ch.ID)), nil
	}
	if p != nil && p.State == StateKicked {
		return nil, mtproto.ErrUserBannedInChannel
	}
	if feat := s.features(ctx, ch.ID); feat.JoinRequest && (p == nil || !p.InChannel()) {
		feat.JoinRequests = append(feat.JoinRequests, JoinReq{UserID: user, Date: int32(time.Now().Unix())})
		if err := s.saveFeatures(ctx, ch.ID, feat); err != nil {
			return nil, err
		}
		return s.pack(ctx, user, ch.ID, channelUpdate(ch.ID)), nil
	}
	if !ch.Public() {
		return nil, mtproto.ErrChannelPrivate
	}
	now := int32(time.Now().Unix())
	np := &Participant{
		ChannelID: ch.ID,
		UserID:    user,
		Role:      RoleMember,
		State:     StateNormal,
		Date:      now,
	}
	if p != nil {
		np.InviterID = p.InviterID
		np.IsBot = p.IsBot
	}
	if err := s.store.PutParticipant(ctx, np); err != nil {
		return nil, err
	}
	if _, err = s.refreshCount(ctx, ch.ID); err != nil {
		return nil, err
	}
	_ = s.store.SaveRead(ctx, &ReadState{ChannelID: ch.ID, UserID: user, ReadInboxMaxID: ch.TopMessage})
	msg := serviceMessage(user, ch.ID, mtproto.MakeMessageActionChatAddUser(user))
	if _, _, err = s.appendMessage(ctx, ch.ID, user, 0, msg); err != nil {
		return nil, err
	}
	return s.actorUpdates(ctx, user, actorAuth, ch.ID, KindNew)
}

func (s *Service) Leave(ctx context.Context, user, actorAuth int64, in *mtproto.InputChannel) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, user, in)
	if err != nil {
		return nil, err
	}
	if p.IsCreator() {
		return nil, mtproto.ErrUserCreator
	}
	p.State = StateLeft
	if err := s.store.PutParticipant(ctx, p); err != nil {
		return nil, err
	}
	if _, err = s.refreshCount(ctx, ch.ID); err != nil {
		return nil, err
	}
	msg := serviceMessage(user, ch.ID, mtproto.MakeMessageActionChatDeleteUser(user))
	if _, _, err = s.appendMessage(ctx, ch.ID, user, 0, msg); err != nil {
		return nil, err
	}
	return s.actorUpdates(ctx, user, actorAuth, ch.ID, KindNew)
}

func (s *Service) EditAdmin(ctx context.Context, actor, actorAuth int64, in *mtproto.InputChannel, userID int64, rights *mtproto.ChatAdminRights, rank string) (*mtproto.Updates, error) {
	ch, actorP, err := s.requireMember(ctx, actor, in)
	if err != nil {
		return nil, err
	}
	if !actorP.IsCreator() && !actorP.adminRights().CanAddAdmins() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if len(rank) > 16 {
		return nil, mtproto.ErrAdminRankInvalid
	}
	target, err := s.store.GetParticipant(ctx, ch.ID, userID)
	if err != nil {
		return nil, err
	}
	if target == nil || !target.InChannel() {
		return nil, mtproto.ErrUserNotParticipant
	}
	if target.IsCreator() {
		return nil, mtproto.ErrUserCreator
	}
	want := int32(0)
	if rights != nil {
		want = int32(mtproto.MakeChatAdminRightsHelper(rights))
	}
	if !actorP.IsCreator() && want&^actorP.AdminRights != 0 {
		return nil, mtproto.ErrRightForbidden
	}
	if want == 0 {
		target.Role = RoleMember
		target.AdminRights = 0
		target.Rank = ""
	} else {
		target.Role = RoleAdmin
		target.AdminRights = want
		target.Rank = rank
		if target.InviterID == 0 {
			target.InviterID = actor
		}
	}
	if err := s.store.PutParticipant(ctx, target); err != nil {
		return nil, err
	}
	return s.pack(ctx, actor, ch.ID, channelUpdate(ch.ID)), nil
}

func (s *Service) EditBanned(ctx context.Context, actor, actorAuth int64, in *mtproto.InputChannel, userID int64, rights *mtproto.ChatBannedRights) (*mtproto.Updates, error) {
	ch, actorP, err := s.requireMember(ctx, actor, in)
	if err != nil {
		return nil, err
	}
	if !actorP.IsCreator() && !actorP.adminRights().CanBanUsers() {
		return nil, mtproto.ErrChatAdminRequired
	}
	target, err := s.store.GetParticipant(ctx, ch.ID, userID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		target = &Participant{ChannelID: ch.ID, UserID: userID, Date: int32(time.Now().Unix())}
	}
	if target.IsCreator() {
		return nil, mtproto.ErrUserCreator
	}
	if target.IsAdmin() && !actorP.IsCreator() {
		return nil, mtproto.ErrRightForbidden
	}
	kicked := false
	if rights == nil || rights.NoBanRights() {
		target.Role = RoleMember
		target.BannedRights = 0
		if target.State == StateKicked {
			target.State = StateLeft
		}
	} else {
		target.Role = RoleBanned
		target.BannedRights = int64(rights.ToBannedRights())
		target.InviterID = actor
		if rights.IsKick() {
			target.State = StateKicked
			kicked = true
		}
	}
	if err := s.store.PutParticipant(ctx, target); err != nil {
		return nil, err
	}
	if _, err = s.refreshCount(ctx, ch.ID); err != nil {
		return nil, err
	}
	if kicked {
		msg := serviceMessage(actor, ch.ID, mtproto.MakeMessageActionChatDeleteUser(userID))
		if _, _, err = s.appendMessage(ctx, ch.ID, actor, 0, msg); err != nil {
			return nil, err
		}
		return s.actorUpdates(ctx, actor, actorAuth, ch.ID, KindNew)
	}
	return s.pack(ctx, actor, ch.ID, channelUpdate(ch.ID)), nil
}

func (s *Service) EditTitle(ctx context.Context, actor, actorAuth int64, in *mtproto.InputChannel, title string) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, actor, in)
	if err != nil {
		return nil, err
	}
	if !s.canChangeInfo(ch, p) {
		return nil, mtproto.ErrChatAdminRequired
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, mtproto.ErrChatTitleEmpty
	}
	if len(title) > 255 {
		title = title[:255]
	}
	prevTitle := ch.Title
	ch.Title = title
	ch.Version++
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	msg := serviceMessage(actor, ch.ID, mtproto.MakeMessageActionChatEditTitle(title))
	if _, _, err = s.appendMessage(ctx, ch.ID, actor, 0, msg); err != nil {
		return nil, err
	}
	s.logAdmin(ctx, ch.ID, actor, mtproto.MakeTLChannelAdminLogEventActionChangeTitle(&mtproto.ChannelAdminLogEventAction{
		PrevValue_STRING: prevTitle,
		NewValue_STRING:  title,
	}).To_ChannelAdminLogEventAction())
	return s.actorUpdates(ctx, actor, actorAuth, ch.ID, KindNew)
}

func (s *Service) EditPhoto(ctx context.Context, actor, actorAuth int64, in *mtproto.InputChannel, photo *mtproto.InputChatPhoto) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, actor, in)
	if err != nil {
		return nil, err
	}
	if !s.canChangeInfo(ch, p) {
		return nil, mtproto.ErrChatAdminRequired
	}
	var photoID int64
	var action *mtproto.MessageAction
	if photo == nil || photo.PredicateName == mtproto.Predicate_inputChatPhotoEmpty {
		ch.PhotoID = 0
		action = mtproto.MakeMessageActionChatDeletePhoto()
	} else if photo.GetId() != nil {
		photoID = photo.GetId().GetId()
		ch.PhotoID = photoID
		action = mtproto.MakeMessageActionChatEditPhoto(mtproto.MakeTLPhotoEmpty(&mtproto.Photo{Id: photoID}).To_Photo())
	} else {
		return nil, mtproto.ErrPhotoInvalid
	}
	ch.Version++
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	msg := serviceMessage(actor, ch.ID, action)
	if _, _, err = s.appendMessage(ctx, ch.ID, actor, 0, msg); err != nil {
		return nil, err
	}
	return s.actorUpdates(ctx, actor, actorAuth, ch.ID, KindNew)
}

func (s *Service) CheckUsername(ctx context.Context, self int64, in *mtproto.InputChannel, username string) (*mtproto.Bool, error) {
	ch, _, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if !validChannelUsername(username) {
		return nil, mtproto.ErrUsernameInvalid
	}
	other, err := s.store.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if other != nil && other.ID != ch.ID {
		return nil, mtproto.ErrUsernameOccupied
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) UpdateUsername(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, username string) (*mtproto.Bool, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsCreator() && !p.adminRights().CanChangeInfo() {
		return nil, mtproto.ErrChatAdminRequired
	}
	username = strings.TrimSpace(username)
	if username == ch.Username {
		return nil, mtproto.ErrUsernameNotModified
	}
	if username != "" && !validChannelUsername(username) {
		return nil, mtproto.ErrUsernameInvalid
	}
	if username != "" {
		other, err := s.store.FindByUsername(ctx, username)
		if err != nil {
			return nil, err
		}
		if other != nil && other.ID != ch.ID {
			return nil, mtproto.ErrUsernameOccupied
		}
	}
	ch.Username = username
	ch.Version++
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	s.emit(ctx, self, actorAuth, ch.ID, func(viewer int64) *mtproto.Updates {
		return s.pack(ctx, viewer, ch.ID, channelUpdate(ch.ID))
	})
	return mtproto.BoolTrue, nil
}

func (s *Service) ToggleSignatures(ctx context.Context, actor, actorAuth int64, in *mtproto.InputChannel, enabled bool) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, actor, in)
	if err != nil {
		return nil, err
	}
	if !p.IsCreator() && !p.adminRights().CanChangeInfo() {
		return nil, mtproto.ErrChatAdminRequired
	}
	ch.Signatures = enabled
	ch.Version++
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	s.emit(ctx, actor, actorAuth, ch.ID, func(viewer int64) *mtproto.Updates {
		return s.pack(ctx, viewer, ch.ID, channelUpdate(ch.ID))
	})
	return s.pack(ctx, actor, ch.ID, channelUpdate(ch.ID)), nil
}

func (s *Service) AdminedPublicChannels(ctx context.Context, userID int64) (*mtproto.Messages_Chats, error) {
	list, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	chats := make([]*mtproto.Chat, 0)
	for _, ch := range list {
		if ch.Username == "" {
			continue
		}
		p, _ := s.store.GetParticipant(ctx, ch.ID, userID)
		if p.IsAdmin() {
			chats = append(chats, s.chatFor(ctx, userID, ch, p))
		}
	}
	return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: chats}).To_Messages_Chats(), nil
}

func (s *Service) DeleteChannel(ctx context.Context, actor, actorAuth int64, in *mtproto.InputChannel) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, actor, in)
	if err != nil {
		return nil, err
	}
	if !p.IsCreator() {
		return nil, mtproto.ErrChatAdminRequired
	}
	ch.Deleted = true
	ch.Version++
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	return mtproto.MakeUpdatesByUpdatesChats(
		[]*mtproto.Chat{forbiddenChat(ch)},
		channelUpdate(ch.ID),
	), nil
}

func (s *Service) ExportMessageLink(ctx context.Context, self int64, in *mtproto.InputChannel, msgID int32) (*mtproto.ExportedMessageLink, error) {
	ch, _, err := s.requireView(ctx, self, in)
	if err != nil {
		return nil, err
	}
	msg, err := s.store.GetMessage(ctx, ch.ID, msgID)
	if err != nil {
		return nil, err
	}
	if msg == nil || msg.Deleted {
		return nil, mtproto.ErrMessageIdInvalid
	}
	var link string
	if ch.Username != "" {
		link = linkHost() + "/" + ch.Username + "/" + itoa(msgID)
	} else {
		link = linkHost() + "/c/" + itoa64(ch.ID) + "/" + itoa(msgID)
	}
	return mtproto.MakeTLExportedMessageLink(&mtproto.ExportedMessageLink{
		Link: link,
		Html: `<a href="` + link + `">` + link + `</a>`,
	}).To_ExportedMessageLink(), nil
}

func missingInvitee(id int64) *mtproto.MissingInvitee {
	return mtproto.MakeTLMissingInvitee(&mtproto.MissingInvitee{UserId: id}).To_MissingInvitee()
}

func hashParticipants(ids []int64) int64 {
	var h int64
	for _, id := range ids {
		h = h*20261 + id
	}
	if h < 0 {
		h = -h
	}
	return h
}

func filterParticipants(all []*Participant, filter, q string) []*Participant {
	var out []*Participant
	for _, p := range all {
		if !matchParticipant(p, filter, q) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func matchParticipant(p *Participant, filter, q string) bool {
	switch filter {
	case mtproto.Predicate_channelParticipantsAdmins:
		return p.IsAdmin()
	case mtproto.Predicate_channelParticipantsKicked:
		return p.State == StateKicked && (q == "" || strings.Contains(itoa64(p.UserID), q))
	case mtproto.Predicate_channelParticipantsBanned:
		if !(p.Role == RoleBanned || p.State == StateKicked) {
			return false
		}
		return q == "" || strings.Contains(itoa64(p.UserID), q) || strings.Contains(strings.ToLower(p.Rank), q)
	case mtproto.Predicate_channelParticipantsBots:
		return p.IsBot && p.State == StateNormal
	case mtproto.Predicate_channelParticipantsSearch:
		if p.State != StateNormal {
			return false
		}
		if q == "" {
			return true
		}
		return strings.Contains(itoa64(p.UserID), q) || strings.Contains(strings.ToLower(p.Rank), q)
	case mtproto.Predicate_channelParticipantsContacts, mtproto.Predicate_channelParticipantsMentions, mtproto.Predicate_channelParticipantsRecent, "":
		return p.State == StateNormal
	default:
		return p.State == StateNormal
	}
}

func randInt63() int64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UnixNano()
	}
	v := int64(binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff)
	if v == 0 {
		return 1
	}
	return v
}

func itoa(v int32) string {
	return strconv.FormatInt(int64(v), 10)
}

func itoa64(v int64) string {
	return strconv.FormatInt(v, 10)
}
