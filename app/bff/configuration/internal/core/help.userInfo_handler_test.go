package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/userinfo"
	"google.golang.org/grpc/status"
)

func TestHelpUserInfoRequiresSupport(t *testing.T) {
	notes := userinfo.NewMemory()
	target := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 9}).To_InputUser()
	edit := &mtproto.TLHelpEditUserInfo{UserId: target, Message: "internal note"}
	get := &mtproto.TLHelpGetUserInfo{UserId: target}

	stranger := supportCore(notes, 4, func(_ context.Context, id int64) (bool, error) {
		return id == 7, nil
	})
	if _, err := stranger.HelpEditUserInfo(edit); !forbiddenUser(err) {
		t.Fatalf("edit: %v", err)
	}
	if _, err := stranger.HelpGetUserInfo(get); !forbiddenUser(err) {
		t.Fatalf("get: %v", err)
	}
	empty, err := notes.Get(context.Background(), 9)
	if err != nil || empty.GetPredicateName() != mtproto.Predicate_help_userInfoEmpty {
		t.Fatalf("note leaked %#v %v", empty, err)
	}

	closed := supportCore(notes, 4, nil)
	if _, err = closed.HelpEditUserInfo(edit); !forbiddenUser(err) {
		t.Fatalf("no checker: %v", err)
	}

	support := supportCore(notes, 7, func(_ context.Context, id int64) (bool, error) {
		return id == 7, nil
	})
	saved, err := support.HelpEditUserInfo(edit)
	if err != nil || saved.GetMessage() != "internal note" || saved.GetAuthor() != "7" {
		t.Fatalf("saved %#v %v", saved, err)
	}
	subject := supportCore(notes, 9, func(_ context.Context, id int64) (bool, error) {
		return id == 7, nil
	})
	if _, err = subject.HelpGetUserInfo(get); !forbiddenUser(err) {
		t.Fatalf("subject read: %v", err)
	}
	got, err := support.HelpGetUserInfo(get)
	if err != nil || got.GetMessage() != "internal note" {
		t.Fatalf("support read %#v %v", got, err)
	}
	anon := &ConfigurationCore{ctx: context.Background(), svcCtx: stranger.svcCtx}
	if _, err = anon.HelpEditUserInfo(edit); status.Convert(err).Message() != "AUTH_KEY_UNREGISTERED" {
		t.Fatalf("anon: %v", err)
	}
}

func supportCore(notes *userinfo.Store, userID int64, allow func(context.Context, int64) (bool, error)) *ConfigurationCore {
	return &ConfigurationCore{
		ctx: context.Background(),
		MD:  &metadata.RpcMetadata{UserId: userID},
		svcCtx: &svc.ServiceContext{
			UserInfo: notes,
			Support:  allow,
		},
	}
}

func forbiddenUser(err error) bool {
	return status.Code(err) == mtproto.ErrForbidden && status.Convert(err).Message() == "USER_INVALID"
}
