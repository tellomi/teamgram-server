package tellomiai

import (
	"context"
	"errors"
	"testing"
	"time"
)

// forbidModel fails the test when a fixed fallback calls the model.
type forbidModel struct{ t *testing.T }

func (m forbidModel) Complete(context.Context, string, string) (string, int, error) {
	m.t.Helper()
	m.t.Fatal("model client was called")
	return "", 0, errors.New("model client was called")
}

type clock struct{ at time.Time }

func (c *clock) Now() time.Time { return c.at }

func gated(t *testing.T, allow *Allow) (*Service, *memSend, *memGroup, *clock) {
	t.Helper()
	sent := &memSend{}
	group := &memGroup{}
	clk := &clock{at: time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC)}
	svc := &Service{
		enabled: true,
		Models:  forbidModel{t: t},
		Ledger:  NewLedger(),
		Sender:  sent,
		Group:   group,
		Allow:   allow,
		Now:     clk.Now,
	}
	Use(svc)
	t.Cleanup(func() { Use(nil) })
	return svc, sent, group, clk
}

func burn(l *Ledger, user int64, n int, start time.Time) {
	for i := 0; i < n; i++ {
		l.Begin(user, "", start.Add(time.Duration(i)*time.Minute))
	}
}

func burnMinute(l *Ledger, user int64, n int, at time.Time) {
	for i := 0; i < n; i++ {
		l.Begin(user, "", at)
	}
}

func TestFallbacksDoNotCallTheModel(t *testing.T) {
	ctx := context.Background()
	allow := ParseAllow("erel", "9", &scriptDir{ids: map[string]int64{"erel": 9}})

	t.Run("not-allowed", func(t *testing.T) {
		svc, _, _, _ := gated(t, allow)
		res, err := svc.Reply(ctx, 8, "你好", "")
		if err != nil || res.Text != NotAllowedMessage || res.AIGenerated || res.Limited != KindAllow {
			t.Fatalf("%v %#v", err, res)
		}
	})

	t.Run("allowed", func(t *testing.T) {
		svc, _, _, _ := gated(t, allow)
		svc.Models = okModel{}
		res, err := svc.Reply(ctx, 9, "你好", "")
		if err != nil || res.Text != "好" || !res.AIGenerated {
			t.Fatalf("%v %#v", err, res)
		}
	})

	t.Run("daily", func(t *testing.T) {
		svc, _, _, clk := gated(t, nil)
		burn(svc.Ledger, 9, 100, clk.at)
		res, err := svc.Reply(ctx, 9, "你好", "")
		if err != nil || res.Text != DailyMessage || res.Limited != KindDaily || res.AIGenerated {
			t.Fatalf("%v %#v", err, res)
		}
	})

	t.Run("minute", func(t *testing.T) {
		svc, _, _, clk := gated(t, nil)
		burnMinute(svc.Ledger, 9, 5, clk.at)
		res, err := svc.Reply(ctx, 9, "你好", "")
		if err != nil || res.Text != MinuteMessage || res.Limited != KindMinute {
			t.Fatalf("%v %#v", err, res)
		}
	})

	t.Run("chat", func(t *testing.T) {
		svc, _, _, clk := gated(t, nil)
		svc.Ledger.ChatMinute = 2
		svc.Ledger.BeginChat(9, "", "3:40", clk.at)
		svc.Ledger.BeginChat(10, "", "3:40", clk.at)
		res, err := svc.ReplyChat(ctx, 11, "你好", "", "3:40")
		if err != nil || res.Text != ChatMessage || res.Limited != KindChat {
			t.Fatalf("%v %#v", err, res)
		}
	})

	t.Run("budget", func(t *testing.T) {
		svc, _, _, clk := gated(t, nil)
		svc.Ledger.MonthBudget = 1
		svc.Ledger.PricePerToken = 1
		svc.Ledger.Finish(9, "", Result{}, 1, clk.at, "")
		res, err := svc.Reply(ctx, 9, "你好", "")
		if err != nil || res.Text != BudgetMessage || res.Limited != KindBudget || res.AIGenerated {
			t.Fatalf("%v %#v", err, res)
		}
	})

	t.Run("empty", func(t *testing.T) {
		svc, _, _, _ := gated(t, nil)
		res, err := svc.Reply(ctx, 9, "   ", "")
		if err != nil || res.Text != TextOnlyMessage || res.Limited != KindInput {
			t.Fatalf("%v %#v", err, res)
		}
	})

	t.Run("media", func(t *testing.T) {
		_, _, group, _ := gated(t, nil)
		ObserveGroup(ctx, GroupAsk{From: 9, PeerType: 3, PeerID: 40, ReplyTo: 4, Text: "上一句", Random: 1, NoText: true})
		if group.n != 1 || group.text != TextOnlyMessage || group.ai || group.replyTo != 4 {
			t.Fatalf("%+v", group)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		sent := &memSend{}
		svc := &Service{enabled: false, Sender: sent, Models: forbidModel{t: t}}
		Use(svc)
		t.Cleanup(func() { Use(nil) })
		ObservePrivate(ctx, 9, "你好", 1)
		if sent.n != 1 || sent.text != DisabledMessage || sent.ai {
			t.Fatalf("sent n=%d text=%q ai=%v", sent.n, sent.text, sent.ai)
		}
	})

	t.Run("lookup-down", func(t *testing.T) {
		svc, _, _, _ := gated(t, ParseAllow("erel", "", &scriptDir{err: errors.New("down")}))
		res, err := svc.Reply(ctx, 9, "你好", "")
		if err != nil || res.Text != BusyMessage || res.Limited != KindBusy {
			t.Fatalf("%v %#v", err, res)
		}
	})

	t.Run("no-client", func(t *testing.T) {
		sent := &memSend{}
		svc := &Service{enabled: true, Ledger: NewLedger(), Sender: sent}
		Use(svc)
		t.Cleanup(func() { Use(nil) })
		ObservePrivate(ctx, 9, "你好", 3)
		if sent.n != 1 || sent.text != BusyMessage || sent.ai {
			t.Fatalf("sent n=%d text=%q", sent.n, sent.text)
		}
	})
}

type okModel struct{}

func (okModel) Complete(context.Context, string, string) (string, int, error) {
	return "好", 3, nil
}

type failModel struct {
	n   int
	err error
}

func (m *failModel) Complete(context.Context, string, string) (string, int, error) {
	m.n++
	return "", 0, m.err
}

func TestBlockedAndBusyUseFixedText(t *testing.T) {
	ctx := context.Background()
	blocked := &failModel{err: ErrSafety}
	svc, sent, _, _ := gated(t, nil)
	svc.Models = blocked
	res, err := svc.Reply(ctx, 9, "不行", "e-block")
	if err != nil || !res.Refused || res.Text != Refusal || blocked.n != 1 {
		t.Fatalf("err=%v n=%d %#v", err, blocked.n, res)
	}

	busy := &failModel{err: ErrTimeout}
	svc.Models = busy
	svc.Ledger = NewLedger()
	ObservePrivate(ctx, 9, "你好", 8)
	if sent.n != 1 || sent.text != BusyMessage || sent.ai {
		t.Fatalf("sent n=%d text=%q ai=%v calls=%d", sent.n, sent.text, sent.ai, busy.n)
	}
	// Primary plus the fallback model. The sentence itself is not a model call.
	if busy.n != 2 {
		t.Fatalf("upstream calls %d", busy.n)
	}
}

func TestNoticeThrottle(t *testing.T) {
	ctx := context.Background()
	deny := ParseAllow("erel", "", &scriptDir{ids: map[string]int64{"erel": 1}})

	t.Run("private-not-allowed", func(t *testing.T) {
		_, sent, _, clk := gated(t, deny)
		ObservePrivate(ctx, 8, "你好", 1)
		ObservePrivate(ctx, 8, "再问", 2)
		if sent.n != 1 || sent.text != NotAllowedMessage {
			t.Fatalf("n=%d text=%q", sent.n, sent.text)
		}
		clk.at = clk.at.Add(time.Minute)
		ObservePrivate(ctx, 8, "又问", 3)
		if sent.n != 2 {
			t.Fatalf("after a minute n=%d", sent.n)
		}
	})

	t.Run("group-not-allowed", func(t *testing.T) {
		_, _, group, clk := gated(t, deny)
		ask := func(random int64, peer int64) {
			ObserveGroup(ctx, GroupAsk{From: 8, PeerType: 3, PeerID: peer, ReplyTo: 2, Text: "问", Random: random})
		}
		ask(1, 40)
		ask(2, 40)
		if group.n != 1 || group.text != NotAllowedMessage || group.ai {
			t.Fatalf("first %+v", group)
		}
		ask(3, 41)
		if group.n != 2 || group.peerID != 41 {
			t.Fatalf("other chat %+v", group)
		}
		clk.at = clk.at.Add(10 * time.Minute)
		ask(4, 40)
		if group.n != 3 || group.peerID != 40 {
			t.Fatalf("after 10 minutes %+v", group)
		}
	})

	t.Run("over-quota", func(t *testing.T) {
		svc, sent, _, clk := gated(t, nil)
		burn(svc.Ledger, 9, 100, clk.at)
		ObservePrivate(ctx, 9, "你好", 1)
		ObservePrivate(ctx, 9, "再问", 2)
		if sent.n != 1 || sent.text != DailyMessage {
			t.Fatalf("n=%d text=%q", sent.n, sent.text)
		}
		clk.at = clk.at.Add(time.Hour)
		ObservePrivate(ctx, 9, "又问", 3)
		if sent.n != 2 || sent.text != DailyMessage {
			t.Fatalf("after an hour n=%d text=%q", sent.n, sent.text)
		}
	})

	t.Run("minute", func(t *testing.T) {
		svc, sent, _, clk := gated(t, nil)
		burnMinute(svc.Ledger, 9, 5, clk.at)
		ObservePrivate(ctx, 9, "你好", 1)
		ObservePrivate(ctx, 9, "再问", 2)
		if sent.n != 1 || sent.text != MinuteMessage {
			t.Fatalf("n=%d text=%q", sent.n, sent.text)
		}
		clk.at = clk.at.Add(time.Minute)
		burnMinute(svc.Ledger, 9, 5, clk.at)
		ObservePrivate(ctx, 9, "又问", 3)
		if sent.n != 2 || sent.text != MinuteMessage {
			t.Fatalf("next minute n=%d text=%q", sent.n, sent.text)
		}
	})
}

func TestMessageEnvOverrides(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		env  string
		kind string
		def  string
	}{
		{EnvMsgNotAllowed, KindAllow, NotAllowedMessage},
		{EnvMsgDaily, KindDaily, DailyMessage},
		{EnvMsgMinute, KindMinute, MinuteMessage},
		{EnvMsgChat, KindChat, ChatMessage},
		{EnvMsgBusy, KindBusy, BusyMessage},
		{EnvMsgBlocked, KindBlocked, Refusal},
		{EnvMsgTextOnly, KindInput, TextOnlyMessage},
		{EnvMsgDisabled, KindDisabled, DisabledMessage},
		{EnvMsgBudget, KindBudget, BudgetMessage},
	}
	for _, tc := range cases {
		if Text(tc.kind) != tc.def {
			t.Fatalf("%s default %q", tc.kind, Text(tc.kind))
		}
	}
	t.Setenv(EnvMsgDaily, "自定义额度")
	if Text(KindDaily) != "自定义额度" {
		t.Fatalf("override %q", Text(KindDaily))
	}
	svc, _, _, clk := gated(t, nil)
	burn(svc.Ledger, 9, 100, clk.at)
	res, err := svc.Reply(ctx, 9, "你好", "")
	if err != nil || res.Text != "自定义额度" {
		t.Fatalf("%v %#v", err, res)
	}
}

type nameGate struct {
	id  int64
	err error
}

func (g nameGate) Verified(_ context.Context, userID int64) (bool, error) {
	if g.err != nil {
		return false, g.err
	}
	return userID == g.id, nil
}

func TestRealNameCheckerReplacesAllowList(t *testing.T) {
	SetRealNameChecker(nameGate{id: 9})
	t.Cleanup(func() { SetRealNameChecker(nil) })
	// The allowlist would refuse user 9. The checker allows only 9.
	allow := ParseAllow("", "1", nil)
	ctx := context.Background()

	svc, _, _, _ := gated(t, allow)
	svc.Models = okModel{}
	res, err := svc.Reply(ctx, 9, "你好", "")
	if err != nil || res.Text != "好" || !res.AIGenerated {
		t.Fatalf("verified %v %#v", err, res)
	}

	svc.Models = forbidModel{t: t}
	res, err = svc.Reply(ctx, 8, "你好", "")
	if err != nil || res.Text != NotAllowedMessage || res.Limited != KindAllow {
		t.Fatalf("unverified %v %#v", err, res)
	}

	SetRealNameChecker(nameGate{err: errors.New("down")})
	res, err = svc.Reply(ctx, 9, "你好", "")
	if err != nil || res.Text != BusyMessage || res.Limited != KindBusy {
		t.Fatalf("checker error %v %#v", err, res)
	}
}
