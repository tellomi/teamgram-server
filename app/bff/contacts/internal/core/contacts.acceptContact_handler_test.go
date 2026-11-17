package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

// The shipped gate is peerSettings.add_contact. The inbox sets it, with
// report_spam and block_contact, only on the recipient of a new private
// dialog. These cases stub that stored row. They do not decide when the row
// is written: a live pair is e2e/client/accept_contact_bar_test.go. A closed
// bar is 400 CONTACT_ADD_MISSING in either order.

func TestAcceptContactRejectsAClosedBar(t *testing.T) {
	c := acceptCore(11, &acceptUsers{})
	empty := mtproto.MakeTLInputUserEmpty(&mtproto.InputUser{}).To_InputUser()
	if _, err := c.ContactsAcceptContact(acceptReq(empty)); rpcMessage(err) != "CONTACT_ID_INVALID" {
		t.Fatalf("empty id: %v", err)
	}
	self := acceptInput(11)
	if _, err := c.ContactsAcceptContact(acceptReq(self)); rpcMessage(err) != "CONTACT_ID_INVALID" {
		t.Fatalf("self: %v", err)
	}
	bare := &ContactsCore{Logger: logx.WithContext(context.Background())}
	if _, err := bare.ContactsAcceptContact(acceptReq(acceptInput(12))); rpcMessage(err) != "USER_ID_INVALID" {
		t.Fatalf("no session: %v", err)
	}

	users := &acceptUsers{}
	a := acceptCore(11, users)
	b := acceptCore(12, users)
	// Neither side has the bar. Both orders stop before a contact row is written.
	for _, call := range []struct {
		name string
		c    *ContactsCore
		id   int64
	}{
		{"a then b / a", a, 12},
		{"a then b / b", b, 11},
		{"b then a / b", b, 11},
		{"b then a / a", a, 12},
	} {
		if _, err := call.c.ContactsAcceptContact(acceptReq(acceptInput(call.id))); rpcMessage(err) != "CONTACT_ADD_MISSING" || status.Code(err) != 400 {
			t.Fatalf("%s: %v", call.name, err)
		}
	}
	if len(users.added) != 0 {
		t.Fatalf("closed bar wrote %d contacts", len(users.added))
	}

	// report_spam and block_contact without add_contact are not this bar.
	users.put(11, 12, &mtproto.PeerSettings{ReportSpam: true, BlockContact: true})
	if _, err := a.ContactsAcceptContact(acceptReq(acceptInput(12))); rpcMessage(err) != "CONTACT_ADD_MISSING" {
		t.Fatalf("spam without add_contact: %v", err)
	}

	// No bar, unknown id: same error, so the call does not reveal that the id exists.
	if _, err := a.ContactsAcceptContact(acceptReq(acceptInput(99))); rpcMessage(err) != "CONTACT_ADD_MISSING" {
		t.Fatalf("unknown without bar: %v", err)
	}
}

func TestAcceptContactBothOrdersWhenStrangerBarIsOpen(t *testing.T) {
	testMutableUsers = func(_ context.Context, in *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
		var users []*mtproto.ImmutableUser
		seen := map[int64]struct{}{}
		for _, id := range in.GetId() {
			if id != 11 && id != 12 {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			phone := "111"
			name := "A"
			if id == 12 {
				phone = "222"
				name = "B"
			}
			users = append(users, &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id, FirstName: name, Phone: phone}})
		}
		return &mtproto.MutableUsers{Users: users}, nil
	}
	t.Cleanup(func() { testMutableUsers = nil })

	for _, order := range [][2]int64{{11, 12}, {12, 11}} {
		users := &acceptUsers{}
		a := acceptCore(order[0], users)
		b := acceptCore(order[1], users)
		users.put(order[0], order[1], strangerBar())
		users.put(order[1], order[0], strangerBar())

		first := acceptOnce(t, a, order[1])
		if !first.contact || !first.cleared || !first.phoneShared {
			t.Fatalf("order %v first %#v", order, first)
		}
		second := acceptOnce(t, b, order[0])
		if !second.contact || !second.cleared || !second.phoneShared {
			t.Fatalf("order %v second %#v", order, second)
		}
		// user.addContact deletes the bar. The next call is the same 400.
		if _, err := a.ContactsAcceptContact(acceptReq(acceptInput(order[1]))); rpcMessage(err) != "CONTACT_ADD_MISSING" {
			t.Fatalf("order %v repeat: %v", order, err)
		}
		if _, err := b.ContactsAcceptContact(acceptReq(acceptInput(order[0]))); rpcMessage(err) != "CONTACT_ADD_MISSING" {
			t.Fatalf("order %v repeat peer: %v", order, err)
		}
	}

	// The bar names a user the account service does not have.
	users := &acceptUsers{}
	users.put(11, 99, strangerBar())
	if _, err := acceptCore(11, users).ContactsAcceptContact(acceptReq(acceptInput(99))); rpcMessage(err) != "CONTACT_ID_INVALID" {
		t.Fatalf("unknown with bar: %v", err)
	}
	if len(users.added) != 0 {
		t.Fatal("invalid id was stored")
	}
}

type acceptResult struct {
	contact     bool
	cleared     bool
	phoneShared bool
}

func acceptOnce(t *testing.T, c *ContactsCore, peer int64) acceptResult {
	t.Helper()
	out, err := c.ContactsAcceptContact(acceptReq(acceptInput(peer)))
	if err != nil {
		t.Fatal(err)
	}
	var got acceptResult
	for _, user := range out.GetUsers() {
		if user.GetId() == peer && user.GetContact() && user.GetMutualContact() {
			got.contact = true
		}
	}
	for _, update := range out.GetUpdates() {
		settings := update.GetSettings()
		peerID := update.GetPeer_PEER().GetUserId()
		if peerID == peer && settings != nil && !settings.GetAddContact() && !settings.GetReportSpam() && !settings.GetBlockContact() && !settings.GetShareContact() {
			got.cleared = true
		}
	}
	store := c.svcCtx.Dao.UserClient.(*acceptUsers)
	if n := len(store.added); n > 0 {
		last := store.added[n-1]
		got.phoneShared = last.GetUserId() == c.MD.UserId && last.GetId() == peer && mtproto.FromBool(last.GetAddPhonePrivacyException())
	}
	return got
}

func strangerBar() *mtproto.PeerSettings {
	return &mtproto.PeerSettings{ReportSpam: true, AddContact: true, BlockContact: true}
}

func acceptCore(self int64, users *acceptUsers) *ContactsCore {
	return &ContactsCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: users}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: self},
	}
}

func acceptReq(id *mtproto.InputUser) *mtproto.TLContactsAcceptContact {
	return &mtproto.TLContactsAcceptContact{
		Constructor: mtproto.CRC32_contacts_acceptContact,
		Id:          id,
	}
}

func acceptInput(id int64) *mtproto.InputUser {
	return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: id, AccessHash: 1}).To_InputUser()
}

func rpcMessage(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}

type acceptUsers struct {
	userclient.UserClient
	settings map[int64]map[int64]*mtproto.PeerSettings
	added    []*userpb.TLUserAddContact
}

func (u *acceptUsers) put(owner, peer int64, settings *mtproto.PeerSettings) {
	if u.settings == nil {
		u.settings = map[int64]map[int64]*mtproto.PeerSettings{}
	}
	if u.settings[owner] == nil {
		u.settings[owner] = map[int64]*mtproto.PeerSettings{}
	}
	u.settings[owner][peer] = settings
}

func (u *acceptUsers) UserGetPeerSettings(_ context.Context, in *userpb.TLUserGetPeerSettings) (*mtproto.PeerSettings, error) {
	return u.settings[in.GetUserId()][in.GetPeerId()], nil
}

func (u *acceptUsers) UserAddContact(_ context.Context, in *userpb.TLUserAddContact) (*mtproto.Bool, error) {
	u.added = append(u.added, in)
	// user.addContact deletes the stranger bar before the RPC returns.
	if row := u.settings[in.GetUserId()]; row != nil {
		delete(row, in.GetId())
	}
	return mtproto.BoolTrue, nil
}
