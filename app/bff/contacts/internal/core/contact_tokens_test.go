package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestContactAndNoPaidTokens(t *testing.T) {
	book := dao.NewTokenBook(nil)
	c := &ContactsCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Tokens: book}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 7},
	}
	testMutableUsers = func(_ context.Context, in *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
		var users []*mtproto.ImmutableUser
		for _, id := range in.GetId() {
			name := ""
			if id == 7 {
				name = "ada"
			}
			users = append(users, &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id, FirstName: "Ada", Username: name}})
		}
		return &mtproto.MutableUsers{Users: users}, nil
	}
	t.Cleanup(func() { testMutableUsers = nil })

	exp, err := c.ContactsExportContactToken(&mtproto.TLContactsExportContactToken{})
	if err != nil {
		t.Fatal(err)
	}
	if exp.GetUrl() == "" || exp.GetExpires() == 0 {
		t.Fatalf("export %#v", exp)
	}
	got, err := c.ContactsImportContactToken(&mtproto.TLContactsImportContactToken{Token: exp.GetUrl()})
	if err != nil || got.GetId() != 7 {
		t.Fatalf("import %#v %v", got, err)
	}
	if _, err := c.ContactsImportContactToken(&mtproto.TLContactsImportContactToken{Token: ""}); status.Convert(err).Message() != "IMPORT_TOKEN_INVALID" {
		t.Fatalf("empty token %v", err)
	}

	free, err := c.ContactsExportNoPaidMessagesToken(&mtproto.TLContactsExportNoPaidMessagesToken{Temporary: true})
	if err != nil {
		t.Fatal(err)
	}
	if free.GetExpires() == 0 || free.GetToken() == "" {
		t.Fatalf("free %#v", free)
	}
	if free.GetUrl() != "https://t.me/ada?free="+free.GetToken() {
		t.Fatalf("url %s", free.GetUrl())
	}
	c.MD.UserId = 9
	imported, err := c.ContactsImportNoPaidMessagesToken(&mtproto.TLContactsImportNoPaidMessagesToken{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 7, AccessHash: 1}).To_InputUser(),
		Token:  free.GetToken(),
	})
	if err != nil || imported.GetId() != 7 {
		t.Fatalf("import free %#v %v", imported, err)
	}
	if !book.HasNoPaid(context.Background(), 7, 9) {
		t.Fatal("missing exception")
	}
	if _, err := c.ContactsImportNoPaidMessagesToken(&mtproto.TLContactsImportNoPaidMessagesToken{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 7}).To_InputUser(),
		Token:  "not-a-token",
	}); status.Convert(err).Message() != "NO_PAID_MESSAGES_TOKEN_INVALID" {
		t.Fatalf("bad token %v", err)
	}
}
