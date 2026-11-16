package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/business/profile"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

func boxAt(date int32) *mtproto.MessageBox {
	return &mtproto.MessageBox{MessageId: date, Message: &mtproto.Message{Date: date}}
}

type scriptedHist struct {
	messageclient.MessageClient
	fail map[int64]error
	rows map[int64][]*mtproto.MessageBox
}

func (s scriptedHist) MessageGetHistoryMessages(_ context.Context, in *message.TLMessageGetHistoryMessages) (*message.Vector_MessageBox, error) {
	if err := s.fail[in.GetUserId()]; err != nil {
		return nil, err
	}
	return &message.Vector_MessageBox{Datas: s.rows[in.GetUserId()]}, nil
}

func (s scriptedHist) MessageGetHistoryMessagesCount(context.Context, *message.TLMessageGetHistoryMessagesCount) (*mtproto.Int32, error) {
	return nil, errors.New("count down")
}

func TestPriorByAgeDoesNotInventANewChat(t *testing.T) {
	now := int32(1_700_000_000)
	c := &MessagesCore{ctx: context.Background(), svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
		MessageClient: scriptedHist{
			fail: map[int64]error{4: errors.New("outbox down")},
			rows: map[int64][]*mtproto.MessageBox{9: {boxAt(now - 10)}},
		},
	}}}
	prior, known := c.priorPrivateMessage(9, 4, now, sentMark{})
	if known || prior != 0 {
		t.Fatalf("single current inbox row %d known=%v", prior, known)
	}

	c.svcCtx.Dao.MessageClient = scriptedHist{
		fail: map[int64]error{4: errors.New("outbox down")},
		rows: map[int64][]*mtproto.MessageBox{9: {boxAt(now - 200)}},
	}
	prior, known = c.priorPrivateMessage(9, 4, now, sentMark{})
	if !known || prior != now-200 {
		t.Fatalf("older inbox row %d known=%v", prior, known)
	}

	c.svcCtx.Dao.MessageClient = scriptedHist{
		fail: map[int64]error{4: errors.New("outbox down")},
		rows: map[int64][]*mtproto.MessageBox{9: {boxAt(now - 10), boxAt(now - 500)}},
	}
	prior, known = c.priorPrivateMessage(9, 4, now, sentMark{})
	if !known || prior != now-500 {
		t.Fatalf("earlier of two %d known=%v", prior, known)
	}
}

type idleMsg struct{ msgclient.MsgClient }

type blockHist struct {
	messageclient.MessageClient
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockHist) MessageGetHistoryMessages(ctx context.Context, _ *message.TLMessageGetHistoryMessages) (*message.Vector_MessageBox, error) {
	b.once.Do(func() { close(b.started) })
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return nil, errors.New("down")
}

func (b *blockHist) MessageGetHistoryMessagesCount(context.Context, *message.TLMessageGetHistoryMessagesCount) (*mtproto.Int32, error) {
	return nil, errors.New("count down")
}

func TestScheduleBusinessAutoReplyDoesNotBlockSend(t *testing.T) {
	profile.ResetShared()
	t.Cleanup(profile.ResetShared)
	if err := profile.Shared().SetGreeting(9, mtproto.MakeTLInputBusinessGreetingMessage(&mtproto.InputBusinessGreetingMessage{
		ShortcutId: 2,
		Recipients: mtproto.MakeTLInputBusinessRecipients(&mtproto.InputBusinessRecipients{NonContacts: true}).To_InputBusinessRecipients(),
	}).To_InputBusinessGreetingMessage()); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	c := &MessagesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MsgClient:     idleMsg{},
			MessageClient: &blockHist{started: started, release: release},
		}},
		MD: &metadata.RpcMetadata{UserId: 2},
	}
	done := make(chan struct{})
	go func() {
		c.scheduleBusinessAutoReply(&mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: 9}, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("send path waited for the auto reply")
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("auto reply did not start")
	}
	close(release)
}
