package core

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageGetUserMessageList is what messages.getMessages reads. A deleted
// row uses the DAO update (deleted=1 and message_data cleared). The same box
// id on another user stays, and is not returned for the caller.
func TestGetMessagesAbsentDeletedOtherUserMySQL(t *testing.T) {
	db, dsn := newRepliesSchema(t)
	msgCore := newRepliesMessageCore(t, dsn)
	const (
		caller  = int64(7)
		other   = int64(9)
		peer    = int64(8)
		liveID  = int32(10)
		neverID = int32(1999999999)
	)
	insertPlainBox(t, db, caller, liveID, peer, "secret-live")
	insertPlainBox(t, db, other, liveID, peer, "other-secret")
	ctx := context.Background()
	before := listBoxes(t, msgCore, caller, liveID, neverID)
	if len(before) != 1 || before[0].GetMessage().GetMessage() != "secret-live" || before[0].GetMessageId() != liveID {
		t.Fatalf("before %#v", before)
	}
	affected, err := msgCore.svcCtx.Dao.MessagesDAO.DeleteMessagesByMessageIdList(ctx, caller, []int32{liveID})
	if err != nil {
		t.Fatal(err)
	}
	if affected != 1 {
		t.Fatalf("deleted rows %d", affected)
	}
	var deleted int
	var data string
	if err = db.QueryRow(`SELECT deleted, message_data FROM messages WHERE user_id=? AND user_message_box_id=?`, caller, liveID).Scan(&deleted, &data); err != nil {
		t.Fatal(err)
	}
	if deleted != 1 || strings.Contains(data, "secret-live") {
		t.Fatalf("stored deleted=%d data=%s", deleted, data)
	}
	after := listBoxes(t, msgCore, caller, liveID, neverID)
	if len(after) != 0 {
		t.Fatalf("caller still sees %#v", after)
	}
	kept := listBoxes(t, msgCore, other, liveID)
	if len(kept) != 1 || kept[0].GetMessage().GetMessage() != "other-secret" {
		t.Fatalf("other user %#v", kept)
	}
	if leaked := listBoxes(t, msgCore, caller, liveID); len(leaked) != 0 {
		t.Fatalf("caller read other user's id %#v", leaked)
	}
}

func listBoxes(t *testing.T, msgCore *MessageCore, user int64, ids ...int32) []*mtproto.MessageBox {
	t.Helper()
	got, err := New(context.Background(), msgCore.svcCtx).MessageGetUserMessageList(&message.TLMessageGetUserMessageList{
		UserId: user,
		IdList: ids,
	})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDatas()
}

func insertPlainBox(t *testing.T, db *sql.DB, user int64, box int32, peer int64, text string) {
	t.Helper()
	raw := fmt.Sprintf(`{"predicate_name":"message","id":%d,"message":%q,"date":10,"peer_id":{"predicate_name":"peerUser","user_id":%d},"from_id":{"predicate_name":"peerUser","user_id":%d}}`, box, text, peer, user)
	_, err := db.Exec(`INSERT INTO messages (user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, message_data, message, date2, ttl_period, deleted)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,0)`,
		user, box, user, peer, int64(box), user, mtproto.PEER_USER, peer, raw, text, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
}
