package profile

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type memDB struct {
	mu   sync.Mutex
	rows map[string][]byte
}

func newMemDB() *memDB {
	return &memDB{rows: map[string][]byte{}}
}

func (m *memDB) Load(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, ok := m.rows[key]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), raw...), true, nil
}

func (m *memDB) Save(_ context.Context, key string, raw []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[key] = append([]byte(nil), raw...)
	return nil
}

func (m *memDB) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, key)
	return nil
}

func TestProfileSurvivesRestart(t *testing.T) {
	db := newMemDB()
	first := New()
	first.db = db
	hours := mtproto.MakeTLBusinessWorkHours(&mtproto.BusinessWorkHours{
		TimezoneId: "Asia/Shanghai",
		WeeklyOpen: []*mtproto.BusinessWeeklyOpen{
			mtproto.MakeTLBusinessWeeklyOpen(&mtproto.BusinessWeeklyOpen{StartMinute: 9 * 60, EndMinute: 18 * 60}).To_BusinessWeeklyOpen(),
		},
	}).To_BusinessWorkHours()
	if err := first.SetWorkHours(7, hours); err != nil {
		t.Fatal(err)
	}
	if err := first.SetLocation(7, mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 31.2, Long: 121.5}).To_InputGeoPoint(), wrapperspb.String("Nanjing Rd")); err != nil {
		t.Fatal(err)
	}
	if err := first.SetIntro(7, mtproto.MakeTLInputBusinessIntro(&mtproto.InputBusinessIntro{Title: "Shop", Description: "Open"}).To_InputBusinessIntro()); err != nil {
		t.Fatal(err)
	}
	greet := mtproto.MakeTLInputBusinessGreetingMessage(&mtproto.InputBusinessGreetingMessage{
		ShortcutId:     4,
		NoActivityDays: 7,
		Recipients:     mtproto.MakeTLInputBusinessRecipients(&mtproto.InputBusinessRecipients{NonContacts: true}).To_InputBusinessRecipients(),
	}).To_InputBusinessGreetingMessage()
	if err := first.SetGreeting(7, greet); err != nil {
		t.Fatal(err)
	}
	away := mtproto.MakeTLInputBusinessAwayMessage(&mtproto.InputBusinessAwayMessage{
		ShortcutId: 5,
		Schedule:   mtproto.MakeTLBusinessAwayMessageScheduleAlways(nil).To_BusinessAwayMessageSchedule(),
		Recipients: mtproto.MakeTLInputBusinessRecipients(&mtproto.InputBusinessRecipients{NonContacts: true}).To_InputBusinessRecipients(),
	}).To_InputBusinessAwayMessage()
	if err := first.SetAway(7, away); err != nil {
		t.Fatal(err)
	}
	link, err := first.CreateLink(7, mtproto.MakeTLInputBusinessChatLink(&mtproto.InputBusinessChatLink{
		Message: "ask us",
		Title:   wrapperspb.String("Desk"),
	}).To_InputBusinessChatLink())
	if err != nil {
		t.Fatal(err)
	}
	slug := link.GetLink()
	if i := strings.LastIndex(slug, "/"); i >= 0 {
		slug = slug[i+1:]
	}

	second := New()
	second.db = db
	card := second.Card(7)
	if card == nil || card.WorkHours.GetTimezoneId() != "Asia/Shanghai" || card.Location.GetAddress() != "Nanjing Rd" || card.Intro.GetTitle() != "Shop" {
		t.Fatalf("card %#v", card)
	}
	if card.Greeting.GetShortcutId() != 4 || card.Greeting.GetNoActivityDays() != 7 || card.Away.GetShortcutId() != 5 {
		t.Fatalf("messages %#v %#v", card.Greeting, card.Away)
	}
	resolved, err := second.Resolve(slug)
	if err != nil || resolved.GetMessage() != "ask us" || resolved.GetPeer().GetUserId() != 7 {
		t.Fatalf("resolve %#v %v", resolved, err)
	}
	if second.Links(7).GetLinks()[0].GetViews() != 1 {
		t.Fatal("view was not stored")
	}

	third := New()
	third.db = db
	if err := third.SetWorkHours(7, nil); err != nil {
		t.Fatal(err)
	}
	fourth := New()
	fourth.db = db
	if got := fourth.Card(7); got == nil || got.WorkHours != nil || got.Intro.GetTitle() != "Shop" {
		t.Fatalf("cleared hours %#v", got)
	}
}
