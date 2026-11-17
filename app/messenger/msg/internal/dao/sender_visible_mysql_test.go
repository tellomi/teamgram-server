package dao

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/zeromicro/go-zero/core/jsonx"
)

// TestSavedForwardRowIsVisibleBeforeTheInboxConsumer is the MySQL half of
// the saved-tag miss. messages.search(peer=self, q="", saved_reaction) loads
// boxes with MessageGetUserMessageList, which is SelectByMessageIdList
// (user_id, user_message_box_id, deleted=0). On master, sendMessageToOutboxV2
// returns that id before any messages row exists; the inbox consumer inserts
// it after the Kafka ack. This branch already calls SendMessageToOutboxV1NoPts
// from sendMessageToOutboxV2 before publishInboxFanout. The consumer here is
// that same insert, held on a channel. There is no sleep and no second search.
func TestSavedForwardRowIsVisibleBeforeTheInboxConsumer(t *testing.T) {
	d := openInboxApplyDB(t)
	if _, err := d.DB.Exec(context.Background(), `ALTER TABLE messages
		ADD COLUMN has_reaction TINYINT NOT NULL DEFAULT 0,
		ADD COLUMN reaction VARCHAR(32) NOT NULL DEFAULT '',
		ADD COLUMN reaction_date BIGINT NOT NULL DEFAULT 0,
		ADD COLUMN reaction_unread TINYINT NOT NULL DEFAULT 0`); err != nil {
		t.Fatal(err)
	}
	// Production dialogs has these columns. The fixture table does not, and
	// stampHistoryTTL must see "no timer", not an unknown-column error.
	if _, err := d.DB.Exec(context.Background(), `ALTER TABLE dialogs
		ADD COLUMN deleted TINYINT NOT NULL DEFAULT 0,
		ADD COLUMN ttl_period INT NOT NULL DEFAULT 0`); err != nil {
		t.Fatal(err)
	}
	d.DialogClient = noopOutboxDialog{}

	const (
		self   int64 = 910442061
		box    int32 = 44
		random int64 = 910442063
		text         = "saved-src-visible-910442061"
	)
	peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: self}
	stored := savedOutbox(self, peer, box, random, text)

	if got := searchableText(t, d, self, box); got != "" {
		t.Fatalf("search saw %q before the sender persist", got)
	}

	release := make(chan struct{})
	started := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		close(started)
		<-release
		finished <- d.SendMessageToOutboxV1NoPts(context.Background(), self, peer, stored)
	}()
	<-started

	if err := d.SendMessageToOutboxV1NoPts(context.Background(), self, peer, stored); err != nil {
		t.Fatal(err)
	}
	if got := searchableText(t, d, self, box); got != text {
		t.Fatalf("search while the inbox consumer is held: %q", got)
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM messages WHERE user_id=? AND deleted=0`, self); n != 1 {
		t.Fatalf("rows while the consumer is held: %d", n)
	}

	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if got := searchableText(t, d, self, box); got != text {
		t.Fatalf("search after the delayed consumer: %q", got)
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM messages WHERE user_id=? AND deleted=0`, self); n != 1 {
		t.Fatalf("rows after the delayed consumer: %d", n)
	}
}

func savedOutbox(self int64, peer *mtproto.PeerUtil, box int32, random int64, text string) *mtproto.MessageBox {
	dialogID := mtproto.MakeDialogId(self, peer.PeerType, peer.PeerId)
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Out:         true,
		Id:          box,
		Date:        box,
		Message:     text,
		PeerId:      mtproto.MakePeerUser(self),
		FromId:      mtproto.MakePeerUser(self),
		SavedPeerId: mtproto.MakePeerUser(self),
	}).To_Message()
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		UserId:          self,
		MessageId:       box,
		SenderUserId:    self,
		PeerType:        peer.PeerType,
		PeerId:          peer.PeerId,
		RandomId:        random,
		DialogId1:       dialogID.A,
		DialogId2:       dialogID.B,
		DialogMessageId: int64(box),
		Message:         msg,
	}).To_MessageBox()
}

// searchableText is the body messages.search would attach for this box id.
// MessageGetUserMessageList reads SelectByMessageIdList and unmarshals
// message_data. An empty string means that read did not return the row.
func searchableText(t *testing.T, d *Dao, user int64, box int32) string {
	t.Helper()
	rows, err := d.MessagesDAO.SelectByMessageIdList(context.Background(), user, []int32{box})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		return ""
	}
	var msg *mtproto.Message
	if err = jsonx.UnmarshalFromString(rows[0].MessageData, &msg); err != nil {
		t.Fatal(err)
	}
	if msg == nil {
		t.Fatal("message_data did not unmarshal")
	}
	return msg.GetMessage()
}

type noopOutboxDialog struct {
	dialogclient.DialogClient
}

func (noopOutboxDialog) DialogInsertOrUpdateDialog(context.Context, *dialog.TLDialogInsertOrUpdateDialog) (*mtproto.Bool, error) {
	return mtproto.ToBool(true), nil
}
