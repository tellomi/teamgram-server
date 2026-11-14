package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestPremiumNotRequiredToContact(t *testing.T) {
	c := &PrivacySettingsCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
	got, err := c.UsersGetIsPremiumRequiredToContact(&mtproto.TLUsersGetIsPremiumRequiredToContact{
		Id: []*mtproto.InputUser{
			mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2}).To_InputUser(),
			mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
			nil,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetDatas()) != 2 {
		t.Fatalf("len %d", len(got.GetDatas()))
	}
	for _, b := range got.GetDatas() {
		if b.GetPredicateName() != mtproto.Predicate_boolFalse {
			t.Fatalf("bool %s", b.GetPredicateName())
		}
	}
	empty, err := c.UsersGetIsPremiumRequiredToContact(&mtproto.TLUsersGetIsPremiumRequiredToContact{})
	if err != nil || len(empty.GetDatas()) != 0 {
		t.Fatalf("empty %+v %v", empty, err)
	}
}

func TestRequirementsToContactSkipsNil(t *testing.T) {
	c := &PrivacySettingsCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
	got, err := c.UsersGetRequirementsToContact(&mtproto.TLUsersGetRequirementsToContact{
		Id: []*mtproto.InputUser{
			mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2}).To_InputUser(),
			nil,
			mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetDatas()) != 2 {
		t.Fatalf("len %d", len(got.GetDatas()))
	}
	for _, item := range got.GetDatas() {
		if item.GetPredicateName() != mtproto.Predicate_requirementToContactEmpty {
			t.Fatalf("requirement %s", item.GetPredicateName())
		}
	}
	if _, err = c.premiumRequired(nil); err != nil {
		t.Fatal(err)
	}
}
