package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/pkg/premium"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ChatlistsExportChatlistInvite
// chatlists.exportChatlistInvite#8472478e chatlist:InputChatlist title:string peers:Vector<InputPeer> = chatlists.ExportedChatlistInvite;
// https://core.telegram.org/method/chatlists.exportChatlistInvite
func (c *FoldersCore) ChatlistsExportChatlistInvite(in *mtproto.TLChatlistsExportChatlistInvite) (*mtproto.Chatlists_ExportedChatlistInvite, error) {
	id, err := chatlistFilterID(in.GetChatlist())
	if err != nil {
		return nil, err
	}
	if len(in.GetPeers()) == 0 {
		return nil, mtproto.ErrPeersListEmpty
	}
	title, err := checkInviteTitle(in.GetTitle())
	if err != nil {
		return nil, err
	}
	filter, err := c.requireOwnChatlist(id)
	if err != nil {
		return nil, err
	}
	refs, err := c.sharePeers(filter, in.GetPeers())
	if err != nil {
		return nil, err
	}
	rows := decodeInvites(filter.GetTitle_STRING())
	if len(rows) >= c.inviteCap() {
		return nil, mtproto.ErrInvitesTooMuch
	}
	if title == "" {
		title = visibleTitle(filter)
	}
	slug, err := newInviteSlug(c.userID(), id)
	if err != nil {
		c.Logger.Errorf("chatlists.exportChatlistInvite - slug: %v", err)
		return nil, err
	}
	inv := storedInvite{
		Slug:  slug,
		Title: title,
		Date:  int32(time.Now().Unix()),
		Peers: refs,
	}
	rows = append(rows, inv)
	if err := c.putInvites(filter, rows); err != nil {
		return nil, err
	}
	return mtproto.MakeTLChatlistsExportedChatlistInvite(&mtproto.Chatlists_ExportedChatlistInvite{
		Filter: clientDialogFilter(filter),
		Invite: c.exportedInvite(filter, inv),
	}).To_Chatlists_ExportedChatlistInvite(), nil
}

// ChatlistsDeleteExportedInvite
// chatlists.deleteExportedInvite#719c5c5e chatlist:InputChatlist slug:string = Bool;
// https://core.telegram.org/method/chatlists.deleteExportedInvite
func (c *FoldersCore) ChatlistsDeleteExportedInvite(in *mtproto.TLChatlistsDeleteExportedInvite) (*mtproto.Bool, error) {
	id, err := chatlistFilterID(in.GetChatlist())
	if err != nil {
		return nil, err
	}
	slug := in.GetSlug()
	if slug == "" {
		return nil, mtproto.ErrInviteSlugExpired
	}
	filter, err := c.requireOwnChatlist(id)
	if err != nil {
		return nil, err
	}
	rows := decodeInvites(filter.GetTitle_STRING())
	next := make([]storedInvite, 0, len(rows))
	found := false
	for _, row := range rows {
		if row.Slug == slug {
			found = true
			continue
		}
		next = append(next, row)
	}
	if !found {
		return nil, mtproto.ErrInviteSlugExpired
	}
	if err := c.putInvites(filter, next); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// ChatlistsEditExportedInvite
// chatlists.editExportedInvite#653db63d flags:# chatlist:InputChatlist slug:string title:flags.1?string peers:flags.2?Vector<InputPeer> = ExportedChatlistInvite;
// https://core.telegram.org/method/chatlists.editExportedInvite
func (c *FoldersCore) ChatlistsEditExportedInvite(in *mtproto.TLChatlistsEditExportedInvite) (*mtproto.ExportedChatlistInvite, error) {
	id, err := chatlistFilterID(in.GetChatlist())
	if err != nil {
		return nil, err
	}
	if in.GetSlug() == "" {
		return nil, mtproto.ErrInviteSlugExpired
	}
	if in.GetPeers() != nil && len(in.GetPeers()) == 0 {
		return nil, mtproto.ErrPeersListEmpty
	}
	var title string
	if in.GetTitle() != nil {
		title, err = checkInviteTitle(in.GetTitle().GetValue())
		if err != nil {
			return nil, err
		}
	}
	filter, err := c.requireOwnChatlist(id)
	if err != nil {
		return nil, err
	}
	rows := decodeInvites(filter.GetTitle_STRING())
	idx := -1
	for i := range rows {
		if rows[i].Slug == in.GetSlug() {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, mtproto.ErrInviteSlugExpired
	}
	if in.GetTitle() != nil {
		if title == "" {
			title = visibleTitle(filter)
		}
		rows[idx].Title = title
	}
	if in.GetPeers() != nil {
		refs, err := c.sharePeers(filter, in.GetPeers())
		if err != nil {
			return nil, err
		}
		rows[idx].Peers = refs
	}
	if err := c.putInvites(filter, rows); err != nil {
		return nil, err
	}
	return c.exportedInvite(filter, rows[idx]), nil
}

// ChatlistsGetExportedInvites
// chatlists.getExportedInvites#ce03da83 chatlist:InputChatlist = chatlists.ExportedInvites;
// https://core.telegram.org/method/chatlists.getExportedInvites
func (c *FoldersCore) ChatlistsGetExportedInvites(in *mtproto.TLChatlistsGetExportedInvites) (*mtproto.Chatlists_ExportedInvites, error) {
	id, err := chatlistFilterID(in.GetChatlist())
	if err != nil {
		return nil, err
	}
	filter, err := c.requireOwnChatlist(id)
	if err != nil {
		return nil, err
	}
	rows := decodeInvites(filter.GetTitle_STRING())
	invites := make([]*mtproto.ExportedChatlistInvite, 0, len(rows))
	var refs []peerRef
	for _, row := range rows {
		invites = append(invites, c.exportedInvite(filter, row))
		refs = append(refs, row.Peers...)
	}
	users, chats := c.peerSide(dedupeRefs(refs))
	return mtproto.MakeTLChatlistsExportedInvites(&mtproto.Chatlists_ExportedInvites{
		Invites: invites,
		Chats:   chats,
		Users:   users,
	}).To_Chatlists_ExportedInvites(), nil
}

// ChatlistsCheckChatlistInvite
// chatlists.checkChatlistInvite#41c10fff slug:string = chatlists.ChatlistInvite;
// https://core.telegram.org/method/chatlists.checkChatlistInvite
func (c *FoldersCore) ChatlistsCheckChatlistInvite(in *mtproto.TLChatlistsCheckChatlistInvite) (*mtproto.Chatlists_ChatlistInvite, error) {
	owner, filter, inv, err := c.lookupInvite(in.GetSlug())
	if err != nil {
		return nil, err
	}
	if owner == c.userID() {
		return c.inviteCard(filter, inv, filter), nil
	}
	joined, err := c.joinedBySlug(inv.Slug)
	if err != nil {
		return nil, err
	}
	return c.inviteCard(filter, inv, joined), nil
}

// ChatlistsJoinChatlistInvite
// chatlists.joinChatlistInvite#a6b1e39a slug:string peers:Vector<InputPeer> = Updates;
// https://core.telegram.org/method/chatlists.joinChatlistInvite
func (c *FoldersCore) ChatlistsJoinChatlistInvite(in *mtproto.TLChatlistsJoinChatlistInvite) (*mtproto.Updates, error) {
	owner, src, inv, err := c.lookupInvite(in.GetSlug())
	if err != nil {
		return nil, err
	}
	if owner == c.userID() {
		return updatesOf(c.filterUpdate(src.GetId(), src)), nil
	}
	picked, err := c.pickInvitePeers(inv, in.GetPeers())
	if err != nil {
		return nil, err
	}
	if len(picked) > c.channelCap() {
		return nil, mtproto.ErrChannelsTooMuch
	}
	existing, err := c.joinedBySlug(inv.Slug)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if err := c.mergeJoinedPeers(existing, picked); err != nil {
			return nil, err
		}
		return updatesOf(c.filterUpdate(existing.GetId(), existing)), nil
	}
	rows, err := c.rawFilters()
	if err != nil {
		return nil, err
	}
	if c.countJoined(rows) >= c.joinedCap() || len(rows) >= premium.Limit(premium.FeatureDialogFilters, c.premiumMember()) {
		return nil, mtproto.ErrChatlistsTooMuch
	}
	title := inv.Title
	if title == "" {
		title = visibleTitle(src)
	}
	created := mtproto.MakeTLDialogFilterChatlist(&mtproto.DialogFilter{
		Title:                  title,
		Title_TEXTWITHENTITIES: textTitle(title),
		TitleNoanimate:         src.GetTitleNoanimate(),
		Emoticon:               cloneEmoticon(src.GetEmoticon()),
		Color:                  cloneInt32(src.GetColor()),
		IncludePeers:           inputsOf(picked),
		PinnedPeers:            []*mtproto.InputPeer{},
		Title_STRING: encodeJoin(joinState{
			Slug:   inv.Slug,
			Owner:  owner,
			Filter: src.GetId(),
		}),
	}).To_DialogFilter()
	applyColorPolicy(created, nil, c.allowFolderColor())
	ext, err := c.svcCtx.Dao.DialogClient.DialogCreateDialogFilter(c.ctx, &dialog.TLDialogCreateDialogFilter{
		UserId: c.userID(),
		DialogFilter: &dialog.DialogFilterExt{
			Slug:         inv.Slug,
			JoinedBySlug: true,
			DialogFilter: created,
		},
	})
	if err != nil {
		c.Logger.Errorf("chatlists.joinChatlistInvite - error: %v", err)
		return nil, err
	}
	stored := created
	if ext != nil && ext.GetDialogFilter() != nil {
		stored = ext.GetDialogFilter()
	}
	c.pushNotMe(c.filterUpdate(stored.GetId(), stored))
	return updatesOf(c.filterUpdate(stored.GetId(), stored)), nil
}

// ChatlistsGetChatlistUpdates
// chatlists.getChatlistUpdates#89419521 chatlist:InputChatlist = chatlists.ChatlistUpdates;
// https://core.telegram.org/method/chatlists.getChatlistUpdates
func (c *FoldersCore) ChatlistsGetChatlistUpdates(in *mtproto.TLChatlistsGetChatlistUpdates) (*mtproto.Chatlists_ChatlistUpdates, error) {
	id, err := chatlistFilterID(in.GetChatlist())
	if err != nil {
		return nil, err
	}
	joined, st, err := c.requireJoinedChatlist(id)
	if err != nil {
		return nil, err
	}
	_, src, inv, err := c.lookupInvite(st.Slug)
	if err != nil {
		if err == mtproto.ErrInviteSlugExpired || sameRPC(err, mtproto.ErrInviteSlugExpired) {
			return emptyChatlistUpdates(), nil
		}
		return nil, err
	}
	missing := missingRefs(inv, joined, st.Hidden)
	users, chats := c.peerSide(missing)
	_ = src
	return mtproto.MakeTLChatlistsChatlistUpdates(&mtproto.Chatlists_ChatlistUpdates{
		MissingPeers: peersOf(missing),
		Chats:        chats,
		Users:        users,
	}).To_Chatlists_ChatlistUpdates(), nil
}

// ChatlistsJoinChatlistUpdates
// chatlists.joinChatlistUpdates#e089f8f5 chatlist:InputChatlist peers:Vector<InputPeer> = Updates;
// https://core.telegram.org/method/chatlists.joinChatlistUpdates
func (c *FoldersCore) ChatlistsJoinChatlistUpdates(in *mtproto.TLChatlistsJoinChatlistUpdates) (*mtproto.Updates, error) {
	id, err := chatlistFilterID(in.GetChatlist())
	if err != nil {
		return nil, err
	}
	if len(in.GetPeers()) == 0 {
		return nil, mtproto.ErrPeersListEmpty
	}
	joined, st, err := c.requireJoinedChatlist(id)
	if err != nil {
		return nil, err
	}
	_, _, inv, err := c.lookupInvite(st.Slug)
	if err != nil {
		return nil, err
	}
	picked, err := c.pickInvitePeers(inv, in.GetPeers())
	if err != nil {
		return nil, err
	}
	open := map[string]struct{}{}
	for _, ref := range missingRefs(inv, joined, st.Hidden) {
		open[ref.key()] = struct{}{}
	}
	var add []peerRef
	for _, ref := range picked {
		if _, ok := open[ref.key()]; ok {
			add = append(add, ref)
		}
	}
	if len(add) == 0 {
		return nil, mtproto.ErrFilterIncludeEmpty
	}
	if len(add) > c.channelCap() {
		return nil, mtproto.ErrChannelsTooMuch
	}
	if err := c.mergeJoinedPeers(joined, add); err != nil {
		return nil, err
	}
	return updatesOf(c.filterUpdate(joined.GetId(), joined)), nil
}

// ChatlistsHideChatlistUpdates
// chatlists.hideChatlistUpdates#66e486fb chatlist:InputChatlist = Bool;
// https://core.telegram.org/method/chatlists.hideChatlistUpdates
func (c *FoldersCore) ChatlistsHideChatlistUpdates(in *mtproto.TLChatlistsHideChatlistUpdates) (*mtproto.Bool, error) {
	id, err := chatlistFilterID(in.GetChatlist())
	if err != nil {
		return nil, err
	}
	joined, st, err := c.requireJoinedChatlist(id)
	if err != nil {
		return nil, err
	}
	_, _, inv, err := c.lookupInvite(st.Slug)
	if err != nil {
		if err == mtproto.ErrInviteSlugExpired || sameRPC(err, mtproto.ErrInviteSlugExpired) {
			return mtproto.BoolTrue, nil
		}
		return nil, err
	}
	// Hide the peers that are new right now. Peers the owner adds later still surface.
	st.Hidden = dedupeRefs(append(st.Hidden, missingRefs(inv, joined, nil)...))
	joined.Title_STRING = encodeJoin(st)
	if err := c.saveOwnChatlist(joined); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// ChatlistsGetLeaveChatlistSuggestions
// chatlists.getLeaveChatlistSuggestions#fdbcd714 chatlist:InputChatlist = Vector<Peer>;
// https://core.telegram.org/method/chatlists.getLeaveChatlistSuggestions
func (c *FoldersCore) ChatlistsGetLeaveChatlistSuggestions(in *mtproto.TLChatlistsGetLeaveChatlistSuggestions) (*mtproto.Vector_Peer, error) {
	id, err := chatlistFilterID(in.GetChatlist())
	if err != nil {
		return nil, err
	}
	joined, st, err := c.requireJoinedChatlist(id)
	if err != nil {
		return nil, err
	}
	have := folderPeerRefs(joined)
	var suggest []peerRef
	if _, _, inv, err := c.lookupInvite(st.Slug); err == nil {
		for _, ref := range inv.Peers {
			if _, ok := have[ref.key()]; ok {
				suggest = append(suggest, ref)
			}
		}
	} else if !sameRPC(err, mtproto.ErrInviteSlugExpired) {
		return nil, err
	} else {
		for _, ref := range have {
			suggest = append(suggest, ref)
		}
	}
	return &mtproto.Vector_Peer{Datas: peersOf(suggest)}, nil
}

// ChatlistsLeaveChatlist
// chatlists.leaveChatlist#74fae13a chatlist:InputChatlist peers:Vector<InputPeer> = Updates;
// https://core.telegram.org/method/chatlists.leaveChatlist
func (c *FoldersCore) ChatlistsLeaveChatlist(in *mtproto.TLChatlistsLeaveChatlist) (*mtproto.Updates, error) {
	id, err := chatlistFilterID(in.GetChatlist())
	if err != nil {
		return nil, err
	}
	joined, _, err := c.requireJoinedChatlist(id)
	if err != nil {
		return nil, err
	}
	have := folderPeerRefs(joined)
	var leaving []peerRef
	for _, peer := range in.GetPeers() {
		ref, err := refFromInput(c.userID(), peer)
		if err != nil {
			return nil, err
		}
		if _, ok := have[ref.key()]; !ok {
			continue
		}
		leaving = append(leaving, ref)
	}
	if _, err := c.svcCtx.Dao.DialogClient.DialogDeleteDialogFilter(c.ctx, &dialog.TLDialogDeleteDialogFilter{
		UserId: c.userID(),
		Id:     id,
	}); err != nil {
		c.Logger.Errorf("chatlists.leaveChatlist - error: %v", err)
		return nil, err
	}
	for _, ref := range dedupeRefs(leaving) {
		if _, err := c.svcCtx.Dao.DialogClient.DialogDeleteDialog(c.ctx, &dialog.TLDialogDeleteDialog{
			UserId:   c.userID(),
			PeerType: ref.peerType(),
			PeerId:   ref.ID,
		}); err != nil {
			c.Logger.Errorf("chatlists.leaveChatlist - dialog %s: %v", ref.key(), err)
		}
	}
	upd := c.filterUpdate(id, nil)
	c.pushNotMe(upd)
	return updatesOf(upd), nil
}

func (c *FoldersCore) pickInvitePeers(inv storedInvite, peers []*mtproto.InputPeer) ([]peerRef, error) {
	if len(peers) == 0 {
		return nil, mtproto.ErrFilterIncludeEmpty
	}
	allowed := map[string]peerRef{}
	for _, ref := range inv.Peers {
		allowed[ref.key()] = ref
	}
	var out []peerRef
	for _, peer := range peers {
		ref, err := refFromInput(c.userID(), peer)
		if err != nil {
			return nil, err
		}
		if _, ok := allowed[ref.key()]; !ok {
			continue
		}
		out = append(out, ref)
	}
	out = dedupeRefs(out)
	if len(out) == 0 {
		return nil, mtproto.ErrFilterIncludeEmpty
	}
	return out, nil
}

func (c *FoldersCore) mergeJoinedPeers(filter *mtproto.DialogFilter, add []peerRef) error {
	st, ok := decodeJoin(filter.GetTitle_STRING())
	if !ok {
		return mtproto.ErrFilterNotSupported
	}
	have := folderPeerRefs(filter)
	include := append([]*mtproto.InputPeer{}, filter.GetIncludePeers()...)
	for _, ref := range add {
		if _, exists := have[ref.key()]; exists {
			continue
		}
		if peer := ref.input(); peer != nil {
			include = append(include, peer)
		}
	}
	filter.IncludePeers = include
	filter.PredicateName = mtproto.Predicate_dialogFilterChatlist
	st.Hidden = subtractRefs(st.Hidden, add)
	filter.Title_STRING = encodeJoin(st)
	return c.saveOwnChatlist(filter)
}

func emptyChatlistUpdates() *mtproto.Chatlists_ChatlistUpdates {
	return mtproto.MakeTLChatlistsChatlistUpdates(&mtproto.Chatlists_ChatlistUpdates{
		MissingPeers: []*mtproto.Peer{},
		Chats:        []*mtproto.Chat{},
		Users:        []*mtproto.User{},
	}).To_Chatlists_ChatlistUpdates()
}

func cloneEmoticon(v *wrapperspb.StringValue) *wrapperspb.StringValue {
	if v == nil || v.GetValue() == "" {
		return nil
	}
	return wrapperspb.String(v.GetValue())
}

func cloneInt32(v *wrapperspb.Int32Value) *wrapperspb.Int32Value {
	if v == nil {
		return nil
	}
	return wrapperspb.Int32(v.GetValue())
}
