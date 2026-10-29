package push

import (
	"context"
	"sync"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type captureFCM struct {
	mu    sync.Mutex
	token string
	body  []byte
}

func (c *captureFCM) Send(_ context.Context, token string, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
	c.body = append([]byte(nil), payload...)
	return nil
}

type memSettings struct {
	MemoryStore
	peer   NotifySettings
	global NotifySettings
}

func (m *memSettings) NotifySettings(_ context.Context, _ int64, peerType int32, peerID int64) (NotifySettings, error) {
	if peerID == 0 {
		return m.global, nil
	}
	_ = peerType
	return m.peer, nil
}

func TestDeliverRespectsMuteAndSendsFCM(t *testing.T) {
	store := &memSettings{
		peer: NotifySettings{Found: true, ShowPreviews: 1, Silent: 0, MuteUntil: 0},
	}
	_ = store.Upsert(context.Background(), Device{
		AuthKeyID: 10, UserID: 9, TokenType: TokenFCM, Token: "fcm-token", Secret: bytesRepeat(256),
	})
	_ = store.Upsert(context.Background(), Device{
		AuthKeyID: 10, UserID: 9, TokenType: TokenAPNSVoIP, Token: "voip-token", Secret: bytesRepeat(256),
	})
	fcm := &captureFCM{}
	svc := NewService(store, Config{})
	svc.SetFCM(fcm)
	svc.Deliver(context.Background(), 9, textUpdate())
	if fcm.token != "fcm-token" || len(fcm.body) == 0 {
		t.Fatalf("fcm token %q body %d", fcm.token, len(fcm.body))
	}

	store.peer.MuteUntil = int32(svc.now().Unix() + 3600)
	fcm.token = ""
	svc.Deliver(context.Background(), 9, textUpdate())
	if fcm.token != "" {
		t.Fatal("muted chat was pushed")
	}
}

func TestRegisterMemoryStorePerAuthKey(t *testing.T) {
	s := &MemoryStore{}
	ctx := context.Background()
	d := Device{AuthKeyID: 5, UserID: 9, TokenType: TokenAPNS, Token: "abc", AppSandbox: true, NoMuted: true, Secret: []byte{1, 2, 3}, OtherUIDs: []int64{8}}
	if err := s.Upsert(ctx, d); err != nil {
		t.Fatal(err)
	}
	d.Token = "def"
	if err := s.Upsert(ctx, d); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListByUser(ctx, 9)
	if err != nil || len(list) != 1 || list[0].Token != "def" || !list[0].AppSandbox || !list[0].NoMuted || len(list[0].Secret) != 3 {
		t.Fatalf("%+v %v", list, err)
	}
	if err := s.Delete(ctx, 9, TokenAPNS, "def", []int64{8}); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListByUser(ctx, 9)
	if len(list) != 0 {
		t.Fatal(list)
	}
}

func textUpdate() *mtproto.Updates {
	user := mtproto.MakeTLUser(&mtproto.User{
		Id:        3,
		FirstName: &wrapperspb.StringValue{Value: "Ada"},
	}).To_User()
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Out: false, Id: 4, Message: "ping",
		FromId: mtproto.MakePeerUser(3), PeerId: mtproto.MakePeerUser(3),
	}).To_Message()
	up := mtproto.MakeUpdatesByUpdatesUsers([]*mtproto.User{user},
		mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: msg}).To_Update())
	return up
}

func bytesRepeat(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = 0x42
	}
	return b
}
