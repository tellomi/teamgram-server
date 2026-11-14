package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestGetBlockedOffsetLimitAndSlice(t *testing.T) {
	users := &blockedPageUser{total: 5}
	ctx := context.Background()
	c := &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: users}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}

	first, err := c.ContactsGetBlocked(&mtproto.TLContactsGetBlocked{Offset: 0, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if users.offset != 0 || users.limit != 2 {
		t.Fatalf("first query offset %d limit %d", users.offset, users.limit)
	}
	if first.GetPredicateName() != mtproto.Predicate_contacts_blockedSlice || first.GetCount() != 5 || len(first.GetBlocked()) != 2 {
		t.Fatalf("first page %s count %d rows %d", first.GetPredicateName(), first.GetCount(), len(first.GetBlocked()))
	}

	last, err := c.ContactsGetBlocked(&mtproto.TLContactsGetBlocked{Offset: 4, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if users.offset != 4 || users.limit != 2 {
		t.Fatalf("last query offset %d limit %d", users.offset, users.limit)
	}
	if last.GetPredicateName() != mtproto.Predicate_contacts_blocked || len(last.GetBlocked()) != 1 {
		t.Fatalf("last page %s rows %d", last.GetPredicateName(), len(last.GetBlocked()))
	}

	if _, err = c.ContactsGetBlocked(&mtproto.TLContactsGetBlocked{Offset: 0, Limit: 0}); err != nil {
		t.Fatal(err)
	}
	if users.limit != 100 {
		t.Fatalf("default limit %d", users.limit)
	}
	if _, err = c.ContactsGetBlocked(&mtproto.TLContactsGetBlocked{Offset: 3, Limit: 500}); err != nil {
		t.Fatal(err)
	}
	if users.offset != 3 || users.limit != 100 {
		t.Fatalf("capped query offset %d limit %d", users.offset, users.limit)
	}
}

// blockedPageUser pages a fixed list the way user.getBlockedPage does:
// offset skips rows, limit is the page size, and a short page is contacts.blocked.
type blockedPageUser struct {
	userclient.UserClient
	total         int
	offset, limit int32
}

func (u *blockedPageUser) UserGetBlockedPage(_ context.Context, in *user.TLUserGetBlockedList) (*mtproto.Contacts_Blocked, error) {
	u.offset = in.GetOffset()
	u.limit = in.GetLimit()
	start := int(in.GetOffset())
	if start > u.total {
		start = u.total
	}
	end := start + int(in.GetLimit())
	if end > u.total {
		end = u.total
	}
	rows := make([]*mtproto.PeerBlocked, 0, end-start)
	for id := start; id < end; id++ {
		rows = append(rows, mtproto.MakeTLPeerBlocked(&mtproto.PeerBlocked{
			PeerId: mtproto.MakePeerUser(int64(100 + id)),
			Date:   int32(id + 1),
		}).To_PeerBlocked())
	}
	out := &mtproto.Contacts_Blocked{
		Blocked: rows,
		Chats:   []*mtproto.Chat{},
		Users:   []*mtproto.User{},
		Count:   int32(u.total),
	}
	if start+len(rows) < u.total {
		return mtproto.MakeTLContactsBlockedSlice(out).To_Contacts_Blocked(), nil
	}
	return mtproto.MakeTLContactsBlocked(out).To_Contacts_Blocked(), nil
}

func (u *blockedPageUser) UserGetMutableUsers(context.Context, *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error) {
	return &user.Vector_ImmutableUser{}, nil
}
