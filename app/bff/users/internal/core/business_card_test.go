package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	profile "github.com/teamgram/teamgram-server/app/bff/business/profile"
)

func TestBusinessCardHidesGreetingAndAwayFromOtherViewers(t *testing.T) {
	card := &profile.Card{
		WorkHours: mtproto.MakeTLBusinessWorkHours(&mtproto.BusinessWorkHours{TimezoneId: "Asia/Shanghai"}).To_BusinessWorkHours(),
		Location:  mtproto.MakeTLBusinessLocation(&mtproto.BusinessLocation{Address: "shop"}).To_BusinessLocation(),
		Intro:     mtproto.MakeTLBusinessIntro(&mtproto.BusinessIntro{Title: "hi"}).To_BusinessIntro(),
		Greeting: mtproto.MakeTLBusinessGreetingMessage(&mtproto.BusinessGreetingMessage{
			ShortcutId: 3,
			Recipients: mtproto.MakeTLBusinessRecipients(&mtproto.BusinessRecipients{
				Users: []int64{9, 10},
			}).To_BusinessRecipients(),
		}).To_BusinessGreetingMessage(),
		Away: mtproto.MakeTLBusinessAwayMessage(&mtproto.BusinessAwayMessage{
			ShortcutId: 4,
			Recipients: mtproto.MakeTLBusinessRecipients(&mtproto.BusinessRecipients{
				ExcludeSelected: true,
				Users:           []int64{8},
			}).To_BusinessRecipients(),
		}).To_BusinessAwayMessage(),
	}

	other := &mtproto.UserFull{}
	applyBusinessCard(other, 3, 7, card)
	if other.GetBusinessGreetingMessage() != nil || other.GetBusinessAwayMessage() != nil {
		t.Fatalf("greeting=%v away=%v", other.GetBusinessGreetingMessage(), other.GetBusinessAwayMessage())
	}
	if other.GetBusinessWorkHours().GetTimezoneId() != "Asia/Shanghai" || other.GetBusinessLocation().GetAddress() != "shop" || other.GetBusinessIntro().GetTitle() != "hi" {
		t.Fatalf("public fields %+v", other)
	}

	owner := &mtproto.UserFull{}
	applyBusinessCard(owner, 7, 7, card)
	if got := owner.GetBusinessGreetingMessage().GetRecipients().GetUsers(); len(got) != 2 || got[0] != 9 {
		t.Fatalf("owner greeting recipients %+v", got)
	}
	away := owner.GetBusinessAwayMessage().GetRecipients()
	if !away.GetExcludeSelected() || len(away.GetUsers()) != 1 || away.GetUsers()[0] != 8 {
		t.Fatalf("owner away recipients %+v", away)
	}
}
