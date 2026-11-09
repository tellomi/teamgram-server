package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"
)

func TestTellomiPrivateOnlyTheServiceAccount(t *testing.T) {
	user := &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: tellomiai.AccountID}
	if !tellomiPrivate(9, user) {
		t.Fatal("a person messaging @tellomi should be eligible")
	}
	if tellomiPrivate(tellomiai.AccountID, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: 9}) {
		t.Fatal("the service account must not reply to itself")
	}
	if tellomiPrivate(9, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: 8}) {
		t.Fatal("other peers are not @tellomi")
	}
	if tellomiPrivate(9, &mtproto.PeerUtil{PeerType: mtproto.PEER_CHAT, PeerId: tellomiai.AccountID}) {
		t.Fatal("a basic group is not a private chat with @tellomi")
	}
	if tellomiPrivate(0, user) {
		t.Fatal("anonymous sender")
	}
}

func TestMaybeTellomiDefaultOff(t *testing.T) {
	if tellomiai.Enabled() {
		t.Fatal("TELLOMI_AI is on in this test")
	}
	maybeTellomi(context.Background(), 9, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: tellomiai.AccountID}, "hi", 1, 0)
	c := &MessagesCore{ctx: context.Background(), MD: &metadata.RpcMetadata{IsBot: true}}
	if !c.skipGroup(9, &mtproto.PeerUtil{PeerType: mtproto.PEER_CHAT, PeerId: 4}) {
		t.Fatal("bots are ignored in groups")
	}
	person := &MessagesCore{ctx: context.Background(), MD: &metadata.RpcMetadata{}}
	if person.skipGroup(9, &mtproto.PeerUtil{PeerType: mtproto.PEER_CHANNEL, PeerId: 70}) {
		t.Fatal("a person in a supergroup can be answered")
	}
	if !person.skipGroup(9, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: 8}) {
		t.Fatal("a private chat with someone else is not a group turn")
	}
	c.noteTellomi(9, &mtproto.PeerUtil{PeerType: mtproto.PEER_CHAT, PeerId: 4}, "@tellomi hi", nil, 1, 0, nil)
}

func TestBasicMemberAndSingleMessageRead(t *testing.T) {
	chat := &mtproto.MutableChat{ChatParticipants: []*mtproto.ImmutableChatParticipant{
		{UserId: tellomiai.AccountID, State: mtproto.ChatMemberStateNormal},
		{UserId: 9, State: mtproto.ChatMemberStateLeft},
	}}
	if !basicMember(chat, tellomiai.AccountID) {
		t.Fatal("777001 is a current member")
	}
	if basicMember(chat, 9) {
		t.Fatal("a member who left cannot post")
	}
	if basicMember(nil, tellomiai.AccountID) {
		t.Fatal("nil chat")
	}
	left := &mtproto.MutableChat{Chat: &mtproto.ImmutableChat{Deactivated: true}, ChatParticipants: chat.ChatParticipants}
	if basicMember(left, tellomiai.AccountID) {
		t.Fatal("deactivated chat")
	}

	fake := &fakeMsgs{box: &mtproto.MessageBox{
		MessageId:    55,
		SenderUserId: tellomiai.AccountID,
		Message:      mtproto.MakeTLMessage(&mtproto.Message{Message: "上一句"}).To_Message(),
	}}
	c := &MessagesCore{ctx: context.Background(), svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: fake}}}
	replied, replyToAI, replyID, ok := c.groupContext(context.Background(), 9, mtproto.PEER_CHAT, 4, 8, 3)
	if !ok || !replyToAI || replied != "上一句" || replyID != 55 {
		t.Fatalf("replied %q ai=%v id=%d ok=%v", replied, replyToAI, replyID, ok)
	}
	if fake.userCalls != 1 || fake.peerCalls != 1 || fake.historyCalls != 0 {
		t.Fatalf("reads user=%d peer=%d history=%d", fake.userCalls, fake.peerCalls, fake.historyCalls)
	}
	if fake.userIn.GetUserId() != 9 || fake.userIn.GetId() != 8 {
		t.Fatalf("user message lookup %+v", fake.userIn)
	}
	if fake.peerIn.GetUserId() != 9 || fake.peerIn.GetPeerUserId() != tellomiai.AccountID || fake.peerIn.GetMsgId() != 3 {
		t.Fatalf("box lookup %+v", fake.peerIn)
	}
}

func TestMentionsFromInputName(t *testing.T) {
	ents, has := mentionsFrom([]*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{Offset: 0, Length: 1}).To_MessageEntity(),
		mtproto.MakeTLInputMessageEntityMentionName(&mtproto.MessageEntity{
			Offset: 2,
			Length: 8,
			UserId_INPUTUSER: mtproto.MakeTLInputUser(&mtproto.InputUser{
				UserId: tellomiai.AccountID,
			}).To_InputUser(),
		}).To_MessageEntity(),
	})
	if !has || len(ents) != 1 || ents[0].Kind != tellomiai.MentionName || ents[0].UserID != tellomiai.AccountID {
		t.Fatalf("%+v has=%v", ents, has)
	}
	q, mentioned := tellomiai.StripMention("x Tellomi你好", ents, false)
	if !mentioned || q == "" {
		t.Fatalf("stripped %q mentioned=%v", q, mentioned)
	}
}

func TestSupergroupReplySkipsHistoryAndNonMembers(t *testing.T) {
	chSvc := channel.New(channel.NewMemoryStore())
	prev := channel.Shared()
	channel.Use(chSvc)
	t.Cleanup(func() { channel.Use(prev) })
	ctx := context.Background()
	created, err := chSvc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  ch.Id,
		AccessHash: ch.AccessHash_FLAGINT64.GetValue(),
	}).To_InputChannel()
	if _, err = chSvc.Invite(ctx, 1, 0, in, []int64{8, 9, tellomiai.AccountID}, nil); err != nil {
		t.Fatal(err)
	}
	secretID := sendChannel(t, chSvc, ctx, 8, ch.Id, "秘密历史不该出现")
	priorID := sendChannel(t, chSvc, ctx, tellomiai.AccountID, ch.Id, "上一句答案")
	if secretID == 0 || priorID == 0 || secretID == priorID {
		t.Fatalf("ids secret=%d prior=%d", secretID, priorID)
	}

	ai, sc := hookAI(t)
	sent := &hookGroup{}
	ai.Group = sent
	tellomiai.Use(ai)
	t.Cleanup(func() { tellomiai.Use(nil) })
	c := &MessagesCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}}
	ents := []*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityMention(&mtproto.MessageEntity{Offset: 0, Length: 8}).To_MessageEntity(),
	}
	c.answerTellomiGroup(ctx, 9, mtproto.PEER_CHANNEL, ch.Id, "@tellomi 你好", ents, 11, priorID, 42)
	if sent.n != 1 || sent.replyTo != 42 || sent.peerID != ch.Id || sent.text != "你好" || sent.ai != true {
		t.Fatalf("sent %+v", sent)
	}
	recs := ai.Ledger.Records()
	if len(recs) != 1 || recs[0].Prompt != "上一句答案\n\n你好" {
		t.Fatalf("prompt %#v", recs)
	}
	if strings.Contains(recs[0].Prompt, "秘密历史不该出现") {
		t.Fatal("group history was sent to the model")
	}
	if len(sc.calls) != 1 {
		t.Fatalf("model calls %d", len(sc.calls))
	}

	c.answerTellomiGroup(ctx, 9, mtproto.PEER_CHANNEL, ch.Id, "不对", nil, 16, priorID, 45)
	if sent.n != 2 || len(sc.calls) != 2 || ai.Ledger.Records()[1].Prompt != "上一句答案\n\n不对" {
		t.Fatalf("reply to 777001 sent=%d calls=%d recs=%#v", sent.n, len(sc.calls), ai.Ledger.Records())
	}
	bare := []*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityMention(&mtproto.MessageEntity{Offset: 0, Length: 8}).To_MessageEntity(),
	}
	c.answerTellomiGroup(ctx, 9, mtproto.PEER_CHANNEL, ch.Id, "@tellomi", bare, 15, 0, 44)
	if sent.n != 2 || len(sc.calls) != 2 {
		t.Fatal("a bare mention with nothing left to ask called the model")
	}
	c.answerTellomiGroup(ctx, 9, mtproto.PEER_CHANNEL, ch.Id, "大家好", nil, 12, 0, 43)
	if sent.n != 2 || len(sc.calls) != 2 {
		t.Fatalf("unrelated message was answered sent=%d calls=%d", sent.n, len(sc.calls))
	}

	broadcast, err := chSvc.Create(ctx, channel.CreateRequest{CreatorID: tellomiai.AccountID, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	c.answerTellomiGroup(ctx, 9, mtproto.PEER_CHANNEL, broadcast.Chats[0].Id, "@tellomi 你好", ents, 13, 0, 1)
	if sent.n != 2 || len(sc.calls) != 2 {
		t.Fatal("a broadcast channel was answered")
	}

	closed, err := chSvc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "Closed", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	c.answerTellomiGroup(ctx, 9, mtproto.PEER_CHANNEL, closed.Chats[0].Id, "@tellomi 你好", ents, 14, 0, 1)
	if sent.n != 2 || len(sc.calls) != 2 {
		t.Fatal("answered a supergroup 777001 has not joined")
	}
}

type hookCalls struct {
	calls []string
	trace *callTrace
}

func (h *hookCalls) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.calls = append(h.calls, r.URL.Path)
	if h.trace != nil {
		h.trace.add("model")
	}
	_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"你好"}}],"usage":{"total_tokens":3}}`))
}

func hookAI(t *testing.T) (*tellomiai.Service, *hookCalls) {
	t.Helper()
	sc := &hookCalls{}
	srv := httptest.NewServer(sc)
	t.Cleanup(srv.Close)
	client := tellomiai.NewClient(srv.URL, "secret-key", "qwen-plus", "qwen-turbo", "deepseek-v3", time.Second)
	client.BaseURL = srv.URL
	return tellomiai.Ready(client, nil), sc
}

func sendChannel(t *testing.T, svc *channel.Service, ctx context.Context, from, channelID int64, text string) int32 {
	t.Helper()
	up, err := svc.Send(ctx, from, channelID, 0, []channel.Outgoing{{
		RandomID: time.Now().UnixNano(),
		Message:  mtproto.MakeTLMessage(&mtproto.Message{Message: text}).To_Message(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	return sentMessageID(up)
}

type hookGroup struct {
	n       int
	peerID  int64
	replyTo int32
	text    string
	ai      bool
	trace   *callTrace
}

func (m *hookGroup) SendGroup(_ context.Context, _ int32, peerID int64, replyTo int32, text string, ai bool) error {
	m.n++
	m.peerID = peerID
	m.replyTo = replyTo
	m.text = text
	m.ai = ai
	if m.trace != nil {
		m.trace.add("send")
	}
	return nil
}

type fakeMsgs struct {
	messageclient.MessageClient
	userCalls    int
	peerCalls    int
	historyCalls int
	userIn       *message.TLMessageGetUserMessage
	peerIn       *message.TLMessageGetPeerUserMessage
	box          *mtproto.MessageBox
	peerBox      *mtproto.MessageBox
	peerFail     int // >0: that many misses, then the box; <0: always miss
	peerLeft     int
	trace        *callTrace
}

func (f *fakeMsgs) MessageGetUserMessage(_ context.Context, in *message.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	f.userCalls++
	f.userIn = in
	return f.box, nil
}

func (f *fakeMsgs) MessageGetPeerUserMessage(_ context.Context, in *message.TLMessageGetPeerUserMessage) (*mtproto.MessageBox, error) {
	f.peerCalls++
	f.peerIn = in
	if f.peerFail < 0 || f.peerLeft < f.peerFail {
		if f.peerFail > 0 {
			f.peerLeft++
		}
		if f.trace != nil {
			f.trace.add("miss")
		}
		return nil, mtproto.ErrMsgIdInvalid
	}
	if f.trace != nil {
		f.trace.add("hit")
	}
	if f.peerBox != nil {
		return f.peerBox, nil
	}
	return f.box, nil
}

func (f *fakeMsgs) MessageGetHistoryMessages(_ context.Context, _ *message.TLMessageGetHistoryMessages) (*message.Vector_MessageBox, error) {
	f.historyCalls++
	return nil, nil
}
