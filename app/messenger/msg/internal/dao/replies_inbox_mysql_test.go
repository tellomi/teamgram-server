package dao

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	idgenpb "github.com/teamgram/teamgram-server/app/service/idgen/idgen"
	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDeliverRepliesNoticeWritesInboxOnce(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_DSN") != "" {
		t.Setenv("TEAMGRAM_REQUIRE_INBOX_MYSQL", "1")
	}
	d := openInboxApplyDB(t)
	d.IDGenClient2 = idgen_client.NewIDGenClient2Direct(&replySeq{})
	pushed := &replyPush{}
	d.SyncClient = pushed
	ctx := context.Background()
	const (
		author    int64 = 910442051
		commenter int64 = 910442052
		other     int64 = 910442053
		groupID   int64 = 910442054
		commentID int32 = 9
	)
	notice := channel.RepliesNotice{
		Recipient: author,
		Commenter: commenter,
		GroupID:   groupID,
		CommentID: commentID,
		ParentID:  4,
		RandomID:  910442055,
		Message: mtproto.MakeTLMessage(&mtproto.Message{
			Message: "reply-note",
			Date:    80,
			FwdFrom: mtproto.MakeTLMessageFwdHeader(&mtproto.MessageFwdHeader{
				FromId:         mtproto.MakePeerUser(commenter),
				Date:           80,
				SavedFromPeer:  mtproto.MakePeerChannel(groupID),
				SavedFromMsgId: wrapperspb.Int32(commentID),
			}).To_MessageFwdHeader(),
			ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
				ReplyToMsgId:           4,
				ReplyToMsgId_INT32:     4,
				ReplyToMsgId_FLAGINT32: wrapperspb.Int32(4),
				ReplyToPeerId:          mtproto.MakePeerChannel(groupID),
			}).To_MessageReplyHeader(),
		}).To_Message(),
	}
	if err := d.DeliverRepliesNotice(ctx, notice, true, nil, 100); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM messages WHERE user_id=?`, author); n != 0 {
		t.Fatalf("blocked write stored %d", n)
	}
	if err := d.DeliverRepliesNotice(ctx, notice, false, []*mtproto.PeerNotifySettings{
		mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{ShowPreviews: mtproto.ToBool(false)}).To_PeerNotifySettings(),
	}, 100); err != nil {
		t.Fatal(err)
	}
	if err := d.DeliverRepliesNotice(ctx, notice, false, nil, 100); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM messages WHERE user_id=?`, author); n != 1 {
		t.Fatalf("rows %d", n)
	}
	var row struct {
		Box    int32  `db:"user_message_box_id"`
		Sender int64  `db:"sender_user_id"`
		Peer   int64  `db:"peer_id"`
		Kind   int32  `db:"peer_type"`
		Text   string `db:"message"`
		Data   string `db:"message_data"`
	}
	if err := d.DB.QueryRow(ctx, &row, `SELECT user_message_box_id, sender_user_id, peer_id, peer_type, message, message_data FROM messages WHERE user_id=?`, author); err != nil {
		t.Fatal(err)
	}
	if row.Sender != channel.RepliesUserID || row.Peer != channel.RepliesUserID || row.Kind != mtproto.PEER_USER {
		t.Fatalf("row sender %d peer %d kind %d", row.Sender, row.Peer, row.Kind)
	}
	if row.Box == 0 || row.Box == commentID {
		t.Fatalf("inbox id %d comment %d", row.Box, commentID)
	}
	if row.Text != "reply-note" {
		t.Fatalf("text %q", row.Text)
	}
	var stored mtproto.Message
	if err := jsonx.Unmarshal([]byte(row.Data), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.GetSilent() {
		t.Fatal("preview off marked the protocol message silent")
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM user_pts_updates WHERE user_id=?`, author); n != 1 {
		t.Fatalf("pts rows %d", n)
	}
	var raw struct {
		Data string `db:"update_data"`
	}
	if err := d.DB.QueryRow(ctx, &raw, `SELECT update_data FROM user_pts_updates WHERE user_id=?`, author); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.Data, "reply-note") || !strings.Contains(raw.Data, "updateNewMessage") {
		t.Fatal("pts update is not the replies message")
	}
	pushed.mu.Lock()
	defer pushed.mu.Unlock()
	if len(pushed.users) == 0 || pushed.users[0] != author {
		t.Fatalf("push %#v", pushed.users)
	}
	for _, user := range pushed.users {
		if user != author {
			t.Fatalf("pushed to %d", user)
		}
	}
	if len(pushed.texts) == 0 || pushed.texts[0] != "reply-note" || pushed.silent[0] {
		t.Fatalf("push text %#v silent %#v", pushed.texts, pushed.silent)
	}
	if stored.GetFwdFrom().GetFromId().GetUserId() != commenter {
		t.Fatalf("fwd %#v", stored.GetFwdFrom())
	}
	if stored.GetFwdFrom().GetSavedFromMsgId().GetValue() != commentID || stored.GetFwdFrom().GetSavedFromPeer().GetChannelId() != groupID {
		t.Fatalf("source %#v", stored.GetFwdFrom())
	}
	if stored.GetId() == commentID {
		t.Fatalf("stored message id %d is the channel id", stored.GetId())
	}
	if _, err := d.DB.Exec(ctx, `INSERT INTO messages (user_id, user_message_box_id, sender_user_id, peer_type, peer_id, random_id, message_data, message, deleted) VALUES (?,?,?,?,?,?,?,?,0)`,
		other, row.Box, 77, mtproto.PEER_USER, 77, 1, `{"predicate_name":"message","message":"other-account"}`, "other-account"); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := queryMessage(t, d, other, row.Box, &got); err != nil || got != "other-account" {
		t.Fatalf("other account %q %v", got, err)
	}
	if err := queryMessage(t, d, author, row.Box, &got); err != nil || got != "reply-note" {
		t.Fatalf("author account %q %v", got, err)
	}
}

func TestDeliverRepliesNoticeConcurrentOneRow(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_DSN") != "" {
		t.Setenv("TEAMGRAM_REQUIRE_INBOX_MYSQL", "1")
	}
	d := openInboxApplyDB(t)
	ctx := context.Background()
	const (
		user   int64 = 910442061
		sender int64 = 910442062
		random int64 = 910442063
		first  int32 = 11
		second int32 = 12
	)
	entered := make(chan struct{})
	release := make(chan struct{})
	tx1 := make(chan error, 1)
	go func() {
		tx1 <- d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
			if _, _, err := d.MessagesDAO.InsertOrReturnIdTx(tx, inboxMessageDO(user, sender, first, random, "one")); err != nil {
				return err
			}
			if err := rememberInboxRandom(tx, user, sender, random, first); err != nil {
				return err
			}
			if _, _, err := d.applyInboxInTx(ctx, tx, inboxApplyRow{
				UserID: user, BoxID: first, PeerType: mtproto.PEER_USER, PeerID: sender,
				UnreadDelta: 1, Date2: 11,
			}, textMessage(first, "one")); err != nil {
				return err
			}
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	tx2 := make(chan error, 1)
	go func() {
		tx2 <- d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
			if _, _, err := d.MessagesDAO.InsertOrReturnIdTx(tx, inboxMessageDO(user, sender, second, random, "two")); err != nil {
				return err
			}
			return rememberInboxRandom(tx, user, sender, random, second)
		})
	}()
	select {
	case err := <-tx2:
		t.Fatalf("second transaction finished before the first committed: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	close(release)
	if err := <-tx1; err != nil {
		t.Fatal(err)
	}
	if err := <-tx2; err == nil {
		t.Fatal("second box committed")
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM messages WHERE user_id=?`, user); n != 1 {
		t.Fatalf("messages %d", n)
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM inbox_random WHERE user_id=?`, user); n != 1 {
		t.Fatalf("random rows %d", n)
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM user_pts_updates WHERE user_id=?`, user); n != 1 {
		t.Fatalf("pts rows %d", n)
	}
	var got string
	if err := queryMessage(t, d, user, first, &got); err != nil || got != "one" {
		t.Fatalf("kept %q %v", got, err)
	}
}

type replyPush struct {
	mu     sync.Mutex
	users  []int64
	texts  []string
	silent []bool
}

func (p *replyPush) SyncUpdatesMe(context.Context, *syncpb.TLSyncUpdatesMe) (*mtproto.Void, error) {
	return mtproto.MakeTLVoid(nil).To_Void(), nil
}
func (p *replyPush) SyncUpdatesNotMe(context.Context, *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	return mtproto.MakeTLVoid(nil).To_Void(), nil
}
func (p *replyPush) SyncPushUpdates(_ context.Context, in *syncpb.TLSyncPushUpdates) (*mtproto.Void, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.users = append(p.users, in.GetUserId())
	for _, upd := range in.GetUpdates().GetUpdates() {
		msg := upd.GetMessage_MESSAGE()
		p.texts = append(p.texts, msg.GetMessage())
		p.silent = append(p.silent, msg.GetSilent())
	}
	return mtproto.MakeTLVoid(nil).To_Void(), nil
}
func (p *replyPush) SyncPushUpdatesIfNot(context.Context, *syncpb.TLSyncPushUpdatesIfNot) (*mtproto.Void, error) {
	return mtproto.MakeTLVoid(nil).To_Void(), nil
}
func (p *replyPush) SyncPushBotUpdates(context.Context, *syncpb.TLSyncPushBotUpdates) (*mtproto.Void, error) {
	return mtproto.MakeTLVoid(nil).To_Void(), nil
}
func (p *replyPush) SyncPushRpcResult(context.Context, *syncpb.TLSyncPushRpcResult) (*mtproto.Void, error) {
	return mtproto.MakeTLVoid(nil).To_Void(), nil
}
func (p *replyPush) SyncBroadcastUpdates(context.Context, *syncpb.TLSyncBroadcastUpdates) (*mtproto.Void, error) {
	return mtproto.MakeTLVoid(nil).To_Void(), nil
}

type replySeq struct{ n atomic.Int64 }

func (s *replySeq) IdgenNextId(context.Context, *idgenpb.TLIdgenNextId) (*mtproto.Int64, error) {
	return &mtproto.Int64{V: s.n.Add(1)}, nil
}

func (s *replySeq) IdgenNextIds(context.Context, *idgenpb.TLIdgenNextIds) (*idgenpb.Vector_Long, error) {
	return &idgenpb.Vector_Long{}, nil
}

func (s *replySeq) IdgenGetCurrentSeqId(context.Context, *idgenpb.TLIdgenGetCurrentSeqId) (*mtproto.Int64, error) {
	return &mtproto.Int64{V: s.n.Load()}, nil
}

func (s *replySeq) IdgenSetCurrentSeqId(context.Context, *idgenpb.TLIdgenSetCurrentSeqId) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func (s *replySeq) IdgenGetNextSeqId(context.Context, *idgenpb.TLIdgenGetNextSeqId) (*mtproto.Int64, error) {
	return &mtproto.Int64{V: s.n.Add(1)}, nil
}

func (s *replySeq) IdgenGetNextNSeqId(context.Context, *idgenpb.TLIdgenGetNextNSeqId) (*mtproto.Int64, error) {
	return &mtproto.Int64{V: s.n.Add(1)}, nil
}

func (s *replySeq) IdgenGetNextIdValList(context.Context, *idgenpb.TLIdgenGetNextIdValList) (*idgenpb.Vector_IdVal, error) {
	return &idgenpb.Vector_IdVal{}, nil
}

func (s *replySeq) IdgenGetCurrentSeqIdList(context.Context, *idgenpb.TLIdgenGetCurrentSeqIdList) (*idgenpb.Vector_IdVal, error) {
	return &idgenpb.Vector_IdVal{}, nil
}
