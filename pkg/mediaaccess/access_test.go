package mediaaccess

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestLogOnly(t *testing.T) {
	t.Setenv(EnvEnforce, "")
	if LogOnly() {
		t.Fatal("empty env enforces")
	}
	for _, v := range []string{"enforce", "on", "1", "true", "ENFORCE", "nope"} {
		t.Setenv(EnvEnforce, v)
		if LogOnly() {
			t.Fatalf("%q is log mode", v)
		}
	}
	t.Setenv(EnvEnforce, "log")
	if !LogOnly() {
		t.Fatal("log")
	}
	t.Setenv(EnvEnforce, "LOG")
	if !LogOnly() {
		t.Fatal("LOG")
	}
}

func TestGate(t *testing.T) {
	t.Setenv(EnvEnforce, "enforce")
	if err := Gate(nil, mtproto.ErrFileIdInvalid); err != mtproto.ErrFileIdInvalid {
		t.Fatalf("enforce %v", err)
	}
	if err := Gate(nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvEnforce, "log")
	if err := Gate(nil, mtproto.ErrFileIdInvalid); err != nil {
		t.Fatalf("log mode returned %v", err)
	}
}

func TestPhotoAndDocumentDownload(t *testing.T) {
	if err := PhotoDownload(9, 9, nil, false, 0, nil, true); err != nil {
		t.Fatalf("web photo %v", err)
	}
	if err := PhotoDownload(9, 1, nil, false, 0, nil, true); err != mtproto.ErrFileIdInvalid {
		t.Fatalf("web photo bad hash %v", err)
	}
	if err := PhotoDownload(0, 1, nil, true, 1, nil, false); err != mtproto.ErrLocationInvalid {
		t.Fatalf("zero id %v", err)
	}
	if err := PhotoDownload(4, 7, nil, true, 7, nil, false); err != nil {
		t.Fatalf("photo %v", err)
	}
	if err := PhotoDownload(4, 1, nil, true, 7, nil, false); err != mtproto.ErrFileIdInvalid {
		t.Fatalf("photo bad hash %v", err)
	}
	if err := PhotoDownload(4, 0, nil, true, 7, nil, false); err != mtproto.ErrFileIdInvalid {
		t.Fatalf("photo zero hash %v", err)
	}
	if err := PhotoDownload(4, 7, nil, false, 0, nil, false); err != mtproto.ErrFileIdInvalid {
		t.Fatalf("missing photo %v", err)
	}
	ref := []byte{1, 2, 3}
	if err := PhotoDownload(4, 7, ref, true, 7, ref, false); err != nil {
		t.Fatalf("reference %v", err)
	}
	if err := PhotoDownload(4, 7, nil, true, 7, ref, false); err != mtproto.ErrFileReferenceExpired {
		t.Fatalf("expired reference %v", err)
	}
	if err := DocumentDownload(8, 3, nil, true, 3, nil); err != nil {
		t.Fatalf("document %v", err)
	}
	if err := DocumentDownload(8, 1, nil, true, 3, nil); err != mtproto.ErrFileIdInvalid {
		t.Fatalf("document bad hash %v", err)
	}
	if err := EncryptedDownload(5, 9, 9, true); err != nil {
		t.Fatal(err)
	}
	if err := EncryptedDownload(5, 1, 9, true); err != mtproto.ErrFileIdInvalid {
		t.Fatalf("encrypted %v", err)
	}
}

func TestPeerPhotos(t *testing.T) {
	if err := ProfilePhoto(3, []int64{2, 3}, true); err != nil {
		t.Fatal(err)
	}
	if err := ProfilePhoto(9, []int64{2, 3}, true); err != mtproto.ErrLocationInvalid {
		t.Fatalf("foreign photo %v", err)
	}
	if err := ProfilePhoto(3, []int64{3}, false); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("hidden peer %v", err)
	}
	if err := ChatPhoto(4, 4, true); err != nil {
		t.Fatal(err)
	}
	if err := ChatPhoto(4, 4, false); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("non member %v", err)
	}
	if err := ChatPhoto(9, 4, true); err != mtproto.ErrLocationInvalid {
		t.Fatalf("chat photo %v", err)
	}
	if err := ChannelPhoto(6, 6, 11, 11, true); err != nil {
		t.Fatal(err)
	}
	if err := ChannelPhoto(6, 6, 1, 11, true); err != mtproto.ErrChannelInvalid {
		t.Fatalf("channel hash %v", err)
	}
	if err := ChannelPhoto(9, 6, 11, 11, true); err != mtproto.ErrLocationInvalid {
		t.Fatalf("channel photo %v", err)
	}
}

func TestMismatchAllowed(t *testing.T) {
	t.Setenv(EnvEnforce, "enforce")
	if MismatchAllowed("inputPhoto", 4) {
		t.Fatal("enforce allowed a mismatch")
	}
	t.Setenv(EnvEnforce, "log")
	if !MismatchAllowed("inputPhoto", 4) {
		t.Fatal("log mode rejected")
	}
}
