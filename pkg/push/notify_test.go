package push

import "testing"

func TestDecideMutePreviewAndSilent(t *testing.T) {
	now := int64(1_700_000_000)
	peer := NotifySettings{Found: true, ShowPreviews: 0, Silent: -1, MuteUntil: int32(now + 100), Sound: "chord"}
	global := NotifySettings{Found: true, ShowPreviews: 1, Silent: 0, MuteUntil: 0, Sound: "default"}

	if d := Decide(now, peer, global, false, false); d.Send {
		t.Fatal("muted chat should not send")
	}
	mentioned := Decide(now, peer, global, true, false)
	if !mentioned.Send || mentioned.Preview || mentioned.Sound != "chord" {
		t.Fatalf("mention: %+v", mentioned)
	}
	silent := Decide(now, NotifySettings{Found: true, ShowPreviews: 1, Silent: 1, MuteUntil: 0, Sound: "chord"}, NotifySettings{}, false, false)
	if !silent.Send || !silent.Silent || silent.Sound != "" {
		t.Fatalf("silent: %+v", silent)
	}
	inherited := Decide(now, NotifySettings{Found: true, ShowPreviews: -1, Silent: -1, MuteUntil: -1}, global, false, false)
	if !inherited.Send || !inherited.Preview || inherited.Sound != "default" {
		t.Fatalf("inherit: %+v", inherited)
	}
	messageSilent := Decide(now, NotifySettings{}, NotifySettings{}, false, true)
	if !messageSilent.Silent || messageSilent.Sound != "" {
		t.Fatalf("message silent: %+v", messageSilent)
	}
}
