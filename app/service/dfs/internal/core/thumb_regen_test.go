package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
)

func TestResolveThumbDownload(t *testing.T) {
	got, err := resolveThumbDownload(0, 32, []byte("jpeg"), nil, nil)
	if err != nil || string(got) != "jpeg" {
		t.Fatalf("hit %q %v", got, err)
	}
	got, err = resolveThumbDownload(0, 0, nil, nil, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("zero limit %q %v", got, err)
	}
	got, err = resolveThumbDownload(4, 8, nil, nil, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("tail %q %v", got, err)
	}

	netErr := errors.New("r2 timeout")
	if _, err = resolveThumbDownload(0, 32, nil, netErr, func() ([]byte, error) {
		t.Fatal("regen on a transport error")
		return nil, nil
	}); err != netErr {
		t.Fatalf("transport %v", err)
	}

	got, err = resolveThumbDownload(0, 32, nil, minio_util.ErrNotFound, func() ([]byte, error) {
		return []byte("rebuilt-jpeg-bytes"), nil
	})
	if err != nil || string(got) != "rebuilt-jpeg-bytes" {
		t.Fatalf("regen %q %v", got, err)
	}
	got, err = resolveThumbDownload(8, 4, nil, minio_util.ErrNotFound, func() ([]byte, error) {
		return []byte("rebuilt-jpeg-bytes"), nil
	})
	if err != nil || string(got) != "jpeg" {
		t.Fatalf("regen range %q %v", got, err)
	}

	if _, err = resolveThumbDownload(0, 32, []byte{}, nil, func() ([]byte, error) {
		return nil, errors.New("no ffmpeg")
	}); err != mtproto.ErrFileIdInvalid {
		t.Fatalf("empty success %v", err)
	}
	if _, err = resolveThumbDownload(0, 32, nil, minio_util.ErrNotFound, nil); err != mtproto.ErrFileIdInvalid {
		t.Fatalf("video thumb %v", err)
	}
}
