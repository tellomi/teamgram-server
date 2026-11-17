package service

import (
	"context"
	"sort"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/statusbook"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Inbox rows are keyed the way message.getUserMessage reads them:
// (user_id, user_message_box_id). Channel and discussion ids are a
// different store and are absent here unless a test inserts a row.

const (
	callerID   int64 = 3
	callerKey  int64 = 9
	otherID    int64 = 4
	otherKey   int64 = 8
	authorID   int64 = 15
	otherReply int64 = 99
	savedID    int64 = 16
	foreignID  int64 = 77
	groupChan  int64 = 500
)

func TestBlockFromRepliesInboxScope(t *testing.T) {
	h := newRepliesHarness(t)
	ctx := userCtx(t, callerID, callerKey)
	otherCtx := userCtx(t, otherID, otherKey)

	// Discussion-shaped row: same numeric id the group would use, but the
	// peer is the group. That is not the @replies dialog.
	h.inbox.put(channelBox(callerID, 3, authorID, groupChan))
	// Private chat with the commenter, same numeric id.
	h.inbox.put(privateBox(callerID, 7, authorID, authorID))
	// Real @replies notifications. Ids 101..200 force a second history page.
	for id := int32(101); id <= 200; id++ {
		h.inbox.put(repliesBox(callerID, id, authorID, 0))
	}
	h.inbox.put(repliesBox(callerID, 80, authorID, 0))
	h.inbox.put(repliesBox(callerID, 42, authorID, savedID))
	h.inbox.put(repliesBox(callerID, 50, otherReply, 0))
	h.inbox.put(repliesBox(callerID, 40, savedID, 0))
	h.inbox.put(repliesBox(otherID, 60, foreignID, 0))
	h.inbox.put(repliesBox(otherID, 200, foreignID, 0))

	reject := []struct {
		name string
		ctx  context.Context
		id   int32
	}{
		{"channel peer", ctx, 3},
		{"private peer", ctx, 7},
		{"missing", ctx, 8},
		{"zero", ctx, 0},
		{"negative", ctx, -1},
		{"no session", context.Background(), 200},
		{"other user's id", ctx, 60},
	}
	for _, tc := range reject {
		t.Run(tc.name, func(t *testing.T) {
			beforeB, beforeD := len(h.blocks), len(h.deletes)
			_, err := h.svc().ContactsBlockFromReplies(tc.ctx, blockReq(true, true, true, tc.id))
			assertMsgIDInvalid(t, err)
			if len(h.blocks) != beforeB || len(h.deletes) != beforeD {
				t.Fatalf("invalid id blocked %d deleted %d", len(h.blocks)-beforeB, len(h.deletes)-beforeD)
			}
		})
	}

	t.Run("self and service sender", func(t *testing.T) {
		h.inbox.put(repliesBox(callerID, 31, callerID, 0))
		h.inbox.put(&mtproto.MessageBox{
			UserId: callerID, MessageId: 32, SenderUserId: repliesUserID,
			PeerType: mtproto.PEER_USER, PeerId: repliesUserID,
			Message: &mtproto.Message{},
		})
		h.inbox.put(repliesBox(callerID, 33, 0, 0))
		h.inbox.rows[callerID][33].SenderUserId = repliesUserID
		h.inbox.rows[callerID][33].Message.FwdFrom.FromId = mtproto.MakePeerChannel(groupChan)
		for _, id := range []int32{31, 32, 33} {
			_, err := h.svc().ContactsBlockFromReplies(ctx, blockReq(true, true, true, id))
			assertMsgIDInvalid(t, err)
		}
		if len(h.blocks) != 0 || len(h.deletes) != 0 {
			t.Fatalf("rejected sender still blocked %#v deleted %#v", h.blocks, h.deletes)
		}
	})

	t.Run("no flags blocks only the forwarded author", func(t *testing.T) {
		if err := h.book.Save(ctx, callerID, statusbook.Record{Pushed: 10, HaveList: true, SetTTL: true, Viewers: []statusbook.Viewer{{ID: authorID, Exact: true}}}); err != nil {
			t.Fatal(err)
		}
		if err := h.book.Save(ctx, authorID, statusbook.Record{Pushed: 10, HaveList: true, SetTTL: true, Viewers: []statusbook.Viewer{{ID: callerID, Exact: true}}}); err != nil {
			t.Fatal(err)
		}
		if err := h.book.Save(ctx, otherReply, statusbook.Record{Pushed: 10, HaveList: true, SetTTL: true, Viewers: []statusbook.Viewer{{ID: 1, Exact: true}}}); err != nil {
			t.Fatal(err)
		}
		up, err := h.svc().ContactsBlockFromReplies(ctx, blockReq(false, false, false, 200))
		if err != nil {
			t.Fatal(err)
		}
		if len(h.blocks) != 1 || h.blocks[0].user != callerID || h.blocks[0].peerType != mtproto.PEER_USER || h.blocks[0].peer != authorID {
			t.Fatalf("block %#v", h.blocks)
		}
		if len(h.deletes) != 0 {
			t.Fatalf("delete without flags %#v", h.deletes)
		}
		assertBlockedUpdate(t, up, authorID)
		for _, id := range []int64{callerID, authorID} {
			rec, err := h.book.Load(ctx, id)
			if err != nil || rec.HaveList || rec.Pushed != 0 {
				t.Fatalf("audience %d %+v %v", id, rec, err)
			}
		}
		kept, err := h.book.Load(ctx, otherReply)
		if err != nil || !kept.HaveList || kept.Pushed != 10 {
			t.Fatalf("other sender audience %+v %v", kept, err)
		}
	})

	t.Run("saved_from when from_id is not a user", func(t *testing.T) {
		h.reset()
		h.inbox.put(repliesBox(callerID, 41, 0, savedID))
		h.inbox.rows[callerID][41].Message.FwdFrom.FromId = mtproto.MakePeerChannel(groupChan)
		up, err := h.svc().ContactsBlockFromReplies(ctx, blockReq(false, false, true, 41))
		if err != nil {
			t.Fatal(err)
		}
		if len(h.blocks) != 1 || h.blocks[0].peer != savedID || len(h.deletes) != 0 {
			t.Fatalf("saved_from block %#v delete %#v", h.blocks, h.deletes)
		}
		assertBlockedUpdate(t, up, savedID)
	})

	t.Run("from_id wins over saved_from", func(t *testing.T) {
		h.reset()
		h.inbox.put(repliesBox(callerID, 42, authorID, savedID))
		if _, err := h.svc().ContactsBlockFromReplies(ctx, blockReq(false, false, false, 42)); err != nil {
			t.Fatal(err)
		}
		if len(h.blocks) != 1 || h.blocks[0].peer != authorID {
			t.Fatalf("from_id %#v", h.blocks)
		}
	})

	t.Run("delete_message removes only that id", func(t *testing.T) {
		h.reset()
		if _, err := h.svc().ContactsBlockFromReplies(ctx, blockReq(true, false, false, 200)); err != nil {
			t.Fatal(err)
		}
		if len(h.deletes) != 1 {
			t.Fatalf("deletes %#v", h.deletes)
		}
		got := h.deletes[0]
		if got.user != callerID || got.auth != callerKey || got.peerType != mtproto.PEER_EMPTY || got.peer != 0 || got.revoke {
			t.Fatalf("delete call %#v", got)
		}
		if !sameIDs(got.ids, []int32{200}) {
			t.Fatalf("deleted ids %v", got.ids)
		}
	})

	t.Run("delete_history stays inside this dialog and this sender", func(t *testing.T) {
		h.reset()
		if _, err := h.svc().ContactsBlockFromReplies(ctx, blockReq(false, true, false, 200)); err != nil {
			t.Fatal(err)
		}
		if len(h.deletes) != 1 || h.deletes[0].revoke || h.deletes[0].peerType != mtproto.PEER_EMPTY || h.deletes[0].peer != 0 || h.deletes[0].user != callerID {
			t.Fatalf("delete %#v", h.deletes)
		}
		want := make([]int32, 0, 103)
		for id := int32(101); id <= 200; id++ {
			want = append(want, id)
		}
		// 80 is on the second history page. 42 is the earlier from_id case.
		want = append(want, 80, 42)
		if !sameIDs(h.deletes[0].ids, want) {
			t.Fatalf("history ids %d", len(h.deletes[0].ids))
		}
		for _, id := range h.deletes[0].ids {
			if id == 50 || id == 7 || id == 3 || id == 60 {
				t.Fatalf("deleted foreign id %d", id)
			}
		}
	})

	t.Run("other session blocks its own author only", func(t *testing.T) {
		h.reset()
		up, err := h.svc().ContactsBlockFromReplies(otherCtx, blockReq(true, true, true, 200))
		if err != nil {
			t.Fatal(err)
		}
		if len(h.blocks) != 1 || h.blocks[0].user != otherID || h.blocks[0].peer != foreignID {
			t.Fatalf("other block %#v", h.blocks)
		}
		assertBlockedUpdate(t, up, foreignID)
		if len(h.deletes) != 1 || h.deletes[0].user != otherID || h.deletes[0].peerType != mtproto.PEER_EMPTY || h.deletes[0].peer != 0 || h.deletes[0].revoke {
			t.Fatalf("other delete %#v", h.deletes)
		}
		if !sameIDs(h.deletes[0].ids, []int32{200, 60}) {
			t.Fatalf("other ids %v", h.deletes[0].ids)
		}
		for _, id := range h.deletes[0].ids {
			if id == 80 || id == 50 || id == 7 {
				t.Fatalf("other session deleted caller content %d", id)
			}
		}
	})
}

type repliesHarness struct {
	t          *testing.T
	inbox      *repliesInbox
	blocks     []blockCall
	deletes    []deleteCall
	book       *statusbook.Book
	failDelete error
}

type blockCall struct {
	user     int64
	peerType int32
	peer     int64
}

type deleteCall struct {
	user     int64
	auth     int64
	peerType int32
	peer     int64
	revoke   bool
	ids      []int32
}

func newRepliesHarness(t *testing.T) *repliesHarness {
	t.Helper()
	book := statusbook.New(nil)
	prev := statusbook.Shared()
	statusbook.Install(book)
	t.Cleanup(func() { statusbook.Install(prev) })
	return &repliesHarness{t: t, inbox: newRepliesInbox(), book: book}
}

func (h *repliesHarness) reset() {
	h.blocks = nil
	h.deletes = nil
}

func (h *repliesHarness) svc() ThreadsService {
	return ThreadsService{
		Messages: h.inbox,
		Users:    h,
		Outbox:   h,
		Sync:     h,
	}
}

func (h *repliesHarness) UserGetMutableUsers(_ context.Context, in *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error) {
	var datas []*mtproto.ImmutableUser
	for _, id := range in.GetId() {
		datas = append(datas, &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id}})
	}
	return &user.Vector_ImmutableUser{Datas: datas}, nil
}

func (h *repliesHarness) UserBlockPeer(_ context.Context, in *user.TLUserBlockPeer) (*mtproto.Bool, error) {
	h.blocks = append(h.blocks, blockCall{user: in.GetUserId(), peerType: in.GetPeerType(), peer: in.GetPeerId()})
	return mtproto.BoolTrue, nil
}

func (h *repliesHarness) MsgDeleteMessages(_ context.Context, in *msg.TLMsgDeleteMessages) (*mtproto.Messages_AffectedMessages, error) {
	if h.failDelete != nil {
		return nil, h.failDelete
	}
	if in.GetPeerType() != mtproto.PEER_EMPTY || in.GetPeerId() != 0 || in.GetRevoke() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	h.deletes = append(h.deletes, deleteCall{
		user: in.GetUserId(), auth: in.GetAuthKeyId(), peerType: in.GetPeerType(),
		peer: in.GetPeerId(), revoke: in.GetRevoke(), ids: append([]int32(nil), in.GetId()...),
	})
	return &mtproto.Messages_AffectedMessages{}, nil
}

func (h *repliesHarness) SyncUpdatesNotMe(_ context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	if in.GetUpdates() != nil {
		for _, up := range in.GetUpdates().GetUpdates() {
			if up.GetMessage_MESSAGE() != nil {
				h.t.Fatal("sync carried a message body")
			}
		}
	}
	return mtproto.MakeTLVoid(nil).To_Void(), nil
}

type repliesInbox struct {
	rows map[int64]map[int32]*mtproto.MessageBox
}

func newRepliesInbox() *repliesInbox {
	return &repliesInbox{rows: map[int64]map[int32]*mtproto.MessageBox{}}
}

func (n *repliesInbox) put(box *mtproto.MessageBox) {
	if n.rows[box.GetUserId()] == nil {
		n.rows[box.GetUserId()] = map[int32]*mtproto.MessageBox{}
	}
	n.rows[box.GetUserId()][box.GetMessageId()] = box
}

func (n *repliesInbox) MessageGetUserMessage(_ context.Context, in *message.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	row := n.rows[in.GetUserId()][in.GetId()]
	if row == nil {
		return nil, mtproto.ErrMsgIdInvalid
	}
	return row, nil
}

func (n *repliesInbox) MessageGetHistoryMessages(_ context.Context, in *message.TLMessageGetHistoryMessages) (*message.Vector_MessageBox, error) {
	var page []*mtproto.MessageBox
	for _, box := range n.rows[in.GetUserId()] {
		if box.GetPeerType() != in.GetPeerType() || box.GetPeerId() != in.GetPeerId() {
			continue
		}
		if in.GetOffsetId() != 0 && box.GetMessageId() >= in.GetOffsetId() {
			continue
		}
		page = append(page, box)
	}
	sort.Slice(page, func(i, j int) bool { return page[i].GetMessageId() > page[j].GetMessageId() })
	limit := int(in.GetLimit())
	if limit > 0 && len(page) > limit {
		page = page[:limit]
	}
	return &message.Vector_MessageBox{Datas: page}, nil
}

func userCtx(t *testing.T, user, key int64) context.Context {
	t.Helper()
	out, err := rpcmd.RpcMetadataToOutgoing(context.Background(), &rpcmd.RpcMetadata{UserId: user, PermAuthKeyId: key})
	if err != nil {
		t.Fatal(err)
	}
	md, _ := metadata.FromOutgoingContext(out)
	return metadata.NewIncomingContext(context.Background(), md)
}

func blockReq(deleteMessage, deleteHistory, reportSpam bool, id int32) *mtproto.TLContactsBlockFromReplies {
	return &mtproto.TLContactsBlockFromReplies{
		DeleteMessage: deleteMessage,
		DeleteHistory: deleteHistory,
		ReportSpam:    reportSpam,
		MsgId:         id,
	}
}

func repliesBox(owner int64, id int32, from, saved int64) *mtproto.MessageBox {
	fwd := &mtproto.MessageFwdHeader{}
	if from != 0 {
		fwd.FromId = mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: from}).To_Peer()
	}
	if saved != 0 {
		fwd.SavedFromId = mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: saved}).To_Peer()
	}
	return &mtproto.MessageBox{
		UserId: owner, MessageId: id, SenderUserId: repliesUserID,
		PeerType: mtproto.PEER_USER, PeerId: repliesUserID,
		Message: &mtproto.Message{Message: "m", FwdFrom: fwd},
	}
}

func channelBox(owner int64, id int32, sender, channel int64) *mtproto.MessageBox {
	return &mtproto.MessageBox{
		UserId: owner, MessageId: id, SenderUserId: sender,
		PeerType: mtproto.PEER_CHANNEL, PeerId: channel,
		Message: &mtproto.Message{Message: "m"},
	}
}

func privateBox(owner int64, id int32, sender, peer int64) *mtproto.MessageBox {
	return &mtproto.MessageBox{
		UserId: owner, MessageId: id, SenderUserId: sender,
		PeerType: mtproto.PEER_USER, PeerId: peer,
		Message: &mtproto.Message{Message: "m"},
	}
}

func assertMsgIDInvalid(t *testing.T, err error) {
	t.Helper()
	st := status.Convert(err)
	if st.Code() != 400 || st.Message() != "MSG_ID_INVALID" {
		t.Fatalf("got %d %s", st.Code(), st.Message())
	}
}

func assertBlockedUpdate(t *testing.T, up *mtproto.Updates, author int64) {
	t.Helper()
	if up == nil || len(up.GetUpdates()) != 1 {
		t.Fatalf("updates %d", len(up.GetUpdates()))
	}
	one := up.GetUpdates()[0]
	if one.GetPredicateName() != mtproto.Predicate_updatePeerBlocked || one.GetMessage_MESSAGE() != nil {
		t.Fatalf("update %s", one.GetPredicateName())
	}
	if one.GetPeerId().GetUserId() != author || !one.GetBlocked_FLAGBOOLEAN() {
		t.Fatalf("blocked peer %d", one.GetPeerId().GetUserId())
	}
	for _, u := range up.GetUsers() {
		if u.GetId() != callerID && u.GetId() != otherID && u.GetId() != author && u.GetId() != savedID && u.GetId() != foreignID {
			t.Fatalf("unexpected user %d", u.GetId())
		}
	}
}

func TestRepliesForwardJSONNamesTheAuthor(t *testing.T) {
	const raw = `{"predicate_name":"message","fwd_from":{"from_id":{"predicate_name":"peerUser","user_id":15}}}`
	var msg mtproto.Message
	if err := jsonx.UnmarshalFromString(raw, &msg); err != nil {
		t.Fatal(err)
	}
	box := &mtproto.MessageBox{
		SenderUserId: repliesUserID,
		PeerType:     mtproto.PEER_USER,
		PeerId:       repliesUserID,
		Message:      &msg,
	}
	if !isRepliesBox(box) || replySender(box) != 15 {
		t.Fatalf("sender %d", replySender(box))
	}
}

func TestBlockFromRepliesReturnsDeleteFailure(t *testing.T) {
	h := newRepliesHarness(t)
	h.inbox.put(repliesBox(callerID, 40, authorID, 0))
	h.failDelete = mtproto.ErrPeerIdInvalid
	ctx := userCtx(t, callerID, callerKey)
	_, err := h.svc().ContactsBlockFromReplies(ctx, blockReq(true, false, false, 40))
	if status.Convert(err).Code() != 400 || status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatalf("delete error %v", err)
	}
	if len(h.blocks) != 1 || h.blocks[0].peer != authorID {
		t.Fatalf("block %#v", h.blocks)
	}
	if len(h.deletes) != 0 {
		t.Fatalf("failed delete recorded %#v", h.deletes)
	}
}

func sameIDs(got, want []int32) bool {
	if len(got) != len(want) {
		return false
	}
	a := append([]int32(nil), got...)
	b := append([]int32(nil), want...)
	sort.Slice(a, func(i, j int) bool { return a[i] < a[j] })
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
