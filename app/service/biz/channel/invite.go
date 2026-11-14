package channel

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	bizchat "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/teamgram/teamgram-server/pkg/invite"
)

// ExportRequest is messages.exportChatInvite for a channel or supergroup.
// Subscription is stars pricing. This server does not sell that, so a set
// value is PAYMENT_UNSUPPORTED rather than a link that cannot be billed.
type ExportRequest struct {
	LegacyRevokePermanent bool
	RequestNeeded         bool
	Expire                int32
	Usage                 int32
	UsageSet              bool
	Title                 string
	TitleSet              bool
	Subscription          bool
}

// EditRequest is messages.editExportedChatInvite. Nil pointers are flags
// the client did not send.
type EditRequest struct {
	Revoked       bool
	Link          string
	Expire        *int32
	Usage         *int32
	RequestNeeded *bool
	Title         *string
}

func canInvite(p *Participant) bool {
	return p != nil && p.InChannel() && (p.IsCreator() || p.adminRights().CanInviteUsers())
}

func inviteURL(hash string) string {
	return brand.LinkBase() + "/+" + hash
}

// ExportInvite creates the primary link or an additional one.
// legacy_revoke_permanent is what the iOS "Invite to group via link" sheet
// sends when the chat has no primary link yet. The additional-link screen
// (name, approval, expiry, usage) leaves that flag unset.
func (s *Service) ExportInvite(ctx context.Context, self int64, in *mtproto.InputChannel, req ExportRequest) (*mtproto.ExportedChatInvite, error) {
	if req.Subscription {
		return nil, mtproto.ErrPaymentUnsupported
	}
	ch, _, err := s.requireInviteAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	exp := invite.Export{
		Legacy:        req.LegacyRevokePermanent,
		RequestNeeded: req.RequestNeeded,
		Expire:        int64(req.Expire),
		Usage:         req.Usage,
		UsageSet:      req.UsageSet,
		Title:         req.Title,
		TitleSet:      req.TitleSet,
		Now:           now,
	}
	if err := exp.Validate(); err != nil {
		return nil, err
	}
	usage := exp.StoredUsage()
	if req.LegacyRevokePermanent {
		rows, err := s.store.ListInvites(ctx, ch.ID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.AdminID == self && row.Permanent && !row.Revoked {
				row.Revoked = true
				if err := s.store.UpdateInvite(ctx, row); err != nil {
					return nil, err
				}
			}
		}
		return s.insertInvite(ctx, &StoredInvite{
			ChannelID: ch.ID,
			AdminID:   self,
			Permanent: true,
			Date:      now,
		})
	}
	return s.insertInvite(ctx, &StoredInvite{
		ChannelID:     ch.ID,
		AdminID:       self,
		RequestNeeded: req.RequestNeeded,
		ExpireDate:    int64(req.Expire),
		UsageLimit:    usage,
		Title:         req.Title,
		Date:          now,
	})
}

func (s *Service) insertInvite(ctx context.Context, inv *StoredInvite) (*mtproto.ExportedChatInvite, error) {
	var err error
	for i := 0; i < 5; i++ {
		inv.Link = bizchat.GenChannelInviteHash()
		err = s.store.InsertInvite(ctx, inv)
		if err == nil {
			return s.exportTL(ctx, inv), nil
		}
		if !errors.Is(err, errInviteExists) {
			return nil, err
		}
	}
	return nil, err
}

// EditInvite updates a link, or revokes it. Revoking the primary link
// replaces it and returns both, which is messages.exportedChatInviteReplaced.
func (s *Service) EditInvite(ctx context.Context, self int64, in *mtproto.InputChannel, req EditRequest) ([]*mtproto.ExportedChatInvite, error) {
	ch, p, err := s.requireInviteAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	inv, err := s.inviteInChannel(ctx, ch.ID, req.Link)
	if err != nil {
		return nil, err
	}
	if inv.AdminID != self && !p.IsCreator() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if inv.Revoked {
		return nil, mtproto.ErrInviteHashExpired
	}
	now := time.Now().Unix()
	if req.Revoked {
		inv.Revoked = true
		if err := s.store.UpdateInvite(ctx, inv); err != nil {
			return nil, err
		}
		out := []*mtproto.ExportedChatInvite{s.exportTL(ctx, inv)}
		if inv.Permanent {
			fresh, err := s.insertInvite(ctx, &StoredInvite{
				ChannelID: ch.ID,
				AdminID:   inv.AdminID,
				Permanent: true,
				Date:      now,
			})
			if err != nil {
				return nil, err
			}
			out = append(out, fresh)
		}
		return out, nil
	}
	if inv.Permanent && (req.Expire != nil || req.Usage != nil || req.RequestNeeded != nil || req.Title != nil) {
		return nil, mtproto.ErrChatInvitePermanent
	}
	if req.Expire != nil {
		if *req.Expire != 0 && int64(*req.Expire) <= now {
			return nil, mtproto.ErrExpireDateInvalid
		}
		inv.ExpireDate = int64(*req.Expire)
	}
	if req.Usage != nil {
		stored, err := invite.NormalizeUsage(true, *req.Usage)
		if err != nil {
			return nil, err
		}
		inv.UsageLimit = stored
	}
	if req.RequestNeeded != nil {
		inv.RequestNeeded = *req.RequestNeeded
	}
	if req.Title != nil {
		if !invite.TitleOK(*req.Title) {
			return nil, mtproto.ErrTitleInvalid
		}
		inv.Title = *req.Title
	}
	if err := s.store.UpdateInvite(ctx, inv); err != nil {
		return nil, err
	}
	return []*mtproto.ExportedChatInvite{s.exportTL(ctx, inv)}, nil
}

// ExportedInvites lists one admin's links. The caller must be that admin
// or the creator. The slice is every match, newest first. The BFF pages it
// so messages.exportedChatInvites.count stays the total.
func (s *Service) ExportedInvites(ctx context.Context, self int64, in *mtproto.InputChannel, adminID int64, revoked bool) ([]*mtproto.ExportedChatInvite, error) {
	ch, p, err := s.requireInviteAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if adminID == 0 {
		adminID = self
	}
	if adminID != self && !p.IsCreator() {
		return nil, mtproto.ErrChatAdminRequired
	}
	rows, err := s.store.ListInvites(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	out := make([]*mtproto.ExportedChatInvite, 0)
	for _, row := range rows {
		if row.AdminID == adminID && row.Revoked == revoked {
			out = append(out, s.exportTL(ctx, row))
		}
	}
	return out, nil
}

// ExportedInvite loads one link by URL or hash.
func (s *Service) ExportedInvite(ctx context.Context, self int64, in *mtproto.InputChannel, link string) (*mtproto.ExportedChatInvite, error) {
	ch, _, err := s.requireInviteAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	inv, err := s.inviteInChannel(ctx, ch.ID, link)
	if err != nil {
		return nil, err
	}
	return s.exportTL(ctx, inv), nil
}

// DeleteInvite removes one link. The primary link can be removed. The next
// export with legacy_revoke_permanent creates a new one.
func (s *Service) DeleteInvite(ctx context.Context, self int64, in *mtproto.InputChannel, link string) error {
	ch, p, err := s.requireInviteAdmin(ctx, self, in)
	if err != nil {
		return err
	}
	inv, err := s.inviteInChannel(ctx, ch.ID, link)
	if err != nil {
		return err
	}
	if inv.AdminID != self && !p.IsCreator() {
		return mtproto.ErrChatAdminRequired
	}
	return s.store.DeleteInvite(ctx, ch.ID, inv.Link)
}

// DeleteRevokedInvites removes every revoked link of one admin.
func (s *Service) DeleteRevokedInvites(ctx context.Context, self int64, in *mtproto.InputChannel, adminID int64) error {
	ch, p, err := s.requireInviteAdmin(ctx, self, in)
	if err != nil {
		return err
	}
	if adminID == 0 {
		adminID = self
	}
	if adminID != self && !p.IsCreator() {
		return mtproto.ErrChatAdminRequired
	}
	return s.store.DeleteRevokedInvites(ctx, ch.ID, adminID)
}

// InviteImporters lists who joined or requested through the channel's links.
// link empty means every link. q is applied by the BFF, which has the user
// profiles. limit 0 returns the whole match.
func (s *Service) InviteImporters(ctx context.Context, self int64, in *mtproto.InputChannel, requested bool, link string, offsetDate int32, offsetUser int64, limit int32) ([]*mtproto.ChatInviteImporter, int32, error) {
	ch, _, err := s.requireInviteAdmin(ctx, self, in)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.store.ListImporters(ctx, ch.ID)
	if err != nil {
		return nil, 0, err
	}
	hash := brand.InviteHash(link)
	matched := make([]*StoredImporter, 0, len(rows))
	for _, row := range rows {
		if row.Requested != requested {
			continue
		}
		if hash != "" && row.Link != hash {
			continue
		}
		matched = append(matched, row)
	}
	total := int32(len(matched))
	start := 0
	if offsetUser != 0 || offsetDate != 0 {
		start = len(matched)
		for i, row := range matched {
			if row.UserID == offsetUser && int32(row.Date) == offsetDate {
				start = i + 1
				break
			}
		}
	}
	if start > len(matched) {
		start = len(matched)
	}
	end := len(matched)
	if limit > 0 && start+int(limit) < end {
		end = start + int(limit)
	}
	out := make([]*mtproto.ChatInviteImporter, 0, end-start)
	for _, row := range matched[start:end] {
		out = append(out, mtproto.MakeTLChatInviteImporter(&mtproto.ChatInviteImporter{
			Requested:  row.Requested,
			UserId:     row.UserID,
			Date:       int32(row.Date),
			About:      mtproto.MakeFlagsString(row.About),
			ApprovedBy: mtproto.MakeFlagsInt64(row.ApprovedBy),
		}).To_ChatInviteImporter())
	}
	return out, total, nil
}

// AdminsWithInvites counts links per admin who can still invite.
func (s *Service) AdminsWithInvites(ctx context.Context, self int64, in *mtproto.InputChannel) ([]*mtproto.ChatAdminWithInvites, error) {
	ch, _, err := s.requireInviteAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	parts, err := s.store.ListParticipants(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	allowed := map[int64]bool{}
	for _, p := range parts {
		if canInvite(p) {
			allowed[p.UserID] = true
		}
	}
	rows, err := s.store.ListInvites(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	byAdmin := map[int64]*mtproto.ChatAdminWithInvites{}
	var order []int64
	for _, row := range rows {
		if !allowed[row.AdminID] {
			continue
		}
		admin := byAdmin[row.AdminID]
		if admin == nil {
			admin = mtproto.MakeTLChatAdminWithInvites(&mtproto.ChatAdminWithInvites{
				AdminId: row.AdminID,
			}).To_ChatAdminWithInvites()
			byAdmin[row.AdminID] = admin
			order = append(order, row.AdminID)
		}
		if row.Revoked {
			admin.RevokedInvitesCount++
		} else {
			admin.InvitesCount++
		}
	}
	out := make([]*mtproto.ChatAdminWithInvites, 0, len(order))
	for _, id := range order {
		out = append(out, byAdmin[id])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AdminId < out[j].AdminId })
	return out, nil
}

// CheckInvite is messages.checkChatInvite for a channel hash (a leading letter).
// The returned ids are member user ids the BFF fills into chatInvite.participants.
func (s *Service) CheckInvite(ctx context.Context, self int64, hash string) (*mtproto.ChatInvite, []int64, error) {
	inv, ch, err := s.liveInvite(ctx, hash)
	if err != nil {
		return nil, nil, err
	}
	me, err := s.store.GetParticipant(ctx, ch.ID, self)
	if err != nil {
		return nil, nil, err
	}
	if me != nil && me.InChannel() {
		return mtproto.MakeTLChatInviteAlready(&mtproto.ChatInvite{
			Chat: s.ChatFor(ctx, self, ch),
		}).To_ChatInvite(), nil, nil
	}
	parts, err := s.store.ListParticipants(ctx, ch.ID)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]int64, 0, 4)
	for _, p := range parts {
		if p.InChannel() {
			ids = append(ids, p.UserID)
			if len(ids) == 4 {
				break
			}
		}
	}
	need := inv.RequestNeeded || s.features(ctx, ch.ID).JoinRequest
	return mtproto.MakeTLChatInvite(&mtproto.ChatInvite{
		Channel:           true,
		Broadcast:         ch.Broadcast,
		Public:            ch.Public(),
		Megagroup:         ch.Megagroup,
		RequestNeeded:     need,
		Title:             ch.Title,
		About:             mtproto.MakeFlagsString(ch.About),
		Photo:             mtproto.MakeTLPhotoEmpty(nil).To_Photo(),
		ParticipantsCount: ch.ParticipantsCount,
		Color:             0,
	}).To_ChatInvite(), ids, nil
}

// ImportInvite joins through a channel invite. A link that asks for approval
// records the request and returns INVITE_REQUEST_SENT.
func (s *Service) ImportInvite(ctx context.Context, self, actorAuth int64, hash string) (*mtproto.Updates, error) {
	inv, ch, err := s.liveInvite(ctx, hash)
	if err != nil {
		return nil, err
	}
	me, err := s.store.GetParticipant(ctx, ch.ID, self)
	if err != nil {
		return nil, err
	}
	if me != nil && me.InChannel() {
		return nil, mtproto.ErrUserAlreadyParticipant
	}
	if me != nil && me.State == StateKicked {
		return nil, mtproto.ErrUserBannedInChannel
	}
	now := time.Now().Unix()
	need := inv.RequestNeeded || s.features(ctx, ch.ID).JoinRequest
	if need {
		if err := s.store.UpsertImporter(ctx, &StoredImporter{
			ChannelID: ch.ID,
			Link:      inv.Link,
			UserID:    self,
			Requested: true,
			Date:      now,
		}); err != nil {
			return nil, err
		}
		if err := s.addJoinRequest(ctx, ch.ID, self, inv.Link); err != nil {
			return nil, err
		}
		s.pushPending(ctx, self, ch.ID)
		return nil, mtproto.ErrInviteRequestSent
	}
	np := &Participant{
		ChannelID: ch.ID,
		UserID:    self,
		Role:      RoleMember,
		State:     StateNormal,
		InviterID: inv.AdminID,
		Date:      int32(now),
	}
	if me != nil {
		np.IsBot = me.IsBot
	}
	if err := s.store.PutParticipant(ctx, np); err != nil {
		return nil, err
	}
	if _, err = s.refreshCount(ctx, ch.ID); err != nil {
		return nil, err
	}
	_ = s.store.SaveRead(ctx, &ReadState{ChannelID: ch.ID, UserID: self, ReadInboxMaxID: ch.TopMessage})
	if err := s.store.UpsertImporter(ctx, &StoredImporter{
		ChannelID: ch.ID,
		Link:      inv.Link,
		UserID:    self,
		Date:      now,
	}); err != nil {
		return nil, err
	}
	msg := serviceMessage(self, ch.ID, mtproto.MakeMessageActionChatJoinByLink(inv.AdminID))
	if _, _, err = s.appendMessage(ctx, ch.ID, self, 0, msg); err != nil {
		return nil, err
	}
	return s.actorUpdates(ctx, self, actorAuth, ch.ID, KindNew)
}

// HideJoinRequest approves or dismisses one pending request.
func (s *Service) HideJoinRequest(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, userID int64, approved bool) (*mtproto.Updates, error) {
	if err := s.ApproveJoin(ctx, self, actorAuth, in, userID, approved); err != nil {
		return nil, err
	}
	ch, err := s.lookup(ctx, in)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.ListImporters(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.UserID != userID || !row.Requested {
			continue
		}
		if approved {
			row.Requested = false
			row.ApprovedBy = self
			row.Date = time.Now().Unix()
			if err := s.store.UpsertImporter(ctx, row); err != nil {
				return nil, err
			}
		} else if err := s.store.DeleteImporter(ctx, row.Link, row.UserID); err != nil {
			return nil, err
		}
	}
	return s.pendingUpdates(ctx, self, ch.ID), nil
}

// PrimaryExported is channelFull.exported_invite for an admin who can invite.
func (s *Service) PrimaryExported(ctx context.Context, channelID, adminID int64) *mtproto.ExportedChatInvite {
	rows, err := s.store.ListInvites(ctx, channelID)
	if err != nil {
		return nil
	}
	var best *StoredInvite
	for _, row := range rows {
		if row.AdminID == adminID && row.Permanent && !row.Revoked && (best == nil || row.Date >= best.Date) {
			best = row
		}
	}
	if best == nil {
		return nil
	}
	return s.exportTL(ctx, best)
}

func (s *Service) requireInviteAdmin(ctx context.Context, self int64, in *mtproto.InputChannel) (*Channel, *Participant, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, nil, err
	}
	if !canInvite(p) {
		return nil, nil, mtproto.ErrChatAdminInviteRequired
	}
	return ch, p, nil
}

func (s *Service) inviteInChannel(ctx context.Context, channelID int64, link string) (*StoredInvite, error) {
	hash := brand.InviteHash(link)
	if hash == "" {
		return nil, mtproto.ErrInviteHashInvalid
	}
	inv, err := s.store.GetInviteByLink(ctx, hash)
	if err != nil {
		return nil, err
	}
	if inv == nil || inv.ChannelID != channelID {
		return nil, mtproto.ErrInviteHashInvalid
	}
	return inv, nil
}

func (s *Service) liveInvite(ctx context.Context, hash string) (*StoredInvite, *Channel, error) {
	if len(hash) == 0 {
		return nil, nil, mtproto.ErrInviteHashEmpty
	}
	if !bizchat.IsChannelInviteHash(hash) {
		return nil, nil, mtproto.ErrInviteHashInvalid
	}
	inv, err := s.store.GetInviteByLink(ctx, hash)
	if err != nil {
		return nil, nil, err
	}
	if inv == nil {
		return nil, nil, mtproto.ErrInviteHashInvalid
	}
	usage, _ := s.importerCounts(ctx, inv.ChannelID, inv.Link)
	if inviteDead(inv, usage, time.Now().Unix()) {
		return nil, nil, mtproto.ErrInviteHashExpired
	}
	ch, err := s.store.GetChannel(ctx, inv.ChannelID)
	if err != nil {
		return nil, nil, err
	}
	if ch == nil || ch.Deleted {
		return nil, nil, mtproto.ErrInviteHashInvalid
	}
	admin, err := s.store.GetParticipant(ctx, ch.ID, inv.AdminID)
	if err != nil {
		return nil, nil, err
	}
	if !canInvite(admin) {
		return nil, nil, mtproto.ErrInviteHashExpired
	}
	return inv, ch, nil
}

func inviteDead(inv *StoredInvite, usage int32, now int64) bool {
	if inv == nil || inv.Revoked {
		return true
	}
	if invite.NotStarted(inv.StartDate, now) {
		return true
	}
	if inv.ExpireDate != 0 && now > inv.ExpireDate {
		return true
	}
	if invite.UsageExhausted(inv.UsageLimit, int(usage)) {
		return true
	}
	return false
}

func (s *Service) importerCounts(ctx context.Context, channelID int64, link string) (usage, requested int32) {
	rows, err := s.store.ListImporters(ctx, channelID)
	if err != nil {
		return 0, 0
	}
	for _, row := range rows {
		if row.Link != link {
			continue
		}
		if row.Requested {
			requested++
		} else {
			usage++
		}
	}
	return usage, requested
}

func (s *Service) exportTL(ctx context.Context, inv *StoredInvite) *mtproto.ExportedChatInvite {
	usage, requested := s.importerCounts(ctx, inv.ChannelID, inv.Link)
	return mtproto.MakeTLChatInviteExported(&mtproto.ExportedChatInvite{
		Revoked:       inv.Revoked,
		Permanent:     inv.Permanent,
		RequestNeeded: inv.RequestNeeded,
		Link:          inviteURL(inv.Link),
		AdminId:       inv.AdminID,
		Date:          int32(inv.Date),
		StartDate:     mtproto.MakeFlagsInt32(int32(inv.StartDate)),
		ExpireDate:    mtproto.MakeFlagsInt32(int32(inv.ExpireDate)),
		UsageLimit:    mtproto.MakeFlagsInt32(inv.UsageLimit),
		Usage:         mtproto.MakeFlagsInt32(usage),
		Requested:     mtproto.MakeFlagsInt32(requested),
		Title:         mtproto.MakeFlagsString(inv.Title),
	}).To_ExportedChatInvite()
}

// publicJoinLink is the importer key for channels.joinChannel when the
// channel itself asks for approval. It is not a 20-character invite hash,
// so messages.importChatInvite cannot redeem it. getChatInviteImporters
// with an empty link still returns the row.
func publicJoinLink(channelID int64) string {
	return "jreq" + strconv.FormatInt(channelID, 10)
}

// recordJoinRequest stores a pending join from channels.joinChannel.
// The user is not added. Admins see the row in getChatInviteImporters
// (requested=true), can approve it with hideChatJoinRequest, and receive
// updatePendingJoinRequests.
func (s *Service) recordJoinRequest(ctx context.Context, channelID, user int64) error {
	link := publicJoinLink(channelID)
	if err := s.addJoinRequest(ctx, channelID, user, link); err != nil {
		return err
	}
	now := time.Now().Unix()
	rows, err := s.store.ListImporters(ctx, channelID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.UserID == user && row.Requested && row.Link == link {
			now = row.Date
			break
		}
	}
	if err := s.store.UpsertImporter(ctx, &StoredImporter{
		ChannelID: channelID,
		Link:      link,
		UserID:    user,
		Requested: true,
		Date:      now,
	}); err != nil {
		return err
	}
	s.pushPending(ctx, user, channelID)
	return nil
}

func (s *Service) addJoinRequest(ctx context.Context, channelID, user int64, hash string) error {
	f := s.features(ctx, channelID)
	for _, req := range f.JoinRequests {
		if req.UserID == user {
			return nil
		}
	}
	f.JoinRequests = append(f.JoinRequests, JoinReq{
		UserID: user,
		Date:   int32(time.Now().Unix()),
		Link:   hash,
	})
	return s.saveFeatures(ctx, channelID, f)
}

func (s *Service) pendingUpdates(ctx context.Context, self, channelID int64) *mtproto.Updates {
	up := s.pendingUpdate(ctx, channelID)
	s.pushPending(ctx, self, channelID)
	return s.pack(ctx, self, channelID, up)
}

func (s *Service) pendingUpdate(ctx context.Context, channelID int64) *mtproto.Update {
	f := s.features(ctx, channelID)
	recent := make([]int64, 0, 5)
	for _, req := range f.JoinRequests {
		recent = append(recent, req.UserID)
		if len(recent) == 5 {
			break
		}
	}
	return mtproto.MakeTLUpdatePendingJoinRequests(&mtproto.Update{
		Peer_PEER:        mtproto.MakePeerChannel(channelID),
		RequestsPending:  int32(len(f.JoinRequests)),
		RecentRequesters: recent,
	}).To_Update()
}

func (s *Service) pushPending(ctx context.Context, actor, channelID int64) {
	h := s.hooks()
	if h.Push == nil {
		return
	}
	parts, err := s.store.ListParticipants(ctx, channelID)
	if err != nil {
		return
	}
	up := s.pendingUpdate(ctx, channelID)
	for _, p := range parts {
		if p.UserID == actor || !canInvite(p) {
			continue
		}
		h.Push(ctx, p.UserID, nil, s.pack(ctx, p.UserID, channelID, up))
	}
}

// syncImportersAfterHide updates the join log after messages.hideAllChatJoinRequests.
func (s *Service) syncImportersAfterHide(ctx context.Context, self, channelID int64, users []int64, approved bool, link string) {
	want := brand.InviteHash(link)
	rows, err := s.store.ListImporters(ctx, channelID)
	if err != nil {
		return
	}
	hit := map[int64]bool{}
	for _, id := range users {
		hit[id] = true
	}
	now := time.Now().Unix()
	for _, row := range rows {
		if !hit[row.UserID] || !row.Requested {
			continue
		}
		if want != "" && row.Link != want {
			continue
		}
		if approved {
			row.Requested = false
			row.ApprovedBy = self
			row.Date = now
			_ = s.store.UpsertImporter(ctx, row)
		} else {
			_ = s.store.DeleteImporter(ctx, row.Link, row.UserID)
		}
	}
}

// PendingRequesters is channelFull.requests_pending for an admin.
func (s *Service) PendingRequesters(ctx context.Context, channelID int64) (int32, []int64) {
	f := s.features(ctx, channelID)
	if len(f.JoinRequests) == 0 {
		return 0, nil
	}
	recent := make([]int64, 0, 5)
	for _, req := range f.JoinRequests {
		recent = append(recent, req.UserID)
		if len(recent) == 5 {
			break
		}
	}
	return int32(len(f.JoinRequests)), recent
}
