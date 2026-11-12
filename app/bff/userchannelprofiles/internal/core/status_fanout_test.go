package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func allowContacts(id int64, forward map[int64]bool) func(int64) bool {
	return func(viewer int64) bool { return forward[viewer] }
}

func TestStatusRecipientsIncludeReverseWhenEverybody(t *testing.T) {
	forward := []int64{2, 0, 2}
	reverse := []int64{2, 5, 1, 6}
	everybody := func(int64) bool { return true }
	got := statusRecipients(1, forward, reverse, everybody, nil)
	if len(got) != 3 || got[0] != 2 || got[1] != 5 || got[2] != 6 {
		t.Fatalf("everybody recipients %v", got)
	}
	again := statusRecipients(1, forward, reverse, everybody, nil)
	if len(again) != len(got) || again[1] != got[1] {
		t.Fatalf("repeat %v", again)
	}
}

func TestStatusRecipientsContactsHideReverseOnly(t *testing.T) {
	forward := map[int64]bool{2: true, 3: true}
	blocked := map[int64]bool{3: true}
	got := statusRecipients(1, []int64{2, 3}, []int64{5, 2}, allowContacts(1, forward), func(id int64) bool {
		return blocked[id]
	})
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("contacts recipients %v, want only the unblocked forward contact", got)
	}
}

func TestStatusRecipientsAllowUserException(t *testing.T) {
	visible := map[int64]bool{9: true}
	got := statusRecipients(1, nil, []int64{8, 9}, func(id int64) bool { return visible[id] }, nil)
	if len(got) != 1 || got[0] != 9 {
		t.Fatalf("exception recipients %v", got)
	}
}

func TestDeliverStatusReachesPrivacyAudience(t *testing.T) {
	updates := mtproto.MakeTLUpdateShort(&mtproto.Updates{
		Update: mtproto.MakeTLUpdateUserStatus(&mtproto.Update{UserId: 1}).To_Update(),
	}).To_Updates()
	var got []int64
	push := func(id int64, up *mtproto.Updates) {
		if up.GetUpdate().GetUserId() != 1 {
			t.Fatalf("payload user %d", up.GetUpdate().GetUserId())
		}
		got = append(got, id)
	}
	deliverUserStatus(1, updates, []int64{2}, []int64{5}, func(int64) bool { return true }, nil, push)
	deliverUserStatus(1, updates, []int64{2}, []int64{5}, func(int64) bool { return true }, nil, push)
	if len(got) != 4 || got[0] != 2 || got[1] != 5 || got[2] != 2 || got[3] != 5 {
		t.Fatalf("pushes %v", got)
	}
	deliverUserStatus(1, updates, []int64{2}, nil, nil, nil, nil)
}

func TestAttachForwardContactsFeedsPrivacy(t *testing.T) {
	me := mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: mtproto.MakeTLUserData(&mtproto.UserData{Id: 1}).To_UserData(),
		KeysPrivacyRules: []*mtproto.PrivacyKeyRules{
			mtproto.MakeTLPrivacyKeyRules(&mtproto.PrivacyKeyRules{
				Key:   mtproto.STATUS_TIMESTAMP,
				Rules: []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowContacts(nil).To_PrivacyRule()},
			}).To_PrivacyKeyRules(),
		},
	}).To_ImmutableUser()
	attachForwardContacts(me, []int64{1, 2, 0})
	if !me.CheckPrivacy(mtproto.STATUS_TIMESTAMP, 2) {
		t.Fatal("a person I added is my contact for status timestamp")
	}
	if me.CheckPrivacy(mtproto.STATUS_TIMESTAMP, 5) {
		t.Fatal("someone who only added me is not my contact")
	}
}
