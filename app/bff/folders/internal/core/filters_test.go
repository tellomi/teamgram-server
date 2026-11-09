package core

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/svc"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncrpc "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestEmptyDialogFiltersEncode(t *testing.T) {
	filters := filtersFromExt(nil)
	if len(filters) != 1 || filters[0].GetPredicateName() != mtproto.Predicate_dialogFilterDefault {
		t.Fatalf("filters %v", filterNames(filters))
	}
	raw := encodeDialogFilters(t, messagesDialogFilters(filters, false))
	// messages.dialogFilters#2ad93719 flags:# tags_enabled:flags.0?true filters:Vector<DialogFilter>
	if len(raw) != 20 {
		t.Fatalf("encoding length %d", len(raw))
	}
	if le32(raw, 0) != 0x2ad93719 {
		t.Fatalf("constructor %x", le32(raw, 0))
	}
	if le32(raw, 4) != 0 {
		t.Fatalf("flags %x", le32(raw, 4))
	}
	if le32(raw, 8) != 0x1cb5c415 {
		t.Fatalf("vector %x", le32(raw, 8))
	}
	if le32(raw, 12) != 1 {
		t.Fatalf("count %d", le32(raw, 12))
	}
	if le32(raw, 16) != 0x363293ae {
		t.Fatalf("element %x", le32(raw, 16))
	}

	tagged := encodeDialogFilters(t, messagesDialogFilters(filters, true))
	if le32(tagged, 0) != 0x2ad93719 || le32(tagged, 4) != 1 || le32(tagged, 16) != 0x363293ae {
		t.Fatalf("tags constructor %x flags %x element %x", le32(tagged, 0), le32(tagged, 4), le32(tagged, 16))
	}
}

func TestDialogFiltersDefaultPosition(t *testing.T) {
	noDefault := filtersFromExt([]*dialog.DialogFilterExt{
		folderRow(2, 20),
		folderRow(3, 10),
	})
	if got := filterNames(noDefault); !sameStrings(got, []string{"dialogFilterDefault", "dialogFilter#2", "dialogFilter#3"}) {
		t.Fatalf("prepend %v", got)
	}

	placed := filtersFromExt([]*dialog.DialogFilterExt{
		folderRow(2, 30),
		{Id: dialog.DialogFilterDefaultID, Order: 20, DialogFilter: mtproto.MakeTLDialogFilterDefault(nil).To_DialogFilter()},
		folderRow(3, 10),
	})
	if got := filterNames(placed); !sameStrings(got, []string{"dialogFilter#2", "dialogFilterDefault", "dialogFilter#3"}) {
		t.Fatalf("stored position %v", got)
	}

	duplicated := filtersFromExt([]*dialog.DialogFilterExt{
		folderRow(2, 40),
		{Id: dialog.DialogFilterDefaultID, Order: 30, DialogFilter: mtproto.MakeTLDialogFilterDefault(nil).To_DialogFilter()},
		{Id: dialog.DialogFilterDefaultID, Order: 20, DialogFilter: mtproto.MakeTLDialogFilterDefault(nil).To_DialogFilter()},
		folderRow(3, 10),
	})
	if got := filterNames(duplicated); !sameStrings(got, []string{"dialogFilter#2", "dialogFilterDefault", "dialogFilter#3"}) {
		t.Fatalf("duplicated default %v", got)
	}
	if n := countDefaults(duplicated); n != 1 {
		t.Fatalf("default count %d", n)
	}

	// A stored id 0 with no JSON still occupies its order_value slot.
	bare := filtersFromExt([]*dialog.DialogFilterExt{
		folderRow(2, 30),
		{Id: 0, Order: 20},
		folderRow(3, 10),
	})
	if got := filterNames(bare); !sameStrings(got, []string{"dialogFilter#2", "dialogFilterDefault", "dialogFilter#3"}) {
		t.Fatalf("bare id 0 %v", got)
	}
}

func TestOldLayerDialogFiltersIncludeDefault(t *testing.T) {
	store := &memDialog{rows: []*dialog.DialogFilterExt{
		folderRow(2, 20),
		folderRow(3, 10),
	}}
	c := newFoldersForTest(store, nil)
	old, err := c.MessagesGetDialogFiltersF19ED96D(&mtproto.TLMessagesGetDialogFiltersF19ED96D{})
	if err != nil {
		t.Fatal(err)
	}
	cur, err := c.MessagesGetDialogFiltersEFD48C89(&mtproto.TLMessagesGetDialogFiltersEFD48C89{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filterNames(old.GetDatas()), filterNames(cur.GetFilters()); !sameStrings(got, want) {
		t.Fatalf("old %v current %v", got, want)
	}
	if got := filterNames(old.GetDatas()); !sameStrings(got, []string{"dialogFilterDefault", "dialogFilter#2", "dialogFilter#3"}) {
		t.Fatalf("old layer %v", got)
	}
	buf := mtproto.NewEncodeBuf(64)
	if err := old.Encode(buf, 158); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if len(raw) < 12 || le32(raw, 0) != 0x1cb5c415 || le32(raw, 4) != 3 || le32(raw, 8) != 0x363293ae {
		t.Fatalf("old encoding %x", raw)
	}
	if cur.GetTagsEnabled() {
		t.Fatal("tags flag changed")
	}
}

func TestUpdateDialogFiltersOrderThenGet(t *testing.T) {
	store := &memDialog{rows: []*dialog.DialogFilterExt{
		folderRow(2, 1),
		folderRow(3, 1),
	}}
	push := &memSync{}
	c := newFoldersForTest(store, push)
	order := []int32{2, 0, 3}
	got, err := c.MessagesUpdateDialogFiltersOrder(&mtproto.TLMessagesUpdateDialogFiltersOrder{Order: order})
	if err != nil || !mtproto.FromBool(got) {
		t.Fatalf("order: %v %v", got, err)
	}
	if !sameInts(store.lastOrder, order) {
		t.Fatalf("forwarded %v", store.lastOrder)
	}
	if len(push.orders) != 1 || !sameInts(push.orders[0], order) {
		t.Fatalf("echo %v", push.orders)
	}

	cur, err := c.MessagesGetDialogFiltersEFD48C89(&mtproto.TLMessagesGetDialogFiltersEFD48C89{})
	if err != nil {
		t.Fatal(err)
	}
	if names := filterNames(cur.GetFilters()); !sameStrings(names, []string{"dialogFilter#2", "dialogFilterDefault", "dialogFilter#3"}) {
		t.Fatalf("after order %v", names)
	}
	if cur.GetTagsEnabled() {
		t.Fatal("tags flag changed")
	}

	again, err := c.MessagesUpdateDialogFiltersOrder(&mtproto.TLMessagesUpdateDialogFiltersOrder{Order: order})
	if err != nil || !mtproto.FromBool(again) {
		t.Fatalf("second order: %v %v", again, err)
	}
	cur, err = c.MessagesGetDialogFiltersEFD48C89(&mtproto.TLMessagesGetDialogFiltersEFD48C89{})
	if err != nil {
		t.Fatal(err)
	}
	if n := countDefaults(cur.GetFilters()); n != 1 {
		t.Fatalf("second get defaults %d %v", n, filterNames(cur.GetFilters()))
	}
	old, err := c.MessagesGetDialogFiltersF19ED96D(&mtproto.TLMessagesGetDialogFiltersF19ED96D{})
	if err != nil {
		t.Fatal(err)
	}
	if names := filterNames(old.GetDatas()); !sameStrings(names, []string{"dialogFilter#2", "dialogFilterDefault", "dialogFilter#3"}) {
		t.Fatalf("old layer after order %v", names)
	}
}

func TestSuggestedDialogFiltersEncode(t *testing.T) {
	got := suggestedDialogFilters()
	if len(got.GetDatas()) != 5 {
		t.Fatalf("suggestions %d", len(got.GetDatas()))
	}
	buf := mtproto.NewEncodeBuf(1024)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	if len(buf.GetBuf()) < 8 {
		t.Fatal("empty encoding")
	}
}

func TestUpdateDialogFilterValidation(t *testing.T) {
	if err := validateDialogFilter(0, nil); err != mtproto.ErrFilterIdInvalid {
		t.Fatalf("id 0: %v", err)
	}
	if err := validateDialogFilter(2, nil); err != nil {
		t.Fatalf("delete: %v", err)
	}
	empty := &mtproto.DialogFilter{}
	if err := validateDialogFilter(2, empty); err != mtproto.ErrFilterTitleEmpty {
		t.Fatalf("empty title: %v", err)
	}
	long := titled("this title is far too long")
	if err := validateDialogFilter(2, long); err != mtproto.ErrMessageTooLong {
		t.Fatalf("long title: %v", err)
	}
	noInclude := titled("Unread")
	if err := validateDialogFilter(2, noInclude); err != mtproto.ErrFilterIncludeEmpty {
		t.Fatalf("include: %v", err)
	}
	ok := titled("Unread")
	ok.Contacts = true
	ok.ExcludeRead = true
	if err := validateDialogFilter(2, ok); err != nil {
		t.Fatalf("flags: %v", err)
	}
	if ok.GetId() != 2 {
		t.Fatalf("id %d", ok.GetId())
	}
	mismatch := titled("Unread")
	mismatch.Id = 3
	mismatch.ExcludeRead = true
	if err := validateDialogFilter(2, mismatch); err != mtproto.ErrFilterIdInvalid {
		t.Fatalf("mismatch: %v", err)
	}
	chatlist := titled("Shared")
	chatlist.PredicateName = mtproto.Predicate_dialogFilterChatlist
	chatlist.ExcludePeers = []*mtproto.InputPeer{mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 5}).To_InputPeer()}
	chatlist.IncludePeers = []*mtproto.InputPeer{mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 6}).To_InputPeer()}
	if err := validateDialogFilter(4, chatlist); err != mtproto.ErrChatlistExcludeInvalid {
		t.Fatalf("chatlist exclude: %v", err)
	}
	badPeer := titled("People")
	badPeer.IncludePeers = []*mtproto.InputPeer{mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{}).To_InputPeer()}
	if err := validateDialogFilter(4, badPeer); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("peer: %v", err)
	}
	badChat := titled("People")
	badChat.IncludePeers = []*mtproto.InputPeer{mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{}).To_InputPeer()}
	if err := validateDialogFilter(4, badChat); err != mtproto.ErrChatIdInvalid {
		t.Fatalf("chat: %v", err)
	}
	badMsg := titled("People")
	badMsg.IncludePeers = []*mtproto.InputPeer{mtproto.MakeTLInputPeerUserFromMessage(&mtproto.InputPeer{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer(),
	}).To_InputPeer()}
	if err := validateDialogFilter(4, badMsg); err != mtproto.ErrMsgIdInvalid {
		t.Fatalf("msg: %v", err)
	}
}

func TestEmptyOrderSucceedsWithoutStorage(t *testing.T) {
	c := &FoldersCore{}
	got, err := c.MessagesUpdateDialogFiltersOrder(&mtproto.TLMessagesUpdateDialogFiltersOrder{})
	if err != nil || !mtproto.FromBool(got) {
		t.Fatalf("empty order: %v %v", got, err)
	}
	_, err = c.MessagesUpdateDialogFiltersOrder(&mtproto.TLMessagesUpdateDialogFiltersOrder{Order: []int32{0, -1}})
	if err != mtproto.ErrFilterIdInvalid {
		t.Fatalf("negative id: %v", err)
	}
}

func TestApplyColorPolicy(t *testing.T) {
	next := &mtproto.DialogFilter{Color: wrapperspb.Int32(2)}
	prev := &mtproto.DialogFilter{Color: wrapperspb.Int32(1)}
	applyColorPolicy(next, prev, false)
	if next.GetColor().GetValue() != 1 {
		t.Fatalf("kept %d", next.GetColor().GetValue())
	}
	next.Color = wrapperspb.Int32(9)
	applyColorPolicy(next, nil, true)
	if next.GetColor() != nil {
		t.Fatalf("out of range color kept")
	}
}

func TestDialogFilterPeersStableOrder(t *testing.T) {
	filter := titled("People")
	filter.PinnedPeers = []*mtproto.InputPeer{
		inUser(9), inChat(2), inUser(3), inChannel(1),
	}
	filter.IncludePeers = []*mtproto.InputPeer{
		inChannel(4), inUser(1), inChat(2),
	}
	filter.ExcludePeers = []*mtproto.InputPeer{
		inUser(8), inUser(2), inChat(1),
	}
	shuffled := titled("People")
	shuffled.PinnedPeers = []*mtproto.InputPeer{
		inChannel(1), inUser(3), inChat(2), inUser(9),
	}
	shuffled.IncludePeers = []*mtproto.InputPeer{
		inChat(2), inChannel(4), inUser(1),
	}
	shuffled.ExcludePeers = []*mtproto.InputPeer{
		inChat(1), inUser(8), inUser(2),
	}

	got := prepareStoredFilter(filter)
	again := prepareStoredFilter(got)
	other := prepareStoredFilter(shuffled)
	wantPinned := []string{"user:3", "user:9", "chat:2", "channel:1"}
	wantInclude := []string{"user:1", "chat:2", "channel:4"}
	wantExclude := []string{"user:2", "user:8", "chat:1"}
	for _, f := range []*mtproto.DialogFilter{got, again, other} {
		if keys := peerKeys(f.GetPinnedPeers()); !sameStrings(keys, wantPinned) {
			t.Fatalf("pinned %v", keys)
		}
		if keys := peerKeys(f.GetIncludePeers()); !sameStrings(keys, wantInclude) {
			t.Fatalf("include %v", keys)
		}
		if keys := peerKeys(f.GetExcludePeers()); !sameStrings(keys, wantExclude) {
			t.Fatalf("exclude %v", keys)
		}
	}
}

func inUser(id int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: id}).To_InputPeer()
}

func inChat(id int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: id}).To_InputPeer()
}

func inChannel(id int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: id}).To_InputPeer()
}

func peerKeys(peers []*mtproto.InputPeer) []string {
	out := make([]string, 0, len(peers))
	for _, peer := range peers {
		switch peer.GetPredicateName() {
		case mtproto.Predicate_inputPeerUser:
			out = append(out, "user:"+strconv.FormatInt(peer.GetUserId(), 10))
		case mtproto.Predicate_inputPeerChat:
			out = append(out, "chat:"+strconv.FormatInt(peer.GetChatId(), 10))
		case mtproto.Predicate_inputPeerChannel:
			out = append(out, "channel:"+strconv.FormatInt(peer.GetChannelId(), 10))
		default:
			out = append(out, peer.GetPredicateName())
		}
	}
	return out
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func sameInts(got, want []int32) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func encodeDialogFilters(t *testing.T, got *mtproto.Messages_DialogFilters) []byte {
	t.Helper()
	if got.GetPredicateName() != mtproto.Predicate_messages_dialogFilters {
		t.Fatalf("predicate %s", got.GetPredicateName())
	}
	buf := mtproto.NewEncodeBuf(128)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	return buf.GetBuf()
}

func le32(raw []byte, off int) uint32 {
	return uint32(raw[off]) | uint32(raw[off+1])<<8 | uint32(raw[off+2])<<16 | uint32(raw[off+3])<<24
}

func filterNames(filters []*mtproto.DialogFilter) []string {
	out := make([]string, 0, len(filters))
	for _, filter := range filters {
		name := filter.GetPredicateName()
		if name == mtproto.Predicate_dialogFilter || name == mtproto.Predicate_dialogFilterChatlist {
			name += "#" + strconv.Itoa(int(filter.GetId()))
		}
		out = append(out, name)
	}
	return out
}

func countDefaults(filters []*mtproto.DialogFilter) int {
	n := 0
	for _, filter := range filters {
		if filter.GetPredicateName() == mtproto.Predicate_dialogFilterDefault {
			n++
		}
	}
	return n
}

func folderRow(id int32, order int64) *dialog.DialogFilterExt {
	filter := titled("F" + strconv.Itoa(int(id)))
	filter.Id = id
	filter.Contacts = true
	return &dialog.DialogFilterExt{
		Id:           id,
		Order:        order,
		DialogFilter: filter,
	}
}

func newFoldersForTest(d dialogclient.DialogClient, s sync_client.SyncClient) *FoldersCore {
	return &FoldersCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{
			Dao: &dao.Dao{
				DialogClient: d,
				SyncClient:   s,
			},
		},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 9},
	}
}

// memDialog applies the same order steps the dialog service persists.
// Id 0 is upserted; unknown positive ids are left untouched.
type memDialog struct {
	dialogclient.DialogClient
	rows      []*dialog.DialogFilterExt
	lastOrder []int32
	tags      bool
}

func (m *memDialog) DialogGetDialogFilters(ctx context.Context, in *dialog.TLDialogGetDialogFilters) (*dialog.Vector_DialogFilterExt, error) {
	return &dialog.Vector_DialogFilterExt{Datas: append([]*dialog.DialogFilterExt(nil), m.rows...)}, nil
}

func (m *memDialog) DialogGetDialogFilterTags(ctx context.Context, in *dialog.TLDialogGetDialogFilterTags) (*mtproto.Bool, error) {
	return mtproto.ToBool(m.tags), nil
}

func (m *memDialog) DialogUpdateDialogFiltersOrder(ctx context.Context, in *dialog.TLDialogUpdateDialogFiltersOrder) (*mtproto.Bool, error) {
	m.lastOrder = append([]int32(nil), in.GetOrder()...)
	m.rows = applyDialogFilterOrder(m.rows, in.GetOrder(), time.Now().Unix())
	return mtproto.BoolTrue, nil
}

func applyDialogFilterOrder(rows []*dialog.DialogFilterExt, order []int32, now int64) []*dialog.DialogFilterExt {
	steps := dialog.FilterOrderSteps(order, now)
	byID := map[int32]*dialog.DialogFilterExt{}
	for _, row := range rows {
		if row != nil {
			byID[row.GetId()] = row
		}
	}
	seen := map[int32]struct{}{}
	next := make([]*dialog.DialogFilterExt, 0, len(rows)+1)
	for _, step := range steps {
		if _, ok := seen[step.ID]; ok {
			continue
		}
		seen[step.ID] = struct{}{}
		if step.UpsertDefault {
			next = append(next, &dialog.DialogFilterExt{
				Id:           dialog.DialogFilterDefaultID,
				Order:        step.OrderValue,
				DialogFilter: mtproto.MakeTLDialogFilterDefault(nil).To_DialogFilter(),
			})
			continue
		}
		row, ok := byID[step.ID]
		if !ok {
			continue
		}
		cloned := *row
		cloned.Order = step.OrderValue
		next = append(next, &cloned)
	}
	for id, row := range byID {
		if _, ok := seen[id]; ok {
			continue
		}
		next = append(next, row)
	}
	return next
}

type memSync struct {
	sync_client.SyncClient
	orders [][]int32
}

func (m *memSync) SyncUpdatesNotMe(ctx context.Context, in *syncrpc.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	if in != nil && in.GetUpdates() != nil {
		for _, update := range in.GetUpdates().GetUpdates() {
			if update.GetPredicateName() == mtproto.Predicate_updateDialogFilterOrder {
				m.orders = append(m.orders, append([]int32(nil), update.GetOrder_VECTORINT32()...))
			}
		}
	}
	return mtproto.EmptyVoid, nil
}

func titled(text string) *mtproto.DialogFilter {
	return mtproto.MakeTLDialogFilter(&mtproto.DialogFilter{
		Title_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
			Text:     text,
			Entities: []*mtproto.MessageEntity{},
		}).To_TextWithEntities(),
	}).To_DialogFilter()
}
