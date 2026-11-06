package dao

import (
	"reflect"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

func TestForwardContactIDsFollowMutableUserShapes(t *testing.T) {
	const viewer, peer, other int64 = 10, 20, 30

	// messages.getDialogs loads the viewer and the dialog peers, with to = viewer.
	// The peer's "my contacts" check needs the edge to the viewer. The viewer's
	// address book needs edges to peers in this batch, and nobody outside it.
	if got := forwardContactIDs(peer, []int64{viewer, peer, other}, []int64{viewer}, []int64{viewer, 99}); !reflect.DeepEqual(got, []int64{viewer}) {
		t.Fatalf("dialog peer contacts = %v", got)
	}
	if got := forwardContactIDs(viewer, []int64{viewer, peer, other}, []int64{viewer}, []int64{peer, 99}); !reflect.DeepEqual(got, []int64{peer}) {
		t.Fatalf("dialog viewer contacts = %v", got)
	}

	// users.getFullUser passes both ids in to, so each side loads the other.
	if got := forwardContactIDs(peer, []int64{viewer, peer}, []int64{viewer, peer}, []int64{viewer, 99}); !reflect.DeepEqual(got, []int64{viewer}) {
		t.Fatalf("full user peer contacts = %v", got)
	}
	if got := forwardContactIDs(viewer, []int64{viewer, peer}, []int64{viewer, peer}, []int64{peer}); !reflect.DeepEqual(got, []int64{peer}) {
		t.Fatalf("full user viewer contacts = %v", got)
	}

	// contacts.addContact puts only the peer in to. The viewer still needs
	// their own edge to that peer, and the peer still needs the edge back.
	if got := forwardContactIDs(viewer, []int64{viewer, peer}, []int64{peer}, []int64{peer}); !reflect.DeepEqual(got, []int64{peer}) {
		t.Fatalf("add-contact viewer contacts = %v", got)
	}
	if got := forwardContactIDs(peer, []int64{viewer, peer}, []int64{peer}, []int64{viewer}); !reflect.DeepEqual(got, []int64{viewer}) {
		t.Fatalf("add-contact peer contacts = %v", got)
	}

	// An empty to (chat send, history) stays inside the loaded ids.
	if got := forwardContactIDs(peer, []int64{viewer, peer}, nil, []int64{viewer, other}); !reflect.DeepEqual(got, []int64{viewer}) {
		t.Fatalf("empty to contacts = %v", got)
	}
	if got := forwardContactIDs(peer, []int64{peer}, []int64{viewer}, nil); got != nil {
		t.Fatalf("no saved contacts = %v", got)
	}
}

func TestAttachContactRowFeedsPrivacyAndContactFlag(t *testing.T) {
	const viewerID, peerID int64 = 10, 20
	viewer, peer := privacyPair(viewerID, peerID, allowContactsRules())
	mUsers := map[int64]*mtproto.ImmutableUser{viewerID: viewer, peerID: peer}

	hidden := peer.ToUnsafeUser(viewer)
	assertHiddenContact(t, hidden, "Peer")

	// The peer saved the viewer. The viewer has not saved the peer.
	attachContactRow(mUsers, &dataobject.UserContactsDO{
		OwnerUserId:      peerID,
		ContactUserId:    viewerID,
		ContactFirstName: "saved-viewer",
		ContactPhone:     "15550001",
	})
	attachContactRow(mUsers, &dataobject.UserContactsDO{
		OwnerUserId:      peerID,
		ContactUserId:    viewerID,
		ContactFirstName: "saved-viewer",
	})
	if len(peer.Contacts) != 1 || peer.Contacts[0].ContactUserId != viewerID {
		t.Fatalf("peer forward contacts = %+v", peer.Contacts)
	}
	if len(viewer.ReverseContacts) != 1 || viewer.ReverseContacts[0].UserId != peerID {
		t.Fatalf("viewer reverse contacts = %+v", viewer.ReverseContacts)
	}
	if len(viewer.Contacts) != 0 || len(peer.ReverseContacts) != 0 {
		t.Fatal("the row must not be stored on the opposite side")
	}
	if ok, _ := peer.CheckContact(viewerID); !ok {
		t.Fatal("CheckContact must see the forward row")
	}
	if ok, _ := viewer.CheckContact(peerID); ok {
		t.Fatal("the viewer has not saved the peer")
	}

	peer.LastSeenAt = time.Now().Unix()
	shown := peer.ToUnsafeUser(viewer)
	if shown.GetMin() {
		t.Fatal("both payloads are non-min, so a client replaces the cached user")
	}
	if shown.GetPhoto() == nil || shown.GetPhoto().GetPhotoId() != 42 {
		t.Fatalf("photo = %+v", shown.GetPhoto())
	}
	if shown.GetPhone().GetValue() != "15550002" {
		t.Fatalf("phone = %v", shown.GetPhone())
	}
	if shown.GetStatus().GetPredicateName() != mtproto.Predicate_userStatusOnline {
		t.Fatalf("status = %s", shown.GetStatus().GetPredicateName())
	}
	if shown.GetContact() || shown.GetFirstName().GetValue() != "Peer" {
		t.Fatalf("contact=%v name=%v", shown.GetContact(), shown.GetFirstName())
	}

	// The viewer's address book is not the peer's contact list.
	viewer2, peer2 := privacyPair(viewerID, peerID, allowContactsRules())
	attachContactRow(map[int64]*mtproto.ImmutableUser{viewerID: viewer2, peerID: peer2}, &dataobject.UserContactsDO{
		OwnerUserId:      viewerID,
		ContactUserId:    peerID,
		ContactFirstName: "Local Name",
		Mutual:           false,
	})
	saved := peer2.ToUnsafeUser(viewer2)
	if !saved.GetContact() || saved.GetMutualContact() || saved.GetFirstName().GetValue() != "Local Name" {
		t.Fatalf("contact=%v mutual=%v name=%v", saved.GetContact(), saved.GetMutualContact(), saved.GetFirstName())
	}
	if saved.GetPhoto() != nil || saved.GetPhone() != nil {
		t.Fatal("being in the viewer's contacts must not reveal a my-contacts photo or phone")
	}
	if saved.GetStatus().GetPredicateName() != mtproto.Predicate_userStatusRecently {
		t.Fatalf("status = %s", saved.GetStatus().GetPredicateName())
	}
}

func TestPrivacyRulesStayNarrowerThanAlwaysSend(t *testing.T) {
	const viewerID, peerID int64 = 10, 20
	viewer, peer := privacyPair(viewerID, peerID, []*mtproto.PrivacyKeyRules{
		privacyKey(mtproto.PROFILE_PHOTO, mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule()),
		privacyKey(mtproto.PHONE_NUMBER, mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule()),
		privacyKey(mtproto.STATUS_TIMESTAMP, mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule()),
	})
	peer.LastSeenAt = time.Now().Unix()
	got := peer.ToUnsafeUser(viewer)
	if got.GetPhoto() == nil {
		t.Fatal("default profile photo is allow all and does not need a contact row")
	}
	if got.GetPhone() != nil {
		t.Fatal("default phone is disallow all")
	}
	if got.GetStatus().GetPredicateName() != mtproto.Predicate_userStatusOnline {
		t.Fatalf("default status = %s", got.GetStatus().GetPredicateName())
	}

	attachContactRow(map[int64]*mtproto.ImmutableUser{viewerID: viewer, peerID: peer}, &dataobject.UserContactsDO{
		OwnerUserId:   peerID,
		ContactUserId: viewerID,
	})
	peer.KeysPrivacyRules = []*mtproto.PrivacyKeyRules{
		privacyKey(mtproto.PROFILE_PHOTO, mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule()),
		privacyKey(mtproto.PHONE_NUMBER, mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule()),
		privacyKey(mtproto.STATUS_TIMESTAMP, mtproto.MakeTLPrivacyValueAllowContacts(nil).To_PrivacyRule()),
	}
	peer.LastSeenAt = time.Now().Unix()
	blocked := peer.ToUnsafeUser(viewer)
	if blocked.GetPhoto() != nil || blocked.GetPhone() != nil {
		t.Fatal("disallow all must hide photo and phone from a contact")
	}
	if blocked.GetStatus().GetPredicateName() != mtproto.Predicate_userStatusOnline {
		t.Fatalf("contact status = %s", blocked.GetStatus().GetPredicateName())
	}
}

func privacyPair(viewerID, peerID int64, peerRules []*mtproto.PrivacyKeyRules) (*mtproto.ImmutableUser, *mtproto.ImmutableUser) {
	viewer := testImmutableUser(viewerID, "Viewer", "15550001", nil)
	peer := testImmutableUser(peerID, "Peer", "15550002", peerRules)
	peer.LastSeenAt = time.Now().Unix()
	return viewer, peer
}

func testImmutableUser(id int64, name, phone string, rules []*mtproto.PrivacyKeyRules) *mtproto.ImmutableUser {
	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: mtproto.MakeTLUserData(&mtproto.UserData{
			Id:        id,
			FirstName: name,
			Phone:     phone,
			ProfilePhoto: mtproto.MakeTLPhoto(&mtproto.Photo{
				Id: 42,
				Sizes: []*mtproto.PhotoSize{
					mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{Type: "a", W: 100, H: 100}).To_PhotoSize(),
				},
			}).To_Photo(),
		}).To_UserData(),
		LastSeenAt:       time.Now().Unix(),
		KeysPrivacyRules: rules,
	}).To_ImmutableUser()
}

func allowContactsRules() []*mtproto.PrivacyKeyRules {
	allow := mtproto.MakeTLPrivacyValueAllowContacts(nil).To_PrivacyRule()
	return []*mtproto.PrivacyKeyRules{
		privacyKey(mtproto.PROFILE_PHOTO, allow),
		privacyKey(mtproto.PHONE_NUMBER, allow),
		privacyKey(mtproto.STATUS_TIMESTAMP, allow),
	}
}

func privacyKey(key int, rule *mtproto.PrivacyRule) *mtproto.PrivacyKeyRules {
	return mtproto.MakeTLPrivacyKeyRules(&mtproto.PrivacyKeyRules{
		Key:   int32(key),
		Rules: []*mtproto.PrivacyRule{rule},
	}).To_PrivacyKeyRules()
}

func assertHiddenContact(t *testing.T, user *mtproto.User, name string) {
	t.Helper()
	if user.GetMin() {
		t.Fatal("user is non-min")
	}
	if user.GetPhoto() != nil || user.GetPhone() != nil {
		t.Fatalf("photo=%v phone=%v", user.GetPhoto(), user.GetPhone())
	}
	if user.GetStatus().GetPredicateName() != mtproto.Predicate_userStatusRecently {
		t.Fatalf("status = %s", user.GetStatus().GetPredicateName())
	}
	if user.GetContact() || user.GetFirstName().GetValue() != name {
		t.Fatalf("contact=%v name=%v", user.GetContact(), user.GetFirstName())
	}
}
