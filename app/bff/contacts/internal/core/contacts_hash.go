package core

import (
	"sort"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/statusbook"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// contactsHash is contacts.getContacts hash from current Android
// ContactsController.getContactsHash. saved_count is mixed first, then each
// contact user id in ascending order, with MediaDataController.calcHash
// (the 64-bit combine / vector hash). A client hash of 0 still asks for the
// full list. A non-zero hash that matches is contacts.contactsNotModified.
//
// The previous mix, (acc*20261 + id) % 0x80000000, is the pre-64-bit
// algorithm. Current Android never sends that value, so the server always
// returned the full book and the client refetched.
func contactsHash(savedCount int32, contacts []*mtproto.Contact) int64 {
	ids := make([]int64, 0, len(contacts))
	for _, c := range contacts {
		if c != nil && c.GetUserId() != 0 {
			ids = append(ids, c.GetUserId())
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	acc := mixContactsHash(0, int64(savedCount))
	for _, id := range ids {
		acc = mixContactsHash(acc, id)
	}
	return acc
}

// mixContactsHash is MediaDataController.calcHash(long, long):
//
//	hash ^= hash >>> 21
//	hash ^= hash << 35
//	hash ^= hash >>> 4
//	return hash + id
//
// The shifts are unsigned, matching Java >>>.
func mixContactsHash(acc, n int64) int64 {
	u := uint64(acc)
	u ^= u >> 21
	u ^= u << 35
	u ^= u >> 4
	return int64(u + uint64(n))
}

// visibleContacts keeps one contact per user id, and only contacts whose
// user object is in the same response. Android processLoadedContacts calls
// loadContacts(false, 0) for every contact that is not in its user cache.
// Hash 0 never accepts contactsNotModified, so a missing user is a tight loop.
func visibleContacts(contacts []*mtproto.Contact, users []*mtproto.User) ([]*mtproto.Contact, []*mtproto.User) {
	have := make(map[int64]*mtproto.User, len(users))
	for _, u := range users {
		if u != nil && u.GetId() != 0 {
			have[u.GetId()] = u
		}
	}
	seen := map[int64]bool{}
	kept := make([]*mtproto.Contact, 0, len(contacts))
	outUsers := make([]*mtproto.User, 0, len(contacts))
	for _, c := range contacts {
		if c == nil || c.GetUserId() == 0 || seen[c.GetUserId()] {
			continue
		}
		u, ok := have[c.GetUserId()]
		if !ok {
			continue
		}
		seen[c.GetUserId()] = true
		kept = append(kept, c)
		outUsers = append(outUsers, u)
	}
	return kept, outUsers
}

func contactIDsForPhones(rows []*mtproto.ContactData, phones []string) []int64 {
	want := map[string]bool{}
	for _, phone := range phones {
		if d := digitsOnly(phone); d != "" {
			want[d] = true
		}
	}
	if len(want) == 0 {
		return nil
	}
	var ids []int64
	seen := map[int64]bool{}
	for _, row := range rows {
		if row == nil {
			continue
		}
		phone := ""
		if row.GetPhone() != nil {
			phone = row.GetPhone().GetValue()
		}
		if !want[digitsOnly(phone)] {
			continue
		}
		id := row.GetContactUserId()
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fetchContactRows, dropContactIDs, and loadContactUsers are the user-service
// calls. Tests replace them and restore the originals.
var (
	fetchContactRows = func(c *ContactsCore) ([]*mtproto.ContactData, error) {
		return c.loadContactRows()
	}
	dropContactIDs = func(c *ContactsCore, ids []int64) error {
		return c.deleteContactIDs(ids)
	}
	loadContactUsers = func(c *ContactsCore, ids []int64) ([]*mtproto.User, error) {
		return c.loadContactUsers(ids)
	}
)

func (c *ContactsCore) contactRows() ([]*mtproto.ContactData, error) {
	return fetchContactRows(c)
}

func (c *ContactsCore) loadContactRows() ([]*mtproto.ContactData, error) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	list, err := c.svcCtx.Dao.UserGetContactList(c.ctx, &userpb.TLUserGetContactList{UserId: c.MD.UserId})
	if err != nil {
		return nil, err
	}
	if list == nil {
		return nil, nil
	}
	return list.GetDatas(), nil
}

func (c *ContactsCore) removeContactIDs(ids []int64) error {
	return dropContactIDs(c, ids)
}

func (c *ContactsCore) contactUsers(ids []int64) ([]*mtproto.User, error) {
	return loadContactUsers(c, ids)
}

func (c *ContactsCore) loadContactUsers(ids []int64) ([]*mtproto.User, error) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || c.MD == nil {
		return nil, mtproto.ErrInternalServerError
	}
	got, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: append([]int64{c.MD.UserId}, ids...),
		To: []int64{c.MD.UserId},
	})
	if err != nil {
		return nil, err
	}
	if got == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return got.GetUserListByIdList(c.MD.UserId, ids...), nil
}

func (c *ContactsCore) deleteContactIDs(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return mtproto.ErrInternalServerError
	}
	for _, id := range ids {
		if _, err := c.svcCtx.Dao.UserDeleteContact(c.ctx, &userpb.TLUserDeleteContact{
			UserId: c.MD.UserId,
			Id:     id,
		}); err != nil {
			return err
		}
	}
	if err := statusbook.Shared().Forget(c.ctx, append([]int64{c.MD.UserId}, ids...)...); err != nil {
		return err
	}
	return nil
}
