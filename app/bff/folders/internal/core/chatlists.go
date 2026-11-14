package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/archive"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/teamgram/teamgram-server/pkg/premium"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Official client configuration defaults.
// https://core.telegram.org/api/config#chatlist-invites-limit-default
// https://core.telegram.org/api/config#chatlists-joined-limit-default
const (
	chatlistInvitesFree    = 3
	chatlistInvitesPremium = 100
	chatlistsJoinedFree    = 2
	chatlistsJoinedPremium = 20
	chatlistInviteTitleMax = 255
)

// slugRe is c{ownerUserId}_{filterId}_{16 hex}. The owner id is in the slug so
// checkChatlistInvite can load the folder without a global invite table.
var slugRe = regexp.MustCompile(`^c([0-9]+)_([0-9]+)_([0-9a-f]{16})$`)

type peerRef struct {
	Kind string `json:"k"`
	ID   int64  `json:"id"`
	Hash int64  `json:"h,omitempty"`
}

func (r peerRef) key() string {
	return r.Kind + ":" + strconv.FormatInt(r.ID, 10)
}

func (r peerRef) peer() *mtproto.Peer {
	switch r.Kind {
	case "user":
		return mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: r.ID}).To_Peer()
	case "chat":
		return mtproto.MakeTLPeerChat(&mtproto.Peer{ChatId: r.ID}).To_Peer()
	case "channel":
		return mtproto.MakeTLPeerChannel(&mtproto.Peer{ChannelId: r.ID}).To_Peer()
	default:
		return nil
	}
}

func (r peerRef) input() *mtproto.InputPeer {
	switch r.Kind {
	case "user":
		return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: r.ID, AccessHash: r.Hash}).To_InputPeer()
	case "chat":
		return mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: r.ID}).To_InputPeer()
	case "channel":
		return mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: r.ID, AccessHash: r.Hash}).To_InputPeer()
	default:
		return nil
	}
}

func (r peerRef) dialogID() int64 {
	switch r.Kind {
	case "user":
		return mtproto.MakePeerDialogId(mtproto.PEER_USER, r.ID)
	case "chat":
		return mtproto.MakePeerDialogId(mtproto.PEER_CHAT, r.ID)
	case "channel":
		return mtproto.MakePeerDialogId(mtproto.PEER_CHANNEL, r.ID)
	default:
		return 0
	}
}

func (r peerRef) peerType() int32 {
	switch r.Kind {
	case "user":
		return mtproto.PEER_USER
	case "chat":
		return mtproto.PEER_CHAT
	case "channel":
		return mtproto.PEER_CHANNEL
	default:
		return mtproto.PEER_EMPTY
	}
}

type storedInvite struct {
	Slug  string    `json:"slug"`
	Title string    `json:"title"`
	Date  int32     `json:"date"`
	Peers []peerRef `json:"peers"`
}

type joinState struct {
	Slug   string    `json:"slug"`
	Owner  int64     `json:"owner"`
	Filter int32     `json:"filter"`
	Hidden []peerRef `json:"hidden,omitempty"`
}

func sameRPC(err, target error) bool {
	if err == nil || target == nil {
		return false
	}
	if err == target {
		return true
	}
	return status.Convert(err).Message() == status.Convert(target).Message() && status.Convert(target).Message() != ""
}

func parseSlug(slug string) (owner int64, filterID int32, ok bool) {
	m := slugRe.FindStringSubmatch(strings.TrimSpace(slug))
	if m == nil {
		return 0, 0, false
	}
	owner, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || owner <= 0 {
		return 0, 0, false
	}
	id, err := strconv.ParseInt(m[2], 10, 32)
	if err != nil || id <= 0 {
		return 0, 0, false
	}
	return owner, int32(id), true
}

func newInviteSlug(owner int64, filterID int32) (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "c" + strconv.FormatInt(owner, 10) + "_" + strconv.FormatInt(int64(filterID), 10) + "_" + hex.EncodeToString(buf[:]), nil
}

func inviteURL(slug string) string {
	return brand.LinkBase() + "/addlist/" + slug
}

func decodeInvites(blob string) []storedInvite {
	raw, ok := strings.CutPrefix(blob, mtproto.ChatlistInvitesBlobPrefix)
	if !ok || raw == "" {
		return nil
	}
	var rows []storedInvite
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	out := make([]storedInvite, 0, len(rows))
	for _, row := range rows {
		if row.Slug == "" {
			continue
		}
		if row.Peers == nil {
			row.Peers = []peerRef{}
		}
		out = append(out, row)
	}
	return out
}

func encodeInvites(rows []storedInvite) string {
	if rows == nil {
		rows = []storedInvite{}
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return mtproto.ChatlistInvitesBlobPrefix + "[]"
	}
	return mtproto.ChatlistInvitesBlobPrefix + string(raw)
}

func decodeJoin(blob string) (joinState, bool) {
	raw, ok := strings.CutPrefix(blob, mtproto.ChatlistJoinBlobPrefix)
	if !ok || raw == "" {
		return joinState{}, false
	}
	var st joinState
	if err := json.Unmarshal([]byte(raw), &st); err != nil || st.Slug == "" || st.Owner <= 0 || st.Filter <= 0 {
		return joinState{}, false
	}
	return st, true
}

func encodeJoin(st joinState) string {
	raw, err := json.Marshal(st)
	if err != nil {
		return mtproto.ChatlistJoinBlobPrefix + "{}"
	}
	return mtproto.ChatlistJoinBlobPrefix + string(raw)
}

func chatlistFilterID(in *mtproto.InputChatlist) (int32, error) {
	if in == nil {
		return 0, mtproto.ErrFilterIdInvalid
	}
	name := in.GetPredicateName()
	if name != "" && name != mtproto.Predicate_inputChatlistDialogFilter {
		return 0, mtproto.ErrFilterIdInvalid
	}
	if in.GetFilterId() <= 0 {
		return 0, mtproto.ErrFilterIdInvalid
	}
	return in.GetFilterId(), nil
}

func (c *FoldersCore) inviteCap() int {
	if c.premiumMember() {
		return chatlistInvitesPremium
	}
	return chatlistInvitesFree
}

func (c *FoldersCore) joinedCap() int {
	if c.premiumMember() {
		return chatlistsJoinedPremium
	}
	return chatlistsJoinedFree
}

func textTitle(s string) *mtproto.TextWithEntities {
	return mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
		Text:     s,
		Entities: []*mtproto.MessageEntity{},
	}).To_TextWithEntities()
}

func visibleTitle(filter *mtproto.DialogFilter) string {
	if filter == nil {
		return ""
	}
	if t := filter.GetTitle_TEXTWITHENTITIES(); t != nil {
		if text := strings.TrimSpace(t.GetText()); text != "" && !mtproto.IsChatlistBlob(text) {
			return text
		}
	}
	if text := strings.TrimSpace(filter.GetTitle()); text != "" && !mtproto.IsChatlistBlob(text) {
		return text
	}
	if text := strings.TrimSpace(filter.GetTitle_STRING()); text != "" && !mtproto.IsChatlistBlob(text) {
		return text
	}
	return ""
}

func clientDialogFilter(in *mtproto.DialogFilter) *mtproto.DialogFilter {
	if in == nil {
		return nil
	}
	cp := *in
	cp.FixData()
	mtproto.StripChatlistBlob(&cp)
	if cp.GetPredicateName() == mtproto.Predicate_dialogFilterChatlist {
		return mtproto.MakeTLDialogFilterChatlist(&cp).To_DialogFilter()
	}
	return mtproto.MakeTLDialogFilter(&cp).To_DialogFilter()
}

func (c *FoldersCore) filterUpdate(id int32, filter *mtproto.DialogFilter) *mtproto.Update {
	upd := &mtproto.Update{Id_INT32: id}
	if filter != nil {
		upd.Filter = clientDialogFilter(filter)
	}
	return mtproto.MakeTLUpdateDialogFilter(upd).To_Update()
}

func updatesOf(upds ...*mtproto.Update) *mtproto.Updates {
	if len(upds) == 0 {
		return mtproto.MakeEmptyUpdates()
	}
	return mtproto.MakeUpdatesByUpdates(upds...)
}

// folderPeerUpdates is the folders.editPeerFolders result. pts is the user's
// next pts and pts_count is 1 when that pts was stored in user_pts_updates.
// Other sessions receive the same update. A session that misses the push
// loads it from updates.getDifference. pts 0 is only used when idgen is not
// configured; clients then apply the folder move without a difference gap.
func folderPeerUpdates(peers []*mtproto.FolderPeer, pts int32) *mtproto.Updates {
	return mtproto.MakeUpdatesByUpdates(archive.NewFolderPeersUpdate(peers, pts))
}

func refFromInput(self int64, peer *mtproto.InputPeer) (peerRef, error) {
	if peer == nil {
		return peerRef{}, mtproto.ErrPeerIdInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerUserFromMessage, mtproto.Predicate_inputPeerChannelFromMessage:
		if peer.GetMsgId() <= 0 {
			return peerRef{}, mtproto.ErrMsgIdInvalid
		}
		return refFromInput(self, peer.GetPeer())
	case mtproto.Predicate_inputPeerSelf:
		if self <= 0 {
			return peerRef{}, mtproto.ErrPeerIdInvalid
		}
		return peerRef{Kind: "user", ID: self}, nil
	case mtproto.Predicate_inputPeerUser:
		if peer.GetUserId() <= 0 {
			return peerRef{}, mtproto.ErrPeerIdInvalid
		}
		return peerRef{Kind: "user", ID: peer.GetUserId(), Hash: peer.GetAccessHash()}, nil
	case mtproto.Predicate_inputPeerChat:
		if peer.GetChatId() <= 0 {
			return peerRef{}, mtproto.ErrChatIdInvalid
		}
		return peerRef{Kind: "chat", ID: peer.GetChatId()}, nil
	case mtproto.Predicate_inputPeerChannel:
		if peer.GetChannelId() <= 0 {
			return peerRef{}, mtproto.ErrChannelInvalid
		}
		return peerRef{Kind: "channel", ID: peer.GetChannelId(), Hash: peer.GetAccessHash()}, nil
	default:
		return peerRef{}, mtproto.ErrPeerIdInvalid
	}
}

func folderPeerRefs(filter *mtproto.DialogFilter) map[string]peerRef {
	out := map[string]peerRef{}
	if filter == nil {
		return out
	}
	add := func(peers []*mtproto.InputPeer) {
		for _, peer := range peers {
			ref, err := refFromInput(0, peer)
			if err != nil || ref.Kind == "user" && peer.GetPredicateName() == mtproto.Predicate_inputPeerSelf {
				continue
			}
			if ref.Kind == "" {
				continue
			}
			if prev, ok := out[ref.key()]; ok && ref.Hash == 0 {
				ref.Hash = prev.Hash
			}
			out[ref.key()] = ref
		}
	}
	add(filter.GetIncludePeers())
	add(filter.GetPinnedPeers())
	return out
}

func peersOf(refs []peerRef) []*mtproto.Peer {
	out := make([]*mtproto.Peer, 0, len(refs))
	for _, ref := range refs {
		if peer := ref.peer(); peer != nil {
			out = append(out, peer)
		}
	}
	return out
}

func inputsOf(refs []peerRef) []*mtproto.InputPeer {
	out := make([]*mtproto.InputPeer, 0, len(refs))
	for _, ref := range refs {
		if peer := ref.input(); peer != nil {
			out = append(out, peer)
		}
	}
	return out
}

func dedupeRefs(refs []peerRef) []peerRef {
	seen := map[string]int{}
	out := make([]peerRef, 0, len(refs))
	for _, ref := range refs {
		if ref.Kind == "" || ref.ID <= 0 {
			continue
		}
		if i, ok := seen[ref.key()]; ok {
			if out[i].Hash == 0 && ref.Hash != 0 {
				out[i].Hash = ref.Hash
			}
			continue
		}
		seen[ref.key()] = len(out)
		out = append(out, ref)
	}
	return out
}

func subtractRefs(all, drop []peerRef) []peerRef {
	gone := map[string]struct{}{}
	for _, ref := range drop {
		gone[ref.key()] = struct{}{}
	}
	out := make([]peerRef, 0, len(all))
	for _, ref := range all {
		if _, ok := gone[ref.key()]; ok {
			continue
		}
		out = append(out, ref)
	}
	return out
}

func (c *FoldersCore) rawFilters() ([]*dialog.DialogFilterExt, error) {
	res, err := c.svcCtx.Dao.DialogClient.DialogGetDialogFilters(c.ctx, &dialog.TLDialogGetDialogFilters{
		UserId: c.userID(),
	})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	return res.GetDatas(), nil
}

func (c *FoldersCore) loadOwnFilter(id int32) (*mtproto.DialogFilter, bool, error) {
	row, err := c.svcCtx.Dao.DialogClient.DialogGetDialogFilter(c.ctx, &dialog.TLDialogGetDialogFilter{
		UserId: c.userID(),
		Id:     id,
	})
	if err != nil {
		if sameRPC(err, mtproto.ErrFilterIdInvalid) {
			return nil, false, mtproto.ErrFilterIdInvalid
		}
		return nil, false, err
	}
	if row == nil || row.GetDialogFilter() == nil {
		return nil, false, mtproto.ErrFilterIdInvalid
	}
	filter := row.GetDialogFilter()
	joined := row.GetJoinedBySlug() || strings.HasPrefix(filter.GetTitle_STRING(), mtproto.ChatlistJoinBlobPrefix)
	return filter, joined, nil
}

func (c *FoldersCore) requireOwnChatlist(id int32) (*mtproto.DialogFilter, error) {
	filter, joined, err := c.loadOwnFilter(id)
	if err != nil {
		return nil, err
	}
	if joined || filter.GetPredicateName() == mtproto.Predicate_dialogFilterDefault {
		return nil, mtproto.ErrFilterNotSupported
	}
	if filter.GetPredicateName() == mtproto.Predicate_dialogFilterChatlist {
		return filter, nil
	}
	// Official export turns a normal dialogFilter into dialogFilterChatlist.
	// contacts/groups/exclude flags do not belong on a shared folder. The
	// concrete include and pinned peers stay.
	// https://core.telegram.org/api/folders
	promoted := promoteToChatlist(filter)
	if err := c.saveOwnChatlist(promoted); err != nil {
		return nil, err
	}
	return promoted, nil
}

// promoteToChatlist copies the folder fields a dialogFilterChatlist can store.
func promoteToChatlist(filter *mtproto.DialogFilter) *mtproto.DialogFilter {
	if filter == nil {
		return nil
	}
	return mtproto.MakeTLDialogFilterChatlist(&mtproto.DialogFilter{
		TitleNoanimate:         filter.GetTitleNoanimate(),
		Id:                     filter.GetId(),
		Title:                  filter.GetTitle(),
		Title_TEXTWITHENTITIES: filter.GetTitle_TEXTWITHENTITIES(),
		Title_STRING:           filter.GetTitle_STRING(),
		Emoticon:               filter.GetEmoticon(),
		Color:                  filter.GetColor(),
		PinnedPeers:            filter.GetPinnedPeers(),
		IncludePeers:           filter.GetIncludePeers(),
	}).To_DialogFilter()
}

func (c *FoldersCore) requireJoinedChatlist(id int32) (*mtproto.DialogFilter, joinState, error) {
	filter, joined, err := c.loadOwnFilter(id)
	if err != nil {
		return nil, joinState{}, err
	}
	if filter.GetPredicateName() != mtproto.Predicate_dialogFilterChatlist || !joined {
		return nil, joinState{}, mtproto.ErrFilterNotSupported
	}
	st, ok := decodeJoin(filter.GetTitle_STRING())
	if !ok {
		return nil, joinState{}, mtproto.ErrFilterNotSupported
	}
	return filter, st, nil
}

func (c *FoldersCore) loadOwnerFilter(owner int64, id int32) (*mtproto.DialogFilter, error) {
	row, err := c.svcCtx.Dao.DialogClient.DialogGetDialogFilter(c.ctx, &dialog.TLDialogGetDialogFilter{
		UserId: owner,
		Id:     id,
	})
	if err != nil {
		if sameRPC(err, mtproto.ErrFilterIdInvalid) {
			return nil, mtproto.ErrInviteSlugExpired
		}
		return nil, err
	}
	if row == nil || row.GetDialogFilter() == nil {
		return nil, mtproto.ErrInviteSlugExpired
	}
	return row.GetDialogFilter(), nil
}

func findInvite(filter *mtproto.DialogFilter, slug string) (storedInvite, bool) {
	for _, inv := range decodeInvites(filter.GetTitle_STRING()) {
		if inv.Slug == slug {
			return inv, true
		}
	}
	return storedInvite{}, false
}

func (c *FoldersCore) lookupInvite(slug string) (owner int64, filter *mtproto.DialogFilter, inv storedInvite, err error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return 0, nil, storedInvite{}, mtproto.ErrInviteSlugEmpty
	}
	owner, id, ok := parseSlug(slug)
	if !ok {
		return 0, nil, storedInvite{}, mtproto.ErrInviteSlugExpired
	}
	filter, err = c.loadOwnerFilter(owner, id)
	if err != nil {
		return 0, nil, storedInvite{}, err
	}
	inv, ok = findInvite(filter, slug)
	if !ok {
		return 0, nil, storedInvite{}, mtproto.ErrInviteSlugExpired
	}
	return owner, filter, inv, nil
}

func (c *FoldersCore) joinedBySlug(slug string) (*mtproto.DialogFilter, error) {
	row, err := c.svcCtx.Dao.DialogClient.DialogGetDialogFilterBySlug(c.ctx, &dialog.TLDialogGetDialogFilterBySlug{
		UserId: c.userID(),
		Slug:   slug,
	})
	if err != nil {
		if sameRPC(err, mtproto.ErrInviteSlugEmpty) || sameRPC(err, mtproto.ErrFilterIdInvalid) {
			return nil, nil
		}
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	return row.GetDialogFilter(), nil
}

func (c *FoldersCore) dialogsPresent(ids []int64) (map[int64]struct{}, error) {
	found := map[int64]struct{}{}
	if len(ids) == 0 {
		return found, nil
	}
	res, err := c.svcCtx.Dao.DialogClient.DialogGetDialogsByIdList(c.ctx, &dialog.TLDialogGetDialogsByIdList{
		UserId: c.userID(),
		IdList: ids,
	})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return found, nil
	}
	for _, row := range res.GetDatas() {
		// getDialogsByIdList invents a row with top_message 0 when the
		// peer has never been a dialog. That row is not on the chat list.
		if row.GetDialog().GetTopMessage() == 0 {
			continue
		}
		id := dialogPeerID(row)
		if id != 0 {
			found[id] = struct{}{}
		}
	}
	return found, nil
}

func dialogPeerID(ext *dialog.DialogExt) int64 {
	if ext == nil || ext.GetDialog() == nil || ext.GetDialog().GetPeer() == nil {
		return 0
	}
	peer := ext.GetDialog().GetPeer()
	switch peer.GetPredicateName() {
	case mtproto.Predicate_peerUser:
		return mtproto.MakePeerDialogId(mtproto.PEER_USER, peer.GetUserId())
	case mtproto.Predicate_peerChat:
		return mtproto.MakePeerDialogId(mtproto.PEER_CHAT, peer.GetChatId())
	case mtproto.Predicate_peerChannel:
		return mtproto.MakePeerDialogId(mtproto.PEER_CHANNEL, peer.GetChannelId())
	default:
		return 0
	}
}

// sharePeers checks export/edit peers. Groups and channels the user can share
// are added to the folder when they are not already in include_peers.
// A basic group is converted to a supergroup, which is what exportChatlistInvite
// does. A peer that is not in the folder is not CHANNEL_INVALID.
// https://core.telegram.org/method/chatlists.exportChatlistInvite
// https://core.telegram.org/method/chatlists.editExportedInvite
func (c *FoldersCore) sharePeers(filter *mtproto.DialogFilter, peers []*mtproto.InputPeer) ([]peerRef, error) {
	if len(peers) == 0 {
		return nil, mtproto.ErrPeersListEmpty
	}
	var refs []peerRef
	replacements := map[string]peerRef{}
	for _, peer := range peers {
		ref, err := refFromInput(c.userID(), peer)
		if err != nil {
			return nil, err
		}
		resolved, replaced, err := c.resolveShareRef(ref)
		if err != nil {
			return nil, err
		}
		refs = append(refs, resolved)
		if replaced.Kind != "" && replaced.key() != resolved.key() {
			replacements[replaced.key()] = resolved
		}
	}
	refs = dedupeRefs(refs)
	if len(refs) == 0 {
		return nil, mtproto.ErrPeersListEmpty
	}
	applyPeerReplacements(filter, replacements, refs)
	return refs, nil
}

func (c *FoldersCore) resolveShareRef(ref peerRef) (resolved, replaced peerRef, err error) {
	switch ref.Kind {
	case "channel":
		if err = channel.Shared().Shareable(c.ctx, c.userID(), ref.ID); err != nil {
			return peerRef{}, peerRef{}, err
		}
		if hash, ok, hashErr := channel.Shared().IssuedAccessHash(c.ctx, ref.ID); hashErr == nil && ok {
			ref.Hash = hash
		}
		return ref, peerRef{}, nil
	case "chat":
		return c.resolveShareChat(ref)
	default:
		return peerRef{}, peerRef{}, mtproto.ErrPeerIdInvalid
	}
}

// resolveShareChat turns a basic group into the supergroup exportChatlistInvite
// shares. The creator is required, matching messages.migrateChat.
func (c *FoldersCore) resolveShareChat(ref peerRef) (resolved, replaced peerRef, err error) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil {
		return peerRef{}, peerRef{}, mtproto.ErrChatIdInvalid
	}
	row, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chat.TLChatGetMutableChat{ChatId: ref.ID})
	if err != nil || row == nil {
		return peerRef{}, peerRef{}, mtproto.ErrChatIdInvalid
	}
	if migrated := row.MigratedTo(); migrated.GetChannelId() != 0 {
		next := peerRef{Kind: "channel", ID: migrated.GetChannelId(), Hash: migrated.GetAccessHash()}
		if err = channel.Shared().Shareable(c.ctx, c.userID(), next.ID); err != nil {
			return peerRef{}, peerRef{}, err
		}
		return next, ref, nil
	}
	if row.Creator() != c.userID() {
		return peerRef{}, peerRef{}, mtproto.ErrChatAdminRequired
	}
	var members []channel.LegacyMember
	row.Walk(func(userID int64, participant *mtproto.ImmutableChatParticipant) error {
		if participant == nil || !participant.IsChatMemberStateNormal() {
			return nil
		}
		members = append(members, channel.LegacyMember{UserID: userID, IsBot: participant.GetIsBot()})
		return nil
	})
	result, err := channel.Shared().Migrate(c.ctx, channel.MigrateRequest{
		Actor:     c.userID(),
		ActorAuth: c.permAuth(),
		ChatID:    row.Id(),
		Title:     row.Title(),
		About:     row.About(),
		CreatorID: row.Creator(),
		Members:   members,
	})
	if err != nil {
		return peerRef{}, peerRef{}, err
	}
	_, _ = c.svcCtx.Dao.ChatClient.ChatMigratedToChannel(c.ctx, &chat.TLChatMigratedToChannel{
		Chat:       row,
		Id:         result.ChannelID,
		AccessHash: result.AccessHash,
	})
	return peerRef{Kind: "channel", ID: result.ChannelID, Hash: result.AccessHash}, ref, nil
}

func (c *FoldersCore) permAuth() int64 {
	if c == nil || c.MD == nil {
		return 0
	}
	return c.MD.GetPermAuthKeyId()
}

// applyPeerReplacements puts shared peers on the folder and swaps a migrated
// basic group for its supergroup, including a pinned copy.
func applyPeerReplacements(filter *mtproto.DialogFilter, replacements map[string]peerRef, extra []peerRef) {
	if filter == nil {
		return
	}
	swap := func(peers []*mtproto.InputPeer, addExtra bool) []*mtproto.InputPeer {
		out := make([]*mtproto.InputPeer, 0, len(peers)+len(extra))
		seen := map[string]struct{}{}
		for _, peer := range peers {
			ref, err := refFromInput(0, peer)
			if err == nil {
				if next, ok := replacements[ref.key()]; ok {
					ref = next
					peer = next.input()
				}
				if peer == nil {
					continue
				}
				if _, ok := seen[ref.key()]; ok {
					continue
				}
				seen[ref.key()] = struct{}{}
			}
			if peer != nil {
				out = append(out, peer)
			}
		}
		if addExtra {
			for _, ref := range extra {
				if _, ok := seen[ref.key()]; ok {
					continue
				}
				if peer := ref.input(); peer != nil {
					out = append(out, peer)
					seen[ref.key()] = struct{}{}
				}
			}
		}
		return out
	}
	filter.IncludePeers = swap(filter.GetIncludePeers(), true)
	filter.PinnedPeers = swap(filter.GetPinnedPeers(), false)
}

func (c *FoldersCore) saveOwnChatlist(filter *mtproto.DialogFilter) error {
	filter.FixData()
	stored := mtproto.MakeTLDialogFilterChatlist(filter).To_DialogFilter()
	if _, err := c.svcCtx.Dao.DialogClient.DialogInsertOrUpdateDialogFilter(c.ctx, &dialog.TLDialogInsertOrUpdateDialogFilter{
		UserId:       c.userID(),
		Id:           stored.GetId(),
		DialogFilter: stored,
	}); err != nil {
		c.Logger.Errorf("chatlists.save - error: %v", err)
		return err
	}
	c.pushNotMe(c.filterUpdate(stored.GetId(), stored))
	return nil
}

func (c *FoldersCore) putInvites(filter *mtproto.DialogFilter, rows []storedInvite) error {
	filter.Title_STRING = encodeInvites(rows)
	filter.HasMyInvites = len(rows) > 0
	return c.saveOwnChatlist(filter)
}

func checkInviteTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) > chatlistInviteTitleMax {
		return "", mtproto.ErrMessageTooLong
	}
	return title, nil
}

func (c *FoldersCore) exportedInvite(filter *mtproto.DialogFilter, inv storedInvite) *mtproto.ExportedChatlistInvite {
	return mtproto.MakeTLExportedChatlistInvite(&mtproto.ExportedChatlistInvite{
		Title: inv.Title,
		Url:   inviteURL(inv.Slug),
		Peers: peersOf(inv.Peers),
	}).To_ExportedChatlistInvite()
}

func (c *FoldersCore) peerSide(refs []peerRef) (users []*mtproto.User, chats []*mtproto.Chat) {
	users = []*mtproto.User{}
	chats = []*mtproto.Chat{}
	var userIDs, chatIDs, channelIDs []int64
	for _, ref := range refs {
		switch ref.Kind {
		case "user":
			userIDs = append(userIDs, ref.ID)
		case "chat":
			chatIDs = append(chatIDs, ref.ID)
		case "channel":
			channelIDs = append(channelIDs, ref.ID)
		}
	}
	if len(userIDs) > 0 && c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.UserClient != nil {
		got, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: userIDs})
		if err != nil {
			c.Logger.Errorf("chatlists.users - error: %v", err)
		} else if got != nil {
			if list := got.GetUserListByIdList(c.userID(), userIDs...); len(list) > 0 {
				users = list
			}
		}
	}
	if len(chatIDs) > 0 && c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.ChatClient != nil {
		got, err := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx, &chat.TLChatGetChatListByIdList{
			SelfId: c.userID(),
			IdList: chatIDs,
		})
		if err != nil {
			c.Logger.Errorf("chatlists.chats - error: %v", err)
		} else if got != nil {
			if list := got.GetChatListByIdList(c.userID(), chatIDs...); len(list) > 0 {
				chats = list
			}
		}
	}
	for _, id := range channelIDs {
		if card := channel.Shared().InviteCard(c.ctx, c.userID(), id); card != nil {
			chats = append(chats, card)
		}
	}
	return users, chats
}

func (c *FoldersCore) inviteCard(filter *mtproto.DialogFilter, inv storedInvite, already *mtproto.DialogFilter) *mtproto.Chatlists_ChatlistInvite {
	users, chats := c.peerSide(inv.Peers)
	title := inv.Title
	if title == "" {
		title = visibleTitle(filter)
	}
	if already != nil {
		have := folderPeerRefs(already)
		var missing, present []peerRef
		for _, ref := range inv.Peers {
			if _, ok := have[ref.key()]; ok {
				present = append(present, ref)
				continue
			}
			missing = append(missing, ref)
		}
		return mtproto.MakeTLChatlistsChatlistInviteAlready(&mtproto.Chatlists_ChatlistInvite{
			FilterId:     already.GetId(),
			MissingPeers: peersOf(missing),
			AlreadyPeers: peersOf(present),
			Chats:        chats,
			Users:        users,
		}).To_Chatlists_ChatlistInvite()
	}
	card := &mtproto.Chatlists_ChatlistInvite{
		Title_TEXTWITHENTITIES: textTitle(title),
		TitleNoanimate:         filter.GetTitleNoanimate(),
		Peers:                  peersOf(inv.Peers),
		Chats:                  chats,
		Users:                  users,
	}
	if emo := filter.GetEmoticon(); emo != nil && emo.GetValue() != "" {
		card.Emoticon = wrapperspb.String(emo.GetValue())
	}
	return mtproto.MakeTLChatlistsChatlistInvite(card).To_Chatlists_ChatlistInvite()
}

func missingRefs(inv storedInvite, joined *mtproto.DialogFilter, hidden []peerRef) []peerRef {
	have := folderPeerRefs(joined)
	gone := map[string]struct{}{}
	for _, ref := range hidden {
		gone[ref.key()] = struct{}{}
	}
	var out []peerRef
	for _, ref := range inv.Peers {
		if _, ok := have[ref.key()]; ok {
			continue
		}
		if _, ok := gone[ref.key()]; ok {
			continue
		}
		out = append(out, ref)
	}
	return out
}

// countStoredFolders is the number of real folders. dialogFilterDefault
// (id 0) is always on the client and does not consume dialog_filters_limit.
func countStoredFolders(rows []*dialog.DialogFilterExt) int {
	n := 0
	for _, row := range rows {
		if row == nil || row.GetId() == dialog.DialogFilterDefaultID {
			continue
		}
		n++
	}
	return n
}

func (c *FoldersCore) countJoined(rows []*dialog.DialogFilterExt) int {
	n := 0
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.GetJoinedBySlug() {
			n++
			continue
		}
		if row.GetDialogFilter() != nil && strings.HasPrefix(row.GetDialogFilter().GetTitle_STRING(), mtproto.ChatlistJoinBlobPrefix) {
			n++
		}
	}
	return n
}

func (c *FoldersCore) channelCap() int {
	return premium.Limit(premium.FeatureChannels, c.premiumMember())
}

// admit joins the user to every selected chat. Channels and supergroups go
// through channels.inviteToChannel (the folder owner invites) and, when the
// owner cannot invite a public channel, channels.joinChannel. Basic groups
// go through chat.addChatUser, the same write messages.addChatUser uses.
func (c *FoldersCore) admit(owner int64, refs []peerRef) ([]*mtproto.Updates, error) {
	var parts []*mtproto.Updates
	for _, ref := range refs {
		switch ref.Kind {
		case "channel":
			up, err := c.admitChannel(owner, ref)
			if err != nil {
				return nil, err
			}
			if up != nil {
				parts = append(parts, up)
			}
		case "chat":
			if err := c.admitChat(owner, ref.ID); err != nil {
				return nil, err
			}
		}
	}
	return parts, nil
}

func (c *FoldersCore) admitChannel(owner int64, ref peerRef) (*mtproto.Updates, error) {
	hash := ref.Hash
	if hash == 0 {
		issued, ok, err := channel.Shared().IssuedAccessHash(c.ctx, ref.ID)
		if err != nil || !ok {
			return nil, mtproto.ErrChannelInvalid
		}
		hash = issued
	}
	in := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  ref.ID,
		AccessHash: hash,
	}).To_InputChannel()
	invited, err := channel.Shared().Invite(c.ctx, owner, 0, in, []int64{c.userID()}, nil)
	if err != nil {
		if sameRPC(err, mtproto.ErrChatAdminRequired) || sameRPC(err, mtproto.ErrChannelPrivate) {
			return channel.Shared().Join(c.ctx, c.userID(), c.permAuth(), in)
		}
		return nil, err
	}
	if invited != nil {
		for _, miss := range invited.GetMissingInvitees() {
			if miss.GetUserId() == c.userID() {
				return nil, mtproto.ErrUserBannedInChannel
			}
		}
		return invited.GetUpdates(), nil
	}
	return nil, nil
}

func (c *FoldersCore) admitChat(owner, id int64) error {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil {
		return mtproto.ErrChatIdInvalid
	}
	_, err := c.svcCtx.Dao.ChatClient.ChatAddChatUser(c.ctx, &chat.TLChatAddChatUser{
		ChatId:    id,
		InviterId: owner,
		UserId:    c.userID(),
	})
	if err == nil || sameRPC(err, mtproto.ErrUserAlreadyParticipant) {
		return nil
	}
	return err
}

func joinUpdates(filter *mtproto.Update, parts []*mtproto.Updates) *mtproto.Updates {
	out := updatesOf(filter)
	for _, part := range parts {
		if part == nil {
			continue
		}
		out.Updates = append(out.Updates, part.GetUpdates()...)
		out.Chats = append(out.Chats, part.GetChats()...)
		out.Users = append(out.Users, part.GetUsers()...)
	}
	if out.Updates == nil {
		out.Updates = []*mtproto.Update{}
	}
	if out.Chats == nil {
		out.Chats = []*mtproto.Chat{}
	}
	if out.Users == nil {
		out.Users = []*mtproto.User{}
	}
	return out
}
