package core

import (
	"sort"
	"strings"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// contactsHash is the contacts.getContacts hash from the official clients.
// hash 0 means the client wants the full list. A matching hash is
// contacts.contactsNotModified.
func contactsHash(savedCount int32, contacts []*mtproto.Contact) int64 {
	ids := make([]int64, 0, len(contacts))
	for _, c := range contacts {
		if c != nil {
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

func mixContactsHash(acc, n int64) int64 {
	const m int64 = 0x80000000
	acc = posMod(acc, m)
	// 0x80000000 is 0 modulo m, which is why the client adds it before
	// the modulo: it keeps a 32-bit intermediate non-negative.
	return posMod(acc*20261+posMod(n, m), m)
}

func posMod(n, m int64) int64 {
	n %= m
	if n < 0 {
		n += m
	}
	return n
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

// fetchContactRows and dropContactIDs are the user-service calls.
// Tests replace them and restore the originals.
var (
	fetchContactRows = func(c *ContactsCore) ([]*mtproto.ContactData, error) {
		return c.loadContactRows()
	}
	dropContactIDs = func(c *ContactsCore, ids []int64) error {
		return c.deleteContactIDs(ids)
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
	return nil
}
