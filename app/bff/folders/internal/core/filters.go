package core

import (
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/premium"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// dialogFilterTitleMaxRunes is the folder title length official clients enforce.
// Longer titles are MESSAGE_TOO_LONG on messages.updateDialogFilter.
const dialogFilterTitleMaxRunes = 12

// dialogFilterPeerMax matches dialog_filters_chats_limit_* in help.getAppConfig.
const dialogFilterPeerMax = premium.DialogFilterChats

func (c *FoldersCore) userID() int64 {
	if c.MD == nil {
		return 0
	}
	return c.MD.UserId
}

func (c *FoldersCore) loadFilters() ([]*mtproto.DialogFilter, bool, error) {
	res, err := c.svcCtx.Dao.DialogClient.DialogGetDialogFilters(c.ctx, &dialog.TLDialogGetDialogFilters{
		UserId: c.userID(),
	})
	if err != nil {
		return nil, false, err
	}
	tags, err := c.svcCtx.Dao.DialogClient.DialogGetDialogFilterTags(c.ctx, &dialog.TLDialogGetDialogFilterTags{
		UserId: c.userID(),
	})
	if err != nil {
		return nil, false, err
	}
	return filtersFromExt(res.GetDatas()), mtproto.FromBool(tags), nil
}

// allChatsFilter is dialogFilterDefault, the "All chats" tab.
// Official clients do not insert it themselves. A reply that omits it makes
// stock Telegram-iOS treat the current tab as deleted and call scrollToTop
// on every folder refresh.
func allChatsFilter() *mtproto.DialogFilter {
	return mtproto.MakeTLDialogFilterDefault(nil).To_DialogFilter()
}

// isAllChatsRow reports a stored dialogFilterDefault. Id 0 is that folder
// even when the JSON is missing. messages.updateDialogFilter rejects id 0;
// messages.updateDialogFiltersOrder is what writes the row.
func isAllChatsRow(row *dialog.DialogFilterExt) bool {
	if row == nil {
		return false
	}
	if row.GetId() == dialog.DialogFilterDefaultID {
		return true
	}
	filter := row.GetDialogFilter()
	return filter != nil && filter.GetPredicateName() == mtproto.Predicate_dialogFilterDefault
}

// filtersFromExt returns the folder vector official clients compare.
// dialogFilterDefault is included once: at its stored order_value, or first
// when the user has never moved All chats. Other folders keep order_value
// order (higher first). An empty table is [dialogFilterDefault], not [].
func filtersFromExt(rows []*dialog.DialogFilterExt) []*mtproto.DialogFilter {
	sorted := append([]*dialog.DialogFilterExt(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i] == nil || sorted[j] == nil {
			return sorted[i] != nil
		}
		return sorted[i].GetOrder() > sorted[j].GetOrder()
	})
	out := make([]*mtproto.DialogFilter, 0, len(sorted)+1)
	seenDefault := false
	for _, row := range sorted {
		if row == nil {
			continue
		}
		if isAllChatsRow(row) {
			if seenDefault {
				continue
			}
			seenDefault = true
			out = append(out, allChatsFilter())
			continue
		}
		filter := prepareStoredFilter(row.GetDialogFilter())
		if filter == nil || filter.GetPredicateName() == mtproto.Predicate_dialogFilterDefault {
			continue
		}
		out = append(out, filter)
	}
	if !seenDefault {
		out = append([]*mtproto.DialogFilter{allChatsFilter()}, out...)
	}
	return out
}

func messagesDialogFilters(filters []*mtproto.DialogFilter, tagsEnabled bool) *mtproto.Messages_DialogFilters {
	if filters == nil {
		filters = []*mtproto.DialogFilter{}
	}
	return mtproto.MakeTLMessagesDialogFilters(&mtproto.Messages_DialogFilters{
		TagsEnabled: tagsEnabled,
		Filters:     filters,
	}).To_Messages_DialogFilters()
}

func prepareStoredFilter(filter *mtproto.DialogFilter) *mtproto.DialogFilter {
	if filter == nil {
		return nil
	}
	filter.FixData()
	// Peer vectors are part of the folder hash clients compare between
	// getDialogFilters calls. A fixed order stops the chat list jumping
	// when the same peers come back shuffled.
	filter.PinnedPeers = orderedInputPeers(filter.GetPinnedPeers())
	filter.IncludePeers = orderedInputPeers(filter.GetIncludePeers())
	filter.ExcludePeers = orderedInputPeers(filter.GetExcludePeers())
	if filter.GetTitle_TEXTWITHENTITIES() == nil {
		filter.Title_TEXTWITHENTITIES = mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
			Text:     filter.GetTitle(),
			Entities: []*mtproto.MessageEntity{},
		}).To_TextWithEntities()
	}
	// Invite JSON stays in the database. Clients receive the visible title.
	mtproto.StripChatlistBlob(filter)
	switch filter.GetPredicateName() {
	case mtproto.Predicate_dialogFilterChatlist:
		return mtproto.MakeTLDialogFilterChatlist(filter).To_DialogFilter()
	case mtproto.Predicate_dialogFilterDefault:
		return mtproto.MakeTLDialogFilterDefault(filter).To_DialogFilter()
	default:
		return mtproto.MakeTLDialogFilter(filter).To_DialogFilter()
	}
}

// orderedInputPeers sorts peers by kind, id, message id, access hash, then
// username. The same folder always returns the same vectors.
func orderedInputPeers(peers []*mtproto.InputPeer) []*mtproto.InputPeer {
	if len(peers) < 2 {
		return peers
	}
	out := append([]*mtproto.InputPeer(nil), peers...)
	sort.SliceStable(out, func(i, j int) bool {
		return inputPeerLess(out[i], out[j])
	})
	return out
}

func inputPeerLess(a, b *mtproto.InputPeer) bool {
	if a == nil || b == nil {
		return a != nil && b == nil
	}
	ak, aid, amsg, aname := inputPeerKey(a)
	bk, bid, bmsg, bname := inputPeerKey(b)
	if ak != bk {
		return ak < bk
	}
	if aid != bid {
		return aid < bid
	}
	if amsg != bmsg {
		return amsg < bmsg
	}
	if a.GetAccessHash() != b.GetAccessHash() {
		return a.GetAccessHash() < b.GetAccessHash()
	}
	return aname < bname
}

func inputPeerKey(p *mtproto.InputPeer) (kind int, id int64, msg int32, name string) {
	switch p.GetPredicateName() {
	case mtproto.Predicate_inputPeerEmpty:
		return 0, 0, 0, ""
	case mtproto.Predicate_inputPeerSelf:
		return 1, 0, 0, ""
	case mtproto.Predicate_inputPeerUser:
		return 2, p.GetUserId(), 0, ""
	case mtproto.Predicate_inputPeerChat:
		return 3, p.GetChatId(), 0, ""
	case mtproto.Predicate_inputPeerChannel:
		return 4, p.GetChannelId(), 0, ""
	case mtproto.Predicate_inputPeerUserFromMessage:
		return 5, p.GetUserId(), p.GetMsgId(), ""
	case mtproto.Predicate_inputPeerChannelFromMessage:
		return 6, p.GetChannelId(), p.GetMsgId(), ""
	case mtproto.Predicate_inputPeerUsername:
		return 7, 0, 0, p.GetUsername()
	default:
		return 8, p.GetUserId(), p.GetMsgId(), p.GetPredicateName()
	}
}

// validateDialogFilter applies the errors documented for messages.updateDialogFilter.
// https://core.telegram.org/method/messages.updateDialogFilter
func validateDialogFilter(id int32, filter *mtproto.DialogFilter) error {
	if id <= 0 {
		return mtproto.ErrFilterIdInvalid
	}
	if filter == nil {
		return nil
	}
	switch filter.GetPredicateName() {
	case "", mtproto.Predicate_dialogFilter, mtproto.Predicate_dialogFilterChatlist:
	default:
		return mtproto.ErrFilterIdInvalid
	}
	if filter.GetId() != 0 && filter.GetId() != id {
		return mtproto.ErrFilterIdInvalid
	}
	filter.Id = id

	filter.FixData()
	title := ""
	if filter.GetTitle_TEXTWITHENTITIES() != nil {
		title = filter.GetTitle_TEXTWITHENTITIES().GetText()
	}
	if strings.TrimSpace(title) == "" {
		title = strings.TrimSpace(filter.GetTitle())
	}
	if title == "" {
		return mtproto.ErrFilterTitleEmpty
	}
	if utf8.RuneCountInString(title) > dialogFilterTitleMaxRunes {
		return mtproto.ErrMessageTooLong
	}
	if filter.GetEmoticon() != nil && utf8.RuneCountInString(filter.GetEmoticon().GetValue()) > 32 {
		return mtproto.ErrMessageTooLong
	}

	chatlist := filter.GetPredicateName() == mtproto.Predicate_dialogFilterChatlist
	if chatlist && len(filter.GetExcludePeers()) > 0 {
		return mtproto.ErrChatlistExcludeInvalid
	}
	if err := checkPeers(filter.GetPinnedPeers()); err != nil {
		return err
	}
	if err := checkPeers(filter.GetIncludePeers()); err != nil {
		return err
	}
	if err := checkPeers(filter.GetExcludePeers()); err != nil {
		return err
	}
	if overlaps(filter.GetExcludePeers(), filter.GetIncludePeers()) || overlaps(filter.GetExcludePeers(), filter.GetPinnedPeers()) {
		return mtproto.ErrPeerIdInvalid
	}
	peers := len(filter.GetPinnedPeers()) + len(filter.GetIncludePeers()) + len(filter.GetExcludePeers())
	if peers > dialogFilterPeerMax || len(filter.GetPinnedPeers()) > premium.FolderPinnedDialogs {
		return mtproto.ErrFilterIdInvalid
	}
	if !filterHasInclude(filter, chatlist) {
		return mtproto.ErrFilterIncludeEmpty
	}
	return nil
}

func filterHasInclude(filter *mtproto.DialogFilter, chatlist bool) bool {
	if len(filter.GetIncludePeers()) > 0 || len(filter.GetPinnedPeers()) > 0 {
		return true
	}
	if chatlist {
		return false
	}
	return filter.GetContacts() || filter.GetNonContacts() || filter.GetGroups() || filter.GetBroadcasts() || filter.GetBots()
}

func checkPeers(peers []*mtproto.InputPeer) error {
	for _, peer := range peers {
		if err := checkPeer(peer); err != nil {
			return err
		}
	}
	return nil
}

func checkPeer(peer *mtproto.InputPeer) error {
	if peer == nil || peer.GetPredicateName() == "" || peer.GetPredicateName() == mtproto.Predicate_inputPeerEmpty {
		return mtproto.ErrPeerIdInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		return nil
	case mtproto.Predicate_inputPeerUser:
		if peer.GetUserId() <= 0 {
			return mtproto.ErrPeerIdInvalid
		}
	case mtproto.Predicate_inputPeerChat:
		if peer.GetChatId() <= 0 {
			return mtproto.ErrChatIdInvalid
		}
	case mtproto.Predicate_inputPeerChannel:
		if peer.GetChannelId() <= 0 {
			return mtproto.ErrChatIdInvalid
		}
	case mtproto.Predicate_inputPeerUserFromMessage, mtproto.Predicate_inputPeerChannelFromMessage:
		if peer.GetMsgId() <= 0 {
			return mtproto.ErrMsgIdInvalid
		}
		return checkPeer(peer.GetPeer())
	case mtproto.Predicate_inputPeerUsername:
		if strings.TrimSpace(peer.GetUsername()) == "" {
			return mtproto.ErrPeerIdInvalid
		}
	default:
		return mtproto.ErrPeerIdInvalid
	}
	return nil
}

func overlaps(a, b []*mtproto.InputPeer) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(a))
	for _, peer := range a {
		seen[peerKey(peer)] = struct{}{}
	}
	for _, peer := range b {
		if _, ok := seen[peerKey(peer)]; ok {
			return true
		}
	}
	return false
}

func peerKey(peer *mtproto.InputPeer) string {
	if peer == nil {
		return ""
	}
	return peer.GetPredicateName() + ":" +
		strconv.FormatInt(peer.GetUserId(), 10) + ":" +
		strconv.FormatInt(peer.GetChatId(), 10) + ":" +
		strconv.FormatInt(peer.GetChannelId(), 10) + ":" +
		peer.GetUsername()
}

func applyColorPolicy(filter, previous *mtproto.DialogFilter, allow bool) {
	if filter.GetColor() != nil {
		v := filter.GetColor().GetValue()
		if v < -1 || v > 6 {
			filter.Color = nil
		}
	}
	if allow {
		return
	}
	if previous != nil && previous.GetColor() != nil {
		filter.Color = wrapperspb.Int32(previous.GetColor().GetValue())
		return
	}
	filter.Color = nil
}

func (c *FoldersCore) allowFolderColor() bool {
	if !c.premiumMember() {
		return false
	}
	tags, err := c.svcCtx.Dao.DialogClient.DialogGetDialogFilterTags(c.ctx, &dialog.TLDialogGetDialogFilterTags{
		UserId: c.userID(),
	})
	if err != nil {
		return false
	}
	return mtproto.FromBool(tags)
}

func (c *FoldersCore) premiumMember() bool {
	now := time.Now().Unix()
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || c.MD == nil {
		return premium.IsMember(false, 0, now)
	}
	me, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: c.MD.UserId})
	if err != nil || me == nil {
		return premium.IsMember(false, 0, now)
	}
	return me.Premium()
}

func (c *FoldersCore) findFilter(id int32) *mtproto.DialogFilter {
	filters, _, err := c.loadFilters()
	if err != nil {
		return nil
	}
	for _, filter := range filters {
		if filter.GetId() == id {
			return filter
		}
	}
	return nil
}

func (c *FoldersCore) pushNotMe(update *mtproto.Update) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.SyncClient == nil || c.MD == nil {
		return
	}
	_, _ = c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       mtproto.MakeUpdatesByUpdates(update),
	})
}
