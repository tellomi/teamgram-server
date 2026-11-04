package core

import (
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func inputChannelOf(peer *mtproto.PeerUtil) *mtproto.InputChannel {
	return mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  peer.PeerId,
		AccessHash: peer.AccessHash,
	}).To_InputChannel()
}

func (c *ChatInvitesCore) fetchUsers(ids []int64) []*mtproto.User {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || c.MD == nil || len(ids) == 0 {
		return []*mtproto.User{}
	}
	seen := map[int64]struct{}{}
	uniq := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return []*mtproto.User{}
	}
	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: uniq})
	if err != nil || users == nil {
		return []*mtproto.User{}
	}
	out := users.GetUserListByIdList(c.MD.UserId, uniq...)
	if out == nil {
		return []*mtproto.User{}
	}
	return out
}

func pageInvites(all []*mtproto.ExportedChatInvite, offsetDate int32, offsetLink string, limit int32) ([]*mtproto.ExportedChatInvite, int32) {
	total := int32(len(all))
	start := 0
	if offsetLink != "" || offsetDate != 0 {
		start = len(all)
		for i, inv := range all {
			if inv.GetLink() == offsetLink && inv.GetDate() == offsetDate {
				start = i + 1
				break
			}
		}
	}
	if limit <= 0 {
		limit = 50
	}
	if start > len(all) {
		start = len(all)
	}
	end := start + int(limit)
	if end > len(all) {
		end = len(all)
	}
	return append([]*mtproto.ExportedChatInvite{}, all[start:end]...), total
}

func userMatches(u *mtproto.User, q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	if u == nil {
		return false
	}
	blob := strings.ToLower(u.GetFirstName().GetValue() + " " + u.GetLastName().GetValue() + " " + u.GetUsername().GetValue())
	return strings.Contains(blob, q)
}

func (c *ChatInvitesCore) exportChannelInvite(in *mtproto.TLMessagesExportChatInvite, peer *mtproto.PeerUtil) (*mtproto.ExportedChatInvite, error) {
	if in.GetSubscriptionPricing() != nil {
		return nil, mtproto.ErrPaymentUnsupported
	}
	return channel.Shared().ExportInvite(c.ctx, c.MD.UserId, inputChannelOf(peer), channel.ExportRequest{
		LegacyRevokePermanent: in.GetLegacyRevokePermanent(),
		RequestNeeded:         in.GetRequestNeeded(),
		Expire:                in.GetExpireDate().GetValue(),
		Usage:                 in.GetUsageLimit().GetValue(),
		UsageSet:              in.GetUsageLimit() != nil,
		Title:                 in.GetTitle().GetValue(),
		TitleSet:              in.GetTitle() != nil,
	})
}

func (c *ChatInvitesCore) editChannelInvite(in *mtproto.TLMessagesEditExportedChatInvite, peer *mtproto.PeerUtil) (*mtproto.Messages_ExportedChatInvite, error) {
	var need *bool
	if in.GetRequestNeeded() != nil {
		v := mtproto.FromBool(in.GetRequestNeeded())
		need = &v
	}
	var expire *int32
	if in.GetExpireDate() != nil {
		v := in.GetExpireDate().GetValue()
		expire = &v
	}
	var usage *int32
	if in.GetUsageLimit() != nil {
		v := in.GetUsageLimit().GetValue()
		usage = &v
	}
	var title *string
	if in.GetTitle() != nil {
		v := in.GetTitle().GetValue()
		title = &v
	}
	list, err := channel.Shared().EditInvite(c.ctx, c.MD.UserId, inputChannelOf(peer), channel.EditRequest{
		Revoked:       in.GetRevoked(),
		Link:          in.GetLink(),
		Expire:        expire,
		Usage:         usage,
		RequestNeeded: need,
		Title:         title,
	})
	if err != nil {
		return nil, err
	}
	return c.wrapInvites(list)
}

func (c *ChatInvitesCore) wrapInvites(list []*mtproto.ExportedChatInvite) (*mtproto.Messages_ExportedChatInvite, error) {
	if len(list) == 0 || len(list) > 2 {
		return nil, mtproto.ErrInternalServerError
	}
	users := c.fetchUsers([]int64{list[0].GetAdminId()})
	if len(list) == 1 {
		return mtproto.MakeTLMessagesExportedChatInvite(&mtproto.Messages_ExportedChatInvite{
			Invite: list[0],
			Users:  users,
		}).To_Messages_ExportedChatInvite(), nil
	}
	return mtproto.MakeTLMessagesExportedChatInviteReplaced(&mtproto.Messages_ExportedChatInvite{
		Invite:    list[0],
		NewInvite: list[1],
		Users:     users,
	}).To_Messages_ExportedChatInvite(), nil
}

func (c *ChatInvitesCore) listChannelInvites(in *mtproto.TLMessagesGetExportedChatInvites, peer *mtproto.PeerUtil, adminID int64) (*mtproto.Messages_ExportedChatInvites, error) {
	all, err := channel.Shared().ExportedInvites(c.ctx, c.MD.UserId, inputChannelOf(peer), adminID, in.GetRevoked())
	if err != nil {
		return nil, err
	}
	page, total := pageInvites(all, in.GetOffsetDate().GetValue(), in.GetOffsetLink().GetValue(), in.GetLimit())
	ids := make([]int64, 0, len(page))
	for _, inv := range page {
		ids = append(ids, inv.GetAdminId())
	}
	if page == nil {
		page = []*mtproto.ExportedChatInvite{}
	}
	return mtproto.MakeTLMessagesExportedChatInvites(&mtproto.Messages_ExportedChatInvites{
		Count:   total,
		Invites: page,
		Users:   c.fetchUsers(ids),
	}).To_Messages_ExportedChatInvites(), nil
}

func (c *ChatInvitesCore) oneChannelInvite(in *mtproto.TLMessagesGetExportedChatInvite, peer *mtproto.PeerUtil) (*mtproto.Messages_ExportedChatInvite, error) {
	inv, err := channel.Shared().ExportedInvite(c.ctx, c.MD.UserId, inputChannelOf(peer), in.GetLink())
	if err != nil {
		return nil, err
	}
	return c.wrapInvites([]*mtproto.ExportedChatInvite{inv})
}

func (c *ChatInvitesCore) channelImporters(in *mtproto.TLMessagesGetChatInviteImporters, peer *mtproto.PeerUtil) (*mtproto.Messages_ChatInviteImporters, error) {
	link := in.GetLink().GetValue()
	q := in.GetQ().GetValue()
	var offsetUser int64
	if in.GetOffsetUser() != nil {
		offsetUser = mtproto.FromInputUser(c.MD.UserId, in.GetOffsetUser()).PeerId
	}
	limit := in.GetLimit()
	offsetDate := in.GetOffsetDate()
	if q != "" {
		limit = 0
		offsetDate = 0
		offsetUser = 0
	}
	rows, total, err := channel.Shared().InviteImporters(c.ctx, c.MD.UserId, inputChannelOf(peer), in.GetRequested(), link, offsetDate, offsetUser, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.GetUserId())
		if row.GetApprovedBy() != nil {
			ids = append(ids, row.GetApprovedBy().GetValue())
		}
	}
	users := c.fetchUsers(ids)
	if q != "" {
		byID := map[int64]*mtproto.User{}
		for _, u := range users {
			byID[u.GetId()] = u
		}
		filtered := make([]*mtproto.ChatInviteImporter, 0, len(rows))
		for _, row := range rows {
			if userMatches(byID[row.GetUserId()], q) {
				filtered = append(filtered, row)
			}
		}
		total = int32(len(filtered))
		off := offsetUser
		if in.GetOffsetUser() != nil {
			off = mtproto.FromInputUser(c.MD.UserId, in.GetOffsetUser()).PeerId
		}
		start := 0
		if off != 0 || in.GetOffsetDate() != 0 {
			start = len(filtered)
			for i, row := range filtered {
				if row.GetUserId() == off && row.GetDate() == in.GetOffsetDate() {
					start = i + 1
					break
				}
			}
		}
		lim := in.GetLimit()
		if lim <= 0 {
			lim = 50
		}
		if start > len(filtered) {
			start = len(filtered)
		}
		end := start + int(lim)
		if end > len(filtered) {
			end = len(filtered)
		}
		rows = filtered[start:end]
		ids = ids[:0]
		for _, row := range rows {
			ids = append(ids, row.GetUserId())
		}
		users = c.fetchUsers(ids)
	}
	if rows == nil {
		rows = []*mtproto.ChatInviteImporter{}
	}
	return mtproto.MakeTLMessagesChatInviteImporters(&mtproto.Messages_ChatInviteImporters{
		Count:     total,
		Importers: rows,
		Users:     users,
	}).To_Messages_ChatInviteImporters(), nil
}

func (c *ChatInvitesCore) channelAdmins(peer *mtproto.PeerUtil) (*mtproto.Messages_ChatAdminsWithInvites, error) {
	admins, err := channel.Shared().AdminsWithInvites(c.ctx, c.MD.UserId, inputChannelOf(peer))
	if err != nil {
		return nil, err
	}
	if admins == nil {
		admins = []*mtproto.ChatAdminWithInvites{}
	}
	ids := make([]int64, 0, len(admins))
	for _, a := range admins {
		ids = append(ids, a.GetAdminId())
	}
	return mtproto.MakeTLMessagesChatAdminsWithInvites(&mtproto.Messages_ChatAdminsWithInvites{
		Admins: admins,
		Users:  c.fetchUsers(ids),
	}).To_Messages_ChatAdminsWithInvites(), nil
}

func (c *ChatInvitesCore) checkChannelInvite(hash string) (*mtproto.ChatInvite, error) {
	inv, ids, err := channel.Shared().CheckInvite(c.ctx, c.MD.UserId, hash)
	if err != nil {
		return nil, err
	}
	if inv != nil && len(ids) > 0 && inv.GetPredicateName() == mtproto.Predicate_chatInvite {
		inv.Participants = c.fetchUsers(ids)
		if len(inv.Participants) > 0 {
			// participants is flags.4. A non-empty slice sets it.
		}
	}
	return inv, nil
}

func (c *ChatInvitesCore) importChannelInvite(hash string) (*mtproto.Updates, error) {
	return channel.Shared().ImportInvite(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, hash)
}
