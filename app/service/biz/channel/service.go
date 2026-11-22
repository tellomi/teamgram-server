package channel

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/chattitle"
	"github.com/teamgram/teamgram-server/pkg/usernamepolicy"
)

// 4–32 characters, [A-Za-z0-9_], starting with a letter. A trailing
// underscore is rejected by validChannelUsername.
var usernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{3,31}$`)

func validChannelUsername(username string) bool {
	return usernamePattern.MatchString(username) && !strings.HasSuffix(username, "_")
}

func allAdminRights() int32 {
	return int32(mtproto.MakeChatAdminRightsHelper(&mtproto.ChatAdminRights{
		ChangeInfo:           true,
		PostMessages:         true,
		EditMessages:         true,
		DeleteMessages:       true,
		BanUsers:             true,
		InviteUsers:          true,
		PinMessages:          true,
		AddAdmins:            true,
		ManageCall:           true,
		Other:                true,
		ManageTopics:         true,
		ManageRanks:          true,
		PostStories:          true,
		EditStories:          true,
		DeleteStories:        true,
		ManageDirectMessages: true,
		ManageLinkedPeers:    true,
	}))
}

func defaultChannelBanned() int64 {
	rights := mtproto.MakeDefaultBannedRights()
	rights.ChangeInfo = true
	rights.PinMessages = true
	return int64(rights.ToBannedRights())
}

// Hooks fan out channel updates and dialog rows. Each func is optional.
// Incoming runs for every member except the sender after a new channel message.
// broadcast is true for channels and false for supergroups.
// RepliesInbox writes one @replies private message. Nil leaves the comment
// stored and does not invent an inbox row.
type Hooks struct {
	Push     func(ctx context.Context, userID int64, excludeAuthKeys []int64, updates *mtproto.Updates)
	Dialog   func(ctx context.Context, userID, channelID int64, top, readInbox, readOutbox, unread int32, date int64)
	Incoming func(ctx context.Context, userID, channelID, sender int64, broadcast bool)
	// DropDialog removes one member's dialog after the channel is deleted.
	DropDialog func(ctx context.Context, userID, channelID int64)
	// Online reports which of ids have a live session. Nil means this
	// process has no presence service. Delete then pushes the affected set
	// only when that set is at most unreadSyncCap.
	Online func(ctx context.Context, ids []int64) (map[int64]bool, error)
	// UserCards loads display names and the viewer's contact bit.
	// channels.getParticipants has no names of its own. Nil means a name
	// or contacts filter cannot see profiles.
	UserCards func(ctx context.Context, viewer int64, ids []int64) map[int64]UserCard
	// RepliesInbox is called after a discussion comment is stored, and only
	// for the author of the message that was replied to. The callback must
	// treat RandomID as an idempotency key.
	RepliesInbox func(ctx context.Context, n RepliesNotice) error
}

// UserCard is the profile slice channels.getParticipants uses for q and
// channelParticipantsContacts.
type UserCard struct {
	First    string
	Last     string
	Username string
	Contact  bool
}

// Service is the channel and supergroup implementation.
type Service struct {
	store Store

	mu          sync.Mutex
	push        Hooks
	photos      ChatPhotoGetter
	loaded      bool
	ttlBackfill bool
}

func New(store Store) *Service {
	return &Service{store: store}
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
	title := chattitle.Clip(in.Title)
	if title == "" {
		return nil, mtproto.ErrChatTitleEmpty
	}
	if err := usernamepolicy.RejectDisplay(ctx, mtproto.PEER_CHANNEL, 0, title); err != nil {
		return nil, mtproto.ErrFirstnameInvalid
	}
	about := chattitle.ClipAbout(in.About)
	if err := usernamepolicy.RejectDisplay(ctx, mtproto.PEER_CHANNEL, 0, about); err != nil {
		return nil, mtproto.ErrFirstnameInvalid
	}
	if in.TtlPeriod < 0 || in.TtlPeriod > 366*86400 {
		return nil, mtproto.ErrTtlPeriodInvalid
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
	// Official clients already have a primary invite after create. A basic
	// group writes it in the same transaction as the chat. This is that row
	// for a supergroup or a broadcast channel, owned by the creator.
	if _, err := s.ensureOwnPrimary(ctx, ch.ID, in.CreatorID); err != nil {
		return nil, err
	}
	msg := serviceMessage(in.CreatorID, ch.ID, mtproto.MakeMessageActionChannelCreate(title))
	if _, _, err := s.appendMessage(ctx, ch.ID, in.CreatorID, 0, msg); err != nil {
		return nil, err
	}
	// Same rule as messages.createChat: ttl_period 0 is Off and writes no
	// messageActionSetMessagesTTL. A positive period is this channel's own
	// timer, so auto_setting_from stays unset.
	if in.TtlPeriod > 0 {
		ttlMsg := serviceMessage(in.CreatorID, ch.ID, mtproto.MakeMessageActionSetMessagesTTL(in.TtlPeriod))
		if _, _, err := s.appendMessage(ctx, ch.ID, in.CreatorID, 0, ttlMsg); err != nil {
			return nil, err
		}
	}
	if ch.Forum {
		s.ensureGeneralTopic(ctx, in.CreatorID, ch.ID)
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
	q := ""
	name := mtproto.Predicate_channelParticipantsRecent
	if filter != nil {
		name = filter.PredicateName
		q = filter.Q_STRING
		if filter.Q_FLAGSTRING != nil {
			q = filter.Q_FLAGSTRING.Value
		}
	}
	q = strings.ToLower(strings.TrimSpace(q))
	// A broadcast subscriber list is admin-only. Megagroups still list
	// members unless participants_hidden is on.
	if ch.Broadcast && (p == nil || !p.IsAdmin()) {
		return nil, mtproto.ErrChatAdminRequired
	}
	// Kicked and banned lists are an admin view. A normal member, and a
	// non-member of a public channel, must not read them.
	if restrictedParticipantFilter(name) && (p == nil || (!p.IsCreator() && !p.adminRights().CanBanUsers())) {
		return nil, mtproto.ErrChatAdminRequired
	}
	if feat := s.features(ctx, ch.ID); feat.ParticipantsHidden && (p == nil || !p.IsAdmin()) && name != mtproto.Predicate_channelParticipantsAdmins {
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
	matched := s.applyProfileFilter(ctx, self, name, q, filterParticipants(all, name))
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
	viewer := p
	if viewer == nil {
		viewer = &Participant{}
	}
	promoted := s.features(ctx, ch.ID).PromotedBy
	for _, part := range page {
		parts = append(parts, participantFor(self, viewer, promoted, part))
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
	return mtproto.MakeTLChannelsChannelParticipant(&mtproto.Channels_ChannelParticipant{
		Participant: participantFor(self, actor, s.features(ctx, ch.ID).PromotedBy, p),
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
		cur, err := s.store.GetParticipant(ctx, ch.ID, id)
		if err != nil {
			return nil, err
		}
		if cur != nil && cur.InChannel() {
			if err = s.repairJoinBoundary(ctx, ch, cur); err != nil {
				return nil, err
			}
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
			InviterID: actor,
			IsBot:     bots[id],
			Date:      now,
		}
		if cur != nil && cur.Role == RoleAdmin {
			np.Role = RoleAdmin
			np.AdminRights = cur.AdminRights
			np.Rank = cur.Rank
		}
		// The inbox cursor and JoinTop are stored before StateNormal.
		// keepOutbox retains a cursor this user already earned.
		if err = s.commitJoin(ctx, ch, np, true); err != nil {
			return nil, err
		}
		added = append(added, id)
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
	p, err := s.store.GetParticipant(ctx, ch.ID, user)
	if err != nil {
		return nil, err
	}
	if p != nil && p.InChannel() {
		if err = s.repairJoinBoundary(ctx, ch, p); err != nil {
			return nil, err
		}
		return s.pack(ctx, user, ch.ID, channelUpdate(ch.ID)), nil
	}
	if p != nil && p.State == StateKicked {
		return nil, mtproto.ErrUserBannedInChannel
	}
	// A public channel or megagroup with join_request does not add the user.
	// Official clients send channels.joinChannel after resolving the username
	// and expect INVITE_REQUEST_SENT plus a pending request an admin can see.
	// A failed feature read is not "join requests are off".
	// https://core.telegram.org/method/channels.joinChannel
	feat, err := s.mutableFeatures(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	if feat.JoinRequest {
		if err := s.recordJoinRequest(ctx, ch.ID, user); err != nil {
			return nil, err
		}
		return nil, mtproto.ErrInviteRequestSent
	}
	if !ch.Public() {
		return nil, mtproto.ErrChannelPrivate
	}
	now := int32(time.Now().Unix())
	np := &Participant{
		ChannelID: ch.ID,
		UserID:    user,
		Role:      RoleMember,
		Date:      now,
	}
	if p != nil {
		np.InviterID = p.InviterID
		np.IsBot = p.IsBot
	}
	if err = s.commitJoin(ctx, ch, np, false); err != nil {
		return nil, err
	}
	if _, err = s.refreshCount(ctx, ch.ID); err != nil {
		return nil, err
	}
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
		if errors.Is(err, errParticipantChanged) {
			latest, lerr := s.store.GetParticipant(ctx, ch.ID, user)
			if lerr != nil {
				return nil, lerr
			}
			if latest != nil && latest.State == StateLeft {
				return s.pack(ctx, user, ch.ID, channelUpdate(ch.ID)), nil
			}
		}
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
	// An admin may edit only the admins they promoted, and may not hand out
	// a right they do not hold. The creator is not limited by either rule.
	// https://core.telegram.org/method/channels.editAdmin
	feat := s.features(ctx, ch.ID)
	if target.IsAdmin() && !actorP.IsCreator() && promoterOf(feat.PromotedBy, target) != actor {
		return nil, mtproto.ErrRightForbidden
	}
	if !actorP.IsCreator() && want&^actorP.AdminRights != 0 {
		return nil, mtproto.ErrRightForbidden
	}
	prev := *target
	if want == 0 {
		target.Role = RoleMember
		target.AdminRights = 0
		target.Rank = ""
		rememberPromoter(feat, target.UserID, 0)
	} else {
		target.Role = RoleAdmin
		target.AdminRights = want
		target.Rank = rank
		if target.InviterID == 0 {
			target.InviterID = actor
		}
		rememberPromoter(feat, target.UserID, actor)
	}
	if err := s.store.PutParticipant(ctx, target); err != nil {
		return nil, err
	}
	if err := s.saveFeatures(ctx, ch.ID, feat); err != nil {
		return nil, err
	}
	s.logAdmin(ctx, ch.ID, actor, mtproto.MakeTLChannelAdminLogEventActionParticipantToggleAdmin(&mtproto.ChannelAdminLogEventAction{
		PrevParticipant: participantTL(actor, &prev),
		NewParticipant:  participantTL(actor, target),
	}).To_ChannelAdminLogEventAction())
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
	prev := *target
	if rights == nil || rights.NoBanRights() {
		// A forever view_messages ban removed the user. Clearing it lifts
		// the ban and does not put them back in the member list. A dated
		// view ban is a restriction: the member stays, and unban keeps them.
		removed := target.State == StateKicked || storedForeverKick(target.BannedRights)
		target.Role = RoleMember
		target.BannedRights = 0
		if removed {
			target.State = StateLeft
		}
	} else {
		target.Role = RoleBanned
		target.BannedRights = int64(rights.ToBannedRights())
		target.InviterID = actor
		if isMemberKick(rights) {
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
	s.logAdmin(ctx, ch.ID, actor, mtproto.MakeTLChannelAdminLogEventActionParticipantToggleBan(&mtproto.ChannelAdminLogEventAction{
		PrevParticipant: participantTL(actor, &prev),
		NewParticipant:  participantTL(actor, target),
	}).To_ChannelAdminLogEventAction())
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
	title = chattitle.Clip(title)
	if title == "" {
		return nil, mtproto.ErrChatTitleEmpty
	}
	if title != ch.Title {
		if err = usernamepolicy.RejectDisplay(ctx, mtproto.PEER_CHANNEL, ch.ID, title); err != nil {
			return nil, mtproto.ErrFirstnameInvalid
		}
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

// EditAbout implements messages.editChatAbout for a channel or supergroup.
// An empty string clears the description. More than 255 characters is
// CHAT_ABOUT_TOO_LONG. The same text is CHAT_ABOUT_NOT_MODIFIED.
func (s *Service) EditAbout(ctx context.Context, actor, actorAuth int64, in *mtproto.InputChannel, about string) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, actor, in)
	if err != nil {
		return nil, err
	}
	if !s.canChangeInfo(ch, p) {
		return nil, mtproto.ErrChatAdminRequired
	}
	if len([]rune(about)) > 255 {
		return nil, mtproto.ErrChatAboutTooLong
	}
	if about == ch.About {
		return nil, mtproto.ErrChatAboutNotModified
	}
	if err := usernamepolicy.RejectDisplay(ctx, mtproto.PEER_CHANNEL, ch.ID, about); err != nil {
		return nil, mtproto.ErrFirstnameInvalid
	}
	prev := ch.About
	ch.About = about
	ch.Version++
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	s.logAdmin(ctx, ch.ID, actor, mtproto.MakeTLChannelAdminLogEventActionChangeAbout(&mtproto.ChannelAdminLogEventAction{
		PrevValue_STRING: prev,
		NewValue_STRING:  about,
	}).To_ChannelAdminLogEventAction())
	return s.pack(ctx, actor, ch.ID, channelUpdate(ch.ID)), nil
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
	if err := usernamepolicy.RejectChannel(ctx, ch.ID, username); err != nil {
		return nil, err
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
	// channelFull.can_set_username is creator-only. change_info does not
	// cover turning a private group into a public one.
	if !p.IsCreator() {
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
		if err := usernamepolicy.RejectChannel(ctx, ch.ID, username); err != nil {
			return nil, err
		}
	}
	if err := s.assignEditableUsername(ctx, self, ch, username); err != nil {
		return nil, err
	}
	s.emit(ctx, self, actorAuth, ch.ID, func(viewer int64) *mtproto.Updates {
		return s.pack(ctx, viewer, ch.ID, channelUpdate(ch.ID))
	})
	return mtproto.BoolTrue, nil
}

// ToggleSignatures stores channels.toggleSignatures. Layer 232 sends
// signatures_enabled and profiles_enabled as true-flags, not the legacy
// enabled:Bool. An unchanged pair is CHAT_NOT_MODIFIED.
func (s *Service) ToggleSignatures(ctx context.Context, actor, actorAuth int64, in *mtproto.InputChannel, signatures, profiles bool) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, actor, in)
	if err != nil {
		return nil, err
	}
	if !p.IsCreator() && !p.adminRights().CanChangeInfo() {
		return nil, mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	if ch.Signatures == signatures && f.SignatureProfiles == profiles {
		return nil, mtproto.ErrChatNotModified
	}
	prevSig := ch.Signatures
	prevProf := f.SignatureProfiles
	ch.Signatures = signatures
	ch.Version++
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	f.SignatureProfiles = profiles
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	if prevSig != signatures {
		s.logAdmin(ctx, ch.ID, actor, mtproto.MakeTLChannelAdminLogEventActionToggleSignatures(&mtproto.ChannelAdminLogEventAction{
			NewValue_BOOL: mtproto.ToBool(signatures),
		}).To_ChannelAdminLogEventAction())
	}
	if prevProf != profiles {
		s.logAdmin(ctx, ch.ID, actor, mtproto.MakeTLChannelAdminLogEventActionToggleSignatureProfiles(&mtproto.ChannelAdminLogEventAction{
			NewValue_BOOL: mtproto.ToBool(profiles),
		}).To_ChannelAdminLogEventAction())
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
	s.notifyDeleted(ctx, actor, actorAuth, ch)
	return mtproto.MakeUpdatesByUpdatesChats(
		[]*mtproto.Chat{forbiddenChat(ch)},
		channelUpdate(ch.ID),
	), nil
}

// notifyDeleted tells every participant the channel is gone. Open clients
// receive updateChannel plus channelForbidden. Their dialog row is removed
// so a later getDialogs does not bring the chat back. The requesting auth
// key is skipped because that session already has the RPC result.
func (s *Service) notifyDeleted(ctx context.Context, actor, actorAuth int64, ch *Channel) {
	if ch == nil {
		return
	}
	h := s.hooks()
	parts, err := s.store.ListParticipants(ctx, ch.ID)
	if err != nil {
		return
	}
	for _, p := range parts {
		if p == nil {
			continue
		}
		if h.DropDialog != nil {
			h.DropDialog(ctx, p.UserID, ch.ID)
		}
		if h.Push == nil || p.State != StateNormal {
			continue
		}
		var excl []int64
		if p.UserID == actor && actorAuth != 0 {
			excl = []int64{actorAuth}
		}
		h.Push(ctx, p.UserID, excl, mtproto.MakeUpdatesByUpdatesChats(
			[]*mtproto.Chat{forbiddenChat(ch)},
			channelUpdate(ch.ID),
		))
	}
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

// isMemberKick reports channels.editBanned rights that remove the user.
// Clients send until_date 0 for a permanent kick. 0x7fffffff is the same
// forever value. Any other until_date is a view restriction that keeps
// the member in the group.
func isMemberKick(rights *mtproto.ChatBannedRights) bool {
	if rights == nil || !rights.GetViewMessages() {
		return false
	}
	until := rights.GetUntilDate()
	return until == 0 || until == math.MaxInt32
}

// storedForeverKick reports a saved ban that was a permanent kick.
// until_date 0 is stored as 0x7fffffff. A different until_date is not a kick.
func storedForeverKick(bits int64) bool {
	if bits&int64(mtproto.BANNED_VIEW_MESSAGES) == 0 {
		return false
	}
	until := int32(bits >> 32)
	return until == 0 || until == math.MaxInt32
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

func filterParticipants(all []*Participant, filter string) []*Participant {
	var out []*Participant
	for _, p := range all {
		if !matchParticipant(p, filter) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func matchParticipant(p *Participant, filter string) bool {
	switch filter {
	case mtproto.Predicate_channelParticipantsAdmins:
		return p.IsAdmin()
	case mtproto.Predicate_channelParticipantsKicked:
		return p.State == StateKicked
	case mtproto.Predicate_channelParticipantsBanned:
		return p.Role == RoleBanned || p.State == StateKicked
	case mtproto.Predicate_channelParticipantsBots:
		return p.IsBot && p.State == StateNormal
	default:
		return p.State == StateNormal
	}
}

func restrictedParticipantFilter(filter string) bool {
	return filter == mtproto.Predicate_channelParticipantsBanned || filter == mtproto.Predicate_channelParticipantsKicked
}

// profileQuery is true when q or the contacts bit needs user profiles.
// Recent and admins have no q in the schema.
func profileQuery(filter, q string) bool {
	if filter == mtproto.Predicate_channelParticipantsContacts {
		return true
	}
	if q == "" {
		return false
	}
	switch filter {
	case mtproto.Predicate_channelParticipantsKicked,
		mtproto.Predicate_channelParticipantsBanned,
		mtproto.Predicate_channelParticipantsSearch,
		mtproto.Predicate_channelParticipantsMentions:
		return true
	default:
		return false
	}
}

func (s *Service) applyProfileFilter(ctx context.Context, viewer int64, filter, q string, matched []*Participant) []*Participant {
	if !profileQuery(filter, q) {
		return matched
	}
	cards := map[int64]UserCard{}
	if fn := s.hooks().UserCards; fn != nil && len(matched) > 0 {
		ids := make([]int64, len(matched))
		for i, p := range matched {
			ids[i] = p.UserID
		}
		if got := fn(ctx, viewer, ids); got != nil {
			cards = got
		}
	}
	out := make([]*Participant, 0, len(matched))
	for _, p := range matched {
		card := cards[p.UserID]
		if filter == mtproto.Predicate_channelParticipantsContacts {
			// The viewer is not in their own address book. ToSelfUser
			// still reports contact and mutual_contact.
			if p.UserID == viewer || !card.Contact {
				continue
			}
		}
		if q != "" && !participantNameMatch(card, p.Rank, q) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func participantNameMatch(card UserCard, rank, q string) bool {
	blob := strings.ToLower(card.First + " " + card.Last + " " + card.Username + " " + rank)
	return strings.Contains(blob, q)
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
