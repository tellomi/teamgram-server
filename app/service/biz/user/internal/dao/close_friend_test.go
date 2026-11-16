package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestCompactCloseFriendsKeepsOnlySetRows(t *testing.T) {
	a := mtproto.MakeTLContactData(&mtproto.ContactData{ContactUserId: 1, CloseFriend: true}).To_ContactData()
	b := mtproto.MakeTLContactData(&mtproto.ContactData{ContactUserId: 2, CloseFriend: true}).To_ContactData()
	got := compactCloseFriends([]*mtproto.ContactData{nil, a, nil, b})
	if len(got) != 2 || got[0].GetContactUserId() != 1 || got[1].GetContactUserId() != 2 {
		t.Fatalf("%v", got)
	}
	if len(compactCloseFriends([]*mtproto.ContactData{nil, nil})) != 0 {
		t.Fatal("all nil should be an empty list, not a failure")
	}
}
