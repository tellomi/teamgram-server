package contactphoto

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
	"google.golang.org/grpc/status"
)

func TestSaveShowsOnlyToTheCallerAndClearRemovesIt(t *testing.T) {
	Reset()
	user := inputUser(8)
	file := inputFile(9, 1)
	out, err := Upload(3, &mtproto.TLPhotosUploadContactProfilePhoto{UserId: user, Save: true, File: file})
	if err != nil || out.GetPhoto().GetId() != 9 {
		t.Fatalf("save %#v %v", out, err)
	}
	if Saved(3, 8).GetId() != 9 || ForViewer(context.Background(), 3, 8).GetId() != 9 {
		t.Fatal("caller cannot see the personal photo")
	}
	if Saved(4, 8) != nil || ForViewer(context.Background(), 8, 3) != nil || ForViewer(context.Background(), 8, 8) != nil {
		t.Fatal("personal photo leaked")
	}
	cleared, err := Upload(3, &mtproto.TLPhotosUploadContactProfilePhoto{UserId: user, Save: true})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.GetPhoto().GetPredicateName() != mtproto.Predicate_photoEmpty {
		t.Fatalf("clear returned %s", cleared.GetPhoto().GetPredicateName())
	}
	if Saved(3, 8) != nil {
		t.Fatal("clear left the personal photo")
	}
}

func TestSuggestSendsServiceMessage(t *testing.T) {
	Reset()
	msg := &fakeMsg{}
	users := &fakeUsers{contact: true}
	md := &metadata.RpcMetadata{UserId: 3, PermAuthKeyId: 11}
	out, err := UploadWith(context.Background(), md, Clients{Users: users, Msg: msg}, &mtproto.TLPhotosUploadContactProfilePhoto{
		UserId:  inputUser(8),
		Suggest: true,
		File:    inputFile(15, 1),
	})
	if err != nil || out.GetPhoto().GetId() != 15 {
		t.Fatalf("suggest %#v %v", out, err)
	}
	if Saved(3, 8) != nil {
		t.Fatal("suggest stored a personal photo")
	}
	if msg.sent == nil || msg.sent.GetPeerId() != 8 {
		t.Fatalf("send %#v", msg.sent)
	}
	action := msg.sent.GetMessage()[0].GetMessage().GetAction()
	if action.GetPredicateName() != mtproto.Predicate_messageActionSuggestProfilePhoto || action.GetPhoto().GetId() != 15 {
		t.Fatalf("action %#v", action)
	}
}

func TestSaveAndSuggestTogether(t *testing.T) {
	Reset()
	msg := &fakeMsg{}
	_, err := UploadWith(context.Background(), &metadata.RpcMetadata{UserId: 3}, Clients{Msg: msg, Users: &fakeUsers{contact: true}}, &mtproto.TLPhotosUploadContactProfilePhoto{
		UserId:  inputUser(8),
		Save:    true,
		Suggest: true,
		File:    inputFile(9, 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if Saved(3, 8).GetId() != 9 || msg.sent == nil {
		t.Fatal("save+suggest did not store and send")
	}
}

func TestContactMissingAndMediaErrors(t *testing.T) {
	Reset()
	md := &metadata.RpcMetadata{UserId: 3}
	_, err := UploadWith(context.Background(), md, Clients{Users: &fakeUsers{contact: false}}, &mtproto.TLPhotosUploadContactProfilePhoto{
		UserId: inputUser(8),
		Save:   true,
		File:   inputFile(9, 1),
	})
	if status.Convert(err).Message() != "CONTACT_MISSING" {
		t.Fatal(err)
	}
	_, err = Upload(3, &mtproto.TLPhotosUploadContactProfilePhoto{
		UserId: inputUser(8),
		Save:   true,
		File:   inputFile(9, 1),
		Video:  inputFile(10, 1),
	})
	if status.Convert(err).Message() != "MEDIA_INVALID" {
		t.Fatal(err)
	}
	media := &fakeMedia{photo: mtproto.MakeTLPhoto(&mtproto.Photo{Id: 77, Sizes: []*mtproto.PhotoSize{{Type: "a"}}}).To_Photo()}
	out, err := UploadWith(context.Background(), md, Clients{Media: media, Users: &fakeUsers{contact: true}}, &mtproto.TLPhotosUploadContactProfilePhoto{
		UserId: inputUser(8),
		Save:   true,
		File:   inputFile(9, 1),
	})
	if err != nil || out.GetPhoto().GetId() != 77 || Saved(3, 8).GetSizes()[0].GetType() != "a" {
		t.Fatalf("media photo %#v %v", out, err)
	}
}

func TestSchemaIsIdempotent(t *testing.T) {
	if !strings.Contains(schemaSQL, "CREATE TABLE IF NOT EXISTS `user_contact_photos`") {
		t.Fatal(schemaSQL)
	}
	if strings.Contains(strings.ToUpper(schemaSQL), "DROP ") || strings.Contains(strings.ToUpper(schemaSQL), "TRUNCATE") {
		t.Fatal("schema deletes data")
	}
}

func inputUser(id int64) *mtproto.InputUser {
	return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: id, AccessHash: 1}).To_InputUser()
}

func inputFile(id int64, parts int32) *mtproto.InputFile {
	return mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: id, Parts: parts, Name: "a.jpg"}).To_InputFile()
}

type fakeUsers struct {
	contact bool
}

func (f *fakeUsers) UserCheckContact(context.Context, *userpb.TLUserCheckContact) (*mtproto.Bool, error) {
	return mtproto.ToBool(f.contact), nil
}

func (f *fakeUsers) UserGetImmutableUser(_ context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: in.GetId(), Bot: &mtproto.BotData{}}}, nil
}

type fakeMsg struct {
	sent *msgpb.TLMsgSendMessageV2
}

func (f *fakeMsg) MsgSendMessageV2(_ context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	f.sent = in
	return mtproto.MakeEmptyUpdates(), nil
}

type fakeMedia struct {
	photo *mtproto.Photo
}

func (f *fakeMedia) MediaUploadProfilePhotoFile(context.Context, *mediapb.TLMediaUploadProfilePhotoFile) (*mtproto.Photo, error) {
	return f.photo, nil
}
