package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"
	"github.com/zeromicro/go-zero/core/logx"
)

// basicChatID is the production chat where the inbox race was observed.
const basicChatID int64 = 120038

func TestBasicGroupInboxRace(t *testing.T) {
	if tellomiBoxEvery != 200*time.Millisecond || tellomiBoxFor != 3*time.Second {
		t.Fatalf("box retry every=%s for=%s", tellomiBoxEvery, tellomiBoxFor)
	}
	prev := logx.Reset()
	sink := &logSink{}
	logx.SetWriter(sink)
	t.Cleanup(func() {
		logx.Reset()
		if prev != nil {
			logx.SetWriter(prev)
		}
	})

	ai, sc := hookAI(t)
	sent := &hookGroup{}
	ai.Group = sent
	tellomiai.Use(ai)
	t.Cleanup(func() { tellomiai.Use(nil) })

	mention := []*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityMention(&mtproto.MessageEntity{Offset: 0, Length: 8}).To_MessageEntity(),
	}
	mapped := &mtproto.MessageBox{MessageId: 55}

	t.Run("context miss keeps reply to 777001", func(t *testing.T) {
		sink.reset()
		fake := &fakeMsgs{
			box: &mtproto.MessageBox{
				SenderUserId: tellomiai.AccountID,
				Message:      mtproto.MakeTLMessage(&mtproto.Message{Message: "上一句"}).To_Message(),
			},
			peerFail: 1,
		}
		c := &MessagesCore{ctx: context.Background(), svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: fake}}}
		replied, replyToAI, replyID, ok := c.groupContext(context.Background(), 9, mtproto.PEER_CHAT, basicChatID, 8, 3)
		if !ok || !replyToAI || replied != "上一句" || replyID != 0 {
			t.Fatalf("replied %q ai=%v id=%d ok=%v", replied, replyToAI, replyID, ok)
		}
		if fake.userCalls != 1 || fake.peerCalls != 1 || fake.historyCalls != 0 {
			t.Fatalf("reads user=%d peer=%d history=%d", fake.userCalls, fake.peerCalls, fake.historyCalls)
		}
		if fake.userIn.GetUserId() != 9 || fake.userIn.GetId() != 8 {
			t.Fatalf("user lookup %+v", fake.userIn)
		}
		if fake.peerIn.GetUserId() != 9 || fake.peerIn.GetPeerUserId() != tellomiai.AccountID || fake.peerIn.GetMsgId() != 3 {
			t.Fatalf("box lookup %+v", fake.peerIn)
		}
		assertPendingLog(t, sink, basicChatID, "上一句")
	})

	t.Run("later lookup fills reply_to", func(t *testing.T) {
		sink.reset()
		tr := &callTrace{}
		fake := &fakeMsgs{peerBox: mapped, peerFail: 1, trace: tr}
		sc.trace = tr
		sent.trace = tr
		sent.n = 0
		before := len(ai.Ledger.Records())
		calls := len(sc.calls)
		c := basicCore(fake, tellomiai.AccountID, 9)
		start := time.Now()
		c.answerTellomiGroup(context.Background(), 9, mtproto.PEER_CHAT, basicChatID, "@tellomi 你好", mention, 101, 0, 4944)
		if time.Since(start) > time.Second {
			t.Fatal("a hit on the next lookup waited out the full window")
		}
		if sent.n != 1 || sent.peerID != basicChatID || sent.replyTo != 55 || sent.text != "你好" || !sent.ai {
			t.Fatalf("sent %+v", sent)
		}
		if len(sc.calls) != calls+1 || fake.peerCalls != 2 || fake.historyCalls != 0 || fake.userCalls != 0 {
			t.Fatalf("calls model=%d peer=%d user=%d history=%d", len(sc.calls)-calls, fake.peerCalls, fake.userCalls, fake.historyCalls)
		}
		if fake.peerIn.GetUserId() != 9 || fake.peerIn.GetPeerUserId() != tellomiai.AccountID || fake.peerIn.GetMsgId() != 4944 {
			t.Fatalf("box lookup %+v", fake.peerIn)
		}
		recs := ai.Ledger.Records()
		if len(recs) != before+1 || recs[before].Prompt != "你好" {
			t.Fatalf("prompt %#v", recs)
		}
		if !hasOrder(tr.snapshot(), "miss", "model", "hit", "send") {
			t.Fatalf("order %v", tr.snapshot())
		}
		assertPendingLog(t, sink, basicChatID, "你好")
	})

	t.Run("reply to 777001 while the box id is delayed", func(t *testing.T) {
		sink.reset()
		tr := &callTrace{}
		fake := &fakeMsgs{
			box: &mtproto.MessageBox{
				SenderUserId: tellomiai.AccountID,
				Message:      mtproto.MakeTLMessage(&mtproto.Message{Message: "上一句"}).To_Message(),
			},
			peerBox:  mapped,
			peerFail: 1,
			trace:    tr,
		}
		sc.trace = tr
		sent.trace = tr
		sent.n = 0
		before := len(ai.Ledger.Records())
		calls := len(sc.calls)
		c := basicCore(fake, tellomiai.AccountID, 11)
		c.answerTellomiGroup(context.Background(), 11, mtproto.PEER_CHAT, basicChatID, "不对", nil, 103, 8, 4945)
		if sent.n != 1 || sent.replyTo != 55 || sent.text != "你好" {
			t.Fatalf("sent %+v", sent)
		}
		if len(sc.calls) != calls+1 || fake.userCalls != 1 || fake.peerCalls != 2 || fake.historyCalls != 0 {
			t.Fatalf("calls model=%d user=%d peer=%d history=%d", len(sc.calls)-calls, fake.userCalls, fake.peerCalls, fake.historyCalls)
		}
		recs := ai.Ledger.Records()
		if len(recs) != before+1 || recs[before].Prompt != "上一句\n\n不对" {
			t.Fatalf("prompt %#v", recs)
		}
		if !hasOrder(tr.snapshot(), "miss", "model", "hit", "send") {
			t.Fatalf("order %v", tr.snapshot())
		}
		assertPendingLog(t, sink, basicChatID, "上一句", "不对")
	})

	t.Run("gives up and still sends", func(t *testing.T) {
		prevEvery, prevFor := tellomiBoxEvery, tellomiBoxFor
		tellomiBoxEvery = 10 * time.Millisecond
		tellomiBoxFor = 45 * time.Millisecond
		t.Cleanup(func() {
			tellomiBoxEvery = prevEvery
			tellomiBoxFor = prevFor
		})
		sink.reset()
		tr := &callTrace{}
		fake := &fakeMsgs{peerFail: -1, trace: tr}
		sc.trace = tr
		sent.trace = tr
		sent.n = 0
		calls := len(sc.calls)
		c := basicCore(fake, tellomiai.AccountID, 10)
		c.answerTellomiGroup(context.Background(), 10, mtproto.PEER_CHAT, basicChatID, "@tellomi 还在吗", mention, 102, 0, 4946)
		if sent.n != 1 || sent.peerID != basicChatID || sent.replyTo != 0 || sent.text != "你好" || !sent.ai {
			t.Fatalf("sent %+v", sent)
		}
		if len(sc.calls) != calls+1 || fake.peerCalls < 3 || fake.historyCalls != 0 {
			t.Fatalf("calls model=%d peer=%d history=%d", len(sc.calls)-calls, fake.peerCalls, fake.historyCalls)
		}
		if !hasOrder(tr.snapshot(), "miss", "model", "miss", "send") {
			t.Fatalf("order %v", tr.snapshot())
		}
		assertPendingLog(t, sink, basicChatID, "还在吗", "你好")
	})

	t.Run("mapped id does not retry", func(t *testing.T) {
		sink.reset()
		tr := &callTrace{}
		fake := &fakeMsgs{box: mapped, trace: tr}
		sc.trace = tr
		sent.trace = tr
		sent.n = 0
		calls := len(sc.calls)
		c := basicCore(fake, tellomiai.AccountID, 12)
		c.answerTellomiGroup(context.Background(), 12, mtproto.PEER_CHAT, basicChatID, "@tellomi 你好", mention, 104, 0, 4947)
		if sent.n != 1 || sent.replyTo != 55 || fake.peerCalls != 1 || len(sc.calls) != calls+1 {
			t.Fatalf("sent %+v peer=%d model=%d", sent, fake.peerCalls, len(sc.calls)-calls)
		}
		if !hasOrder(tr.snapshot(), "hit", "model", "send") {
			t.Fatalf("order %v", tr.snapshot())
		}
		for _, ev := range tr.snapshot() {
			if ev == "miss" {
				t.Fatal("a visible box id was treated as pending")
			}
		}
		if lines := pendingLines(sink.errors()); len(lines) != 0 {
			t.Fatalf("pending log %q", lines)
		}
	})

	t.Run("non member does not call the model", func(t *testing.T) {
		sink.reset()
		tr := &callTrace{}
		fake := &fakeMsgs{peerFail: 1, trace: tr}
		sc.trace = tr
		calls := len(sc.calls)
		sent.n = 0
		c := basicCore(fake, 13)
		start := time.Now()
		c.answerTellomiGroup(context.Background(), 13, mtproto.PEER_CHAT, basicChatID, "@tellomi 你好", mention, 105, 0, 4948)
		if time.Since(start) > time.Second {
			t.Fatal("membership failure waited on the inbox")
		}
		if sent.n != 0 || len(sc.calls) != calls || fake.peerCalls != 1 || fake.historyCalls != 0 {
			t.Fatalf("sent %d model=%d peer=%d history=%d", sent.n, len(sc.calls)-calls, fake.peerCalls, fake.historyCalls)
		}
		assertPendingLog(t, sink, basicChatID, "你好")
	})

	t.Run("supergroup does not wait", func(t *testing.T) {
		sink.reset()
		chID := testMegagroup(t, 14)
		tr := &callTrace{}
		fake := &fakeMsgs{peerBox: mapped, peerFail: -1, trace: tr}
		sc.trace = tr
		sent.trace = tr
		sent.n = 0
		calls := len(sc.calls)
		c := &MessagesCore{ctx: context.Background(), svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: fake}}}
		start := time.Now()
		c.answerTellomiGroup(context.Background(), 14, mtproto.PEER_CHANNEL, chID, "@tellomi 你好", mention, 106, 0, 42)
		if time.Since(start) > time.Second {
			t.Fatal("supergroup waited on a basic-group inbox row")
		}
		if sent.n != 1 || sent.peerID != chID || sent.replyTo != 42 || sent.text != "你好" || !sent.ai {
			t.Fatalf("sent %+v", sent)
		}
		if fake.peerCalls != 0 || fake.userCalls != 0 || fake.historyCalls != 0 || len(sc.calls) != calls+1 {
			t.Fatalf("reads peer=%d user=%d history=%d model=%d", fake.peerCalls, fake.userCalls, fake.historyCalls, len(sc.calls)-calls)
		}
		if !hasOrder(tr.snapshot(), "model", "send") {
			t.Fatalf("order %v", tr.snapshot())
		}
		if lines := pendingLines(sink.errors()); len(lines) != 0 {
			t.Fatalf("pending log %q", lines)
		}
	})
}

func basicCore(fake *fakeMsgs, members ...int64) *MessagesCore {
	return &MessagesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: fake,
			ChatClient:    chat_client.NewDirectChatClient(memberChat{users: members}),
		}},
	}
}

type memberChat struct {
	chat_client.ChatClient
	users []int64
}

func (m memberChat) ChatGetMutableChat(context.Context, *chatpb.TLChatGetMutableChat) (*mtproto.MutableChat, error) {
	ps := make([]*mtproto.ImmutableChatParticipant, 0, len(m.users))
	for _, id := range m.users {
		ps = append(ps, &mtproto.ImmutableChatParticipant{UserId: id, State: mtproto.ChatMemberStateNormal})
	}
	return &mtproto.MutableChat{ChatParticipants: ps}, nil
}

func testMegagroup(t *testing.T, user int64) int64 {
	t.Helper()
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
	if _, err = chSvc.Invite(ctx, 1, 0, in, []int64{user, tellomiai.AccountID}, nil); err != nil {
		t.Fatal(err)
	}
	return ch.Id
}

type callTrace struct {
	mu sync.Mutex
	ev []string
}

func (t *callTrace) add(s string) {
	t.mu.Lock()
	t.ev = append(t.ev, s)
	t.mu.Unlock()
}

func (t *callTrace) snapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.ev...)
}

func hasOrder(ev []string, want ...string) bool {
	i := 0
	for _, e := range ev {
		if i < len(want) && e == want[i] {
			i++
		}
	}
	return i == len(want)
}

type logSink struct {
	mu   sync.Mutex
	errs []string
}

func (s *logSink) Alert(any)                   {}
func (s *logSink) Close() error                { return nil }
func (s *logSink) Debug(any, ...logx.LogField) {}
func (s *logSink) Info(any, ...logx.LogField)  {}
func (s *logSink) Severe(any)                  {}
func (s *logSink) Slow(any, ...logx.LogField)  {}
func (s *logSink) Stack(any)                   {}
func (s *logSink) Stat(any, ...logx.LogField)  {}
func (s *logSink) Error(v any, _ ...logx.LogField) {
	s.mu.Lock()
	s.errs = append(s.errs, fmt.Sprint(v))
	s.mu.Unlock()
}

func (s *logSink) errors() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.errs...)
}

func (s *logSink) reset() {
	s.mu.Lock()
	s.errs = nil
	s.mu.Unlock()
}

func pendingLines(lines []string) []string {
	var out []string
	for _, line := range lines {
		if strings.Contains(line, "tellomi ai: group box id pending") {
			out = append(out, line)
		}
	}
	return out
}

func assertPendingLog(t *testing.T, sink *logSink, chatID int64, banned ...string) {
	t.Helper()
	lines := pendingLines(sink.errors())
	want := fmt.Sprintf("tellomi ai: group box id pending chat=%d", chatID)
	if len(lines) != 1 || lines[0] != want {
		t.Fatalf("pending log %q", lines)
	}
	for _, word := range banned {
		if strings.Contains(lines[0], word) {
			t.Fatalf("log contains message text %q: %s", word, lines[0])
		}
	}
}
