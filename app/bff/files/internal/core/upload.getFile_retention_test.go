package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/media/retention"
)

type expiredSet map[int64]bool

func (e expiredSet) ExpiredAt(_ context.Context, _ retention.Kind, id int64) (int64, error) {
	if e[id] {
		return 1, nil
	}
	return 0, nil
}

func TestGetFileExpiredMedia(t *testing.T) {
	t.Setenv("TELLOMI_CDN_PUBLIC_KEYS", "")
	downloads := 0
	downloadFile = func(_ context.Context, _ *mtproto.InputFileLocation, _ int64, _ int32) (*mtproto.Upload_File, error) {
		downloads++
		return mtproto.MakeTLUploadFile(&mtproto.Upload_File{Bytes: []byte("x")}).To_Upload_File(), nil
	}
	t.Cleanup(func() { downloadFile = nil })
	c := fileCore()
	c.svcCtx = &svc.ServiceContext{Retention: retention.NewChecker(expiredSet{7: true}, 0)}

	photo := func(id int64, thumb string) *mtproto.InputFileLocation {
		return mtproto.MakeTLInputPhotoFileLocation(&mtproto.InputFileLocation{Id: id, AccessHash: 1, ThumbSize: thumb}).To_InputFileLocation()
	}
	thumbDoc := docLoc(7)
	thumbDoc.ThumbSize = "m"
	cases := []struct {
		name    string
		loc     *mtproto.InputFileLocation
		expired bool
	}{
		{"expired document", docLoc(7), true},
		{"expired document thumbnail", thumbDoc, false},
		{"live document", docLoc(8), false},
		{"expired photo full size", photo(7, "x"), true},
		{"expired photo original", photo(7, "0"), true},
		{"expired photo small thumb", photo(7, "s"), false},
		{"expired photo stripped", photo(7, "i"), false},
		{"live photo", photo(8, "x"), false},
	}
	for _, tc := range cases {
		before := downloads
		_, err := c.UploadGetFile(&mtproto.TLUploadGetFile{Location: tc.loc, Limit: 1024})
		if tc.expired {
			if err != mtproto.ErrFileIdInvalid {
				t.Errorf("%s: err %v, want FILE_ID_INVALID", tc.name, err)
			}
			if downloads != before {
				t.Errorf("%s: dfs was called", tc.name)
			}
		} else if err != nil || downloads != before+1 {
			t.Errorf("%s: err %v downloads %d", tc.name, err, downloads-before)
		}
	}
}
