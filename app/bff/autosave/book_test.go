package autosave

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestAutoSave(t *testing.T) {
	Reset()
	if _, err := Get(0); status.Convert(err).Message() != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
	got, err := Get(7)
	if err != nil || !got.GetUsersSettings().GetPhotos() || !got.GetUsersSettings().GetVideos() {
		t.Fatalf("default: %v %#v", err, got)
	}
	if len(got.GetExceptions()) != 0 {
		t.Fatalf("exceptions %#v", got.GetExceptions())
	}
	off := mtproto.MakeTLAutoSaveSettings(&mtproto.AutoSaveSettings{}).To_AutoSaveSettings()
	if _, err := Save(7, &mtproto.TLAccountSaveAutoSaveSettings{Settings: off}); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatal(err)
	}
	if _, err := Save(7, &mtproto.TLAccountSaveAutoSaveSettings{
		Users: true, Settings: off,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = Get(7)
	if err != nil || got.GetUsersSettings().GetPhotos() || !got.GetChatsSettings().GetPhotos() {
		t.Fatalf("users off: %#v %v", got, err)
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer()
	capped := mtproto.MakeTLAutoSaveSettings(&mtproto.AutoSaveSettings{
		Photos: true, Videos: true, VideoMaxSize: wrapperspb.Int64(1024),
	}).To_AutoSaveSettings()
	if _, err := Save(7, &mtproto.TLAccountSaveAutoSaveSettings{Peer: peer, Settings: capped}); err != nil {
		t.Fatal(err)
	}
	got, err = Get(7)
	if err != nil || len(got.GetExceptions()) != 1 || got.GetExceptions()[0].GetSettings().GetVideoMaxSize().GetValue() != 1024 {
		t.Fatalf("exception: %#v %v", got, err)
	}
	if len(got.GetUsers()) != 1 || got.GetUsers()[0].GetId() != 9 {
		t.Fatalf("users %#v", got.GetUsers())
	}
	bad := mtproto.MakeTLAutoSaveSettings(&mtproto.AutoSaveSettings{VideoMaxSize: wrapperspb.Int64(-1)}).To_AutoSaveSettings()
	if _, err := Save(7, &mtproto.TLAccountSaveAutoSaveSettings{Chats: true, Settings: bad}); status.Convert(err).Message() != "MEDIA_INVALID" {
		t.Fatal(err)
	}
	empty := mtproto.MakeTLInputPeerEmpty(&mtproto.InputPeer{}).To_InputPeer()
	if _, err := Save(7, &mtproto.TLAccountSaveAutoSaveSettings{Peer: empty, Settings: off}); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatal(err)
	}
	if _, err := DeleteExceptions(7); err != nil {
		t.Fatal(err)
	}
	got, err = Get(7)
	if err != nil || len(got.GetExceptions()) != 0 || got.GetUsersSettings().GetPhotos() {
		t.Fatalf("cleared %#v %v", got, err)
	}
}
