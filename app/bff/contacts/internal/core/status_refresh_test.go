package core

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func statusRules(rules ...*mtproto.PrivacyRule) []*mtproto.PrivacyKeyRules {
	return []*mtproto.PrivacyKeyRules{
		mtproto.MakeTLPrivacyKeyRules(&mtproto.PrivacyKeyRules{
			Key:   mtproto.STATUS_TIMESTAMP,
			Rules: rules,
		}).To_PrivacyKeyRules(),
	}
}

func statusUser(id int64, rules []*mtproto.PrivacyKeyRules, contacts ...int64) *mtproto.ImmutableUser {
	user := mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User:             mtproto.MakeTLUserData(&mtproto.UserData{Id: id, FirstName: "U"}).To_UserData(),
		LastSeenAt:       time.Now().Unix(),
		KeysPrivacyRules: rules,
	}).To_ImmutableUser()
	setForwardContacts(user, contacts)
	return user
}

func TestContactStatusRefreshRespectsPrivacy(t *testing.T) {
	const meID, peerID int64 = 1, 2
	allowAll := statusRules(mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule())
	allowContacts := statusRules(mtproto.MakeTLPrivacyValueAllowContacts(nil).To_PrivacyRule())
	disallowAll := statusRules(mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule())

	// Default rules are everybody. Adding them lets both sides see the exact
	// online window, which is the heartbeat plus UserOnlinePeriod.
	me := statusUser(meID, allowAll, peerID)
	peer := statusUser(peerID, allowAll)
	forPeer, forMe := contactStatusRefresh(me, peer)
	if forPeer.GetPredicateName() != mtproto.Predicate_userStatusOnline || forMe.GetPredicateName() != mtproto.Predicate_userStatusOnline {
		t.Fatalf("allow all: %s %s", forPeer.GetPredicateName(), forMe.GetPredicateName())
	}
	if int64(forPeer.GetExpires()) != me.LastSeenAt+int64(mtproto.UserOnlinePeriod) {
		t.Fatalf("online expires %d", forPeer.GetExpires())
	}

	// My contacts only, and I added them. They allow everybody, so both
	// directions are open: they are my contact, and they do not hide theirs.
	me = statusUser(meID, allowContacts, peerID)
	peer = statusUser(peerID, allowAll)
	forPeer, forMe = contactStatusRefresh(me, peer)
	if forPeer.GetPredicateName() != mtproto.Predicate_userStatusOnline || forMe.GetPredicateName() != mtproto.Predicate_userStatusOnline {
		t.Fatalf("I added them and they allow everybody: %s %s", forPeer.GetPredicateName(), forMe.GetPredicateName())
	}

	// Both sides are contacts-only and they have not added me. Each side is
	// hiding exact last seen from the other, so neither push is exact.
	peer = statusUser(peerID, allowContacts)
	forPeer, forMe = contactStatusRefresh(me, peer)
	if forPeer.GetPredicateName() == mtproto.Predicate_userStatusOnline || forMe.GetPredicateName() == mtproto.Predicate_userStatusOnline {
		t.Fatalf("one-way contacts-only must stay approximate: %s %s", forPeer.GetPredicateName(), forMe.GetPredicateName())
	}

	// Mutual contacts under the contacts rule see the exact window.
	peer = statusUser(peerID, allowContacts, meID)
	forPeer, forMe = contactStatusRefresh(me, peer)
	if forPeer.GetPredicateName() != mtproto.Predicate_userStatusOnline || forMe.GetPredicateName() != mtproto.Predicate_userStatusOnline {
		t.Fatalf("mutual contacts: %s %s", forPeer.GetPredicateName(), forMe.GetPredicateName())
	}

	// I hide last seen from everybody. I also stop seeing their exact time.
	me = statusUser(meID, disallowAll, peerID)
	peer = statusUser(peerID, allowAll)
	forPeer, forMe = contactStatusRefresh(me, peer)
	if forPeer.GetPredicateName() == mtproto.Predicate_userStatusOnline || forPeer.GetWasOnline() != 0 {
		t.Fatalf("disallow all must not push an exact status: %+v", forPeer)
	}
	if forMe.GetPredicateName() == mtproto.Predicate_userStatusOnline {
		t.Fatal("hiding my last seen also hides theirs")
	}
}

func TestUserStatusPushSkipsEmpty(t *testing.T) {
	if userStatusPush(1, nil) != nil {
		t.Fatal("nil status is not a push")
	}
	up := userStatusPush(4, mtproto.MakeTLUserStatusEmpty(nil).To_UserStatus())
	if up.GetUpdate().GetUserId() != 4 || up.GetUpdate().GetStatus_USERSTATUS().GetPredicateName() != mtproto.Predicate_userStatusEmpty {
		t.Fatalf("push %+v", up)
	}
}
