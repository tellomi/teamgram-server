package dao

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	idgenpb "github.com/teamgram/teamgram-server/app/service/idgen/idgen"
	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDeliverRepliesNoticeWritesInboxOnce(t *testing.T) {
	d := openInboxApplyDB(t)
	d.IDGenClient2 = idgen_client.NewIDGenClient2Direct(&replySeq{})
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
	if !stored.GetSilent() {
		t.Fatal("hidden preview was not silent")
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
