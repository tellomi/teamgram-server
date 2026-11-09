package tellomiai

import (
	"sync"
	"time"
)

const (
	defaultDaily  = 100
	defaultMinute = 5
	// ChatPerMinute is the extra cap for one basic group or supergroup.
	// Private chats do not use it. It is shared by everyone in that chat.
	ChatPerMinute = 20
	smallAt       = 0.8
)

// Ledger counts daily and per-minute uses and the monthly spend.
// It is process-local: a restart clears the counters. Replies are also
// appended here for tests; production copies them to MySQL when that
// recorder is installed.
type Ledger struct {
	mu      sync.Mutex
	daily   map[string]int
	minute  map[string]int
	chatMin map[string]int
	spent   map[string]float64
	done    map[string]Result
	busy    map[string]struct{}
	records []Record
	Daily   int
	Minute  int
	// ChatMinute is the per-chat cap. Zero means no chat cap.
	ChatMinute    int
	MonthBudget   float64
	PricePerToken float64
}

// Record is one kept reply. Prompt and Reply are the audit trail.
type Record struct {
	UserID  int64
	EventID string
	Prompt  string
	Reply   string
	Model   string
	Refused bool
	Limited string
	At      time.Time
}

// NewLedger returns the spec limits: 100 a day, 5 a minute, 20 a minute per chat.
func NewLedger() *Ledger {
	return &Ledger{
		daily:      map[string]int{},
		minute:     map[string]int{},
		chatMin:    map[string]int{},
		spent:      map[string]float64{},
		done:       map[string]Result{},
		busy:       map[string]struct{}{},
		Daily:      defaultDaily,
		Minute:     defaultMinute,
		ChatMinute: ChatPerMinute,
	}
}

// Records returns a copy of kept replies.
func (l *Ledger) Records() []Record {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Record, len(l.records))
	copy(out, l.records)
	return out
}

type gate struct {
	Stored  bool
	Busy    bool
	Result  Result
	Limited string
	Small   bool
}

// Begin reserves one private use. A repeated event id returns the stored
// result and does not reserve again. Limited is daily, minute, or budget.
func (l *Ledger) Begin(user int64, event string, now time.Time) gate {
	return l.BeginChat(user, event, "", now)
}

// BeginChat is Begin plus the per-chat minute cap. An empty chat skips that cap.
// Limited is daily, minute, budget, or chat. A chat rejection does not consume
// the user's own counters.
func (l *Ledger) BeginChat(user int64, event, chat string, now time.Time) gate {
	l.mu.Lock()
	defer l.mu.Unlock()
	if event != "" {
		if res, ok := l.done[event]; ok {
			return gate{Stored: true, Result: res}
		}
		if _, ok := l.busy[event]; ok {
			return gate{Busy: true}
		}
		l.busy[event] = struct{}{}
	}
	if chat != "" && l.ChatMinute > 0 {
		if l.chatMin == nil {
			l.chatMin = map[string]int{}
		}
		ck := chatMinuteKey(chat, now)
		if l.chatMin[ck] >= l.ChatMinute {
			return gate{Limited: "chat"}
		}
		l.chatMin[ck]++
	}
	if l.MonthBudget > 0 && l.spent[monthKey(user, now)] >= l.MonthBudget {
		return gate{Limited: "budget"}
	}
	mk := minuteKey(user, now)
	l.minute[mk]++
	if l.Minute > 0 && l.minute[mk] > l.Minute {
		return gate{Limited: "minute"}
	}
	dk := dayKey(user, now)
	l.daily[dk]++
	if l.Daily > 0 && l.daily[dk] > l.Daily {
		return gate{Limited: "daily"}
	}
	small := false
	if l.MonthBudget > 0 && l.spent[monthKey(user, now)] >= l.MonthBudget*smallAt {
		small = true
	}
	return gate{Small: small}
}

// Finish stores the result and adds token spend.
func (l *Ledger) Finish(user int64, event string, res Result, tokens int, now time.Time, prompt string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if tokens > 0 && l.PricePerToken > 0 {
		l.spent[monthKey(user, now)] += float64(tokens) * l.PricePerToken
	}
	if event != "" {
		l.done[event] = res
		delete(l.busy, event)
	}
	l.records = append(l.records, Record{
		UserID: user, EventID: event, Prompt: prompt, Reply: res.Text,
		Model: res.Model, Refused: res.Refused, Limited: res.Limited, At: now,
	})
}

// Abort drops an in-flight event id so the caller can retry it.
// The daily and minute counters stay incremented.
func (l *Ledger) Abort(event string) {
	if event == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.busy, event)
}

func dayKey(user int64, now time.Time) string {
	return formatKey(user, now, "20060102")
}

func minuteKey(user int64, now time.Time) string {
	return formatKey(user, now, "200601021504")
}

func monthKey(user int64, now time.Time) string {
	return formatKey(user, now, "200601")
}

func chatMinuteKey(chat string, now time.Time) string {
	return "chat:" + chat + ":" + now.UTC().Format("200601021504")
}

func formatKey(user int64, now time.Time, layout string) string {
	return itoa(user) + ":" + now.UTC().Format(layout)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
