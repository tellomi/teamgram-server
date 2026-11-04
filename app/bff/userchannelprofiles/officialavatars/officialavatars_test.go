package officialavatars

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"image/jpeg"
	"testing"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	dfspb "github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
)

type fake struct {
	parts   map[string][][]byte // creator_fileid -> parts
	photos  map[int64][]int64
	uploads []*mediapb.TLMediaUploadProfilePhotoFile
	nextID  int64
	failMed bool
}

func newFake() *fake { return &fake{parts: map[string][][]byte{}, photos: map[int64][]int64{}} }

func key(c, f int64) string { return fmt.Sprintf("%d_%d", c, f) }

func (f *fake) DfsWriteFilePartData(_ context.Context, in *dfspb.TLDfsWriteFilePartData) (*mtproto.Bool, error) {
	k := key(in.Creator, in.FileId)
	if int(in.FilePart) != len(f.parts[k]) {
		return nil, errors.New("part out of order")
	}
	f.parts[k] = append(f.parts[k], append([]byte(nil), in.Bytes...))
	return mtproto.BoolTrue, nil
}

func (f *fake) assemble(t *testing.T, owner int64, in *mtproto.InputFile) []byte {
	t.Helper()
	ps := f.parts[key(owner, in.GetId_INT64())]
	if len(ps) != int(in.GetParts()) {
		t.Fatalf("parts %d, want %d", len(ps), in.GetParts())
	}
	b := bytes.Join(ps, nil)
	if fmt.Sprintf("%x", md5.Sum(b)) != in.GetMd5Checksum() {
		t.Fatalf("md5 mismatch")
	}
	return b
}

func (f *fake) MediaUploadProfilePhotoFile(_ context.Context, in *mediapb.TLMediaUploadProfilePhotoFile) (*mtproto.Photo, error) {
	if f.failMed {
		return nil, errors.New("boom")
	}
	f.uploads = append(f.uploads, in)
	f.nextID++
	p := &mtproto.Photo{Id: 1000 + f.nextID, Sizes: []*mtproto.PhotoSize{{Type: "a"}}}
	if in.Video != nil {
		p.VideoSizes = []*mtproto.VideoSize{{Type: "u", W: 800, H: 800}}
	}
	return mtproto.MakeTLPhoto(p).To_Photo(), nil
}

func (f *fake) UserGetProfilePhotos(_ context.Context, in *userpb.TLUserGetProfilePhotos) (*userpb.Vector_Long, error) {
	return &userpb.Vector_Long{Datas: f.photos[in.UserId]}, nil
}

func (f *fake) UserUpdateProfilePhoto(_ context.Context, in *userpb.TLUserUpdateProfilePhoto) (*mtproto.Int64, error) {
	f.photos[in.UserId] = append([]int64{in.Id}, f.photos[in.UserId]...)
	return &mtproto.Int64{V: in.Id}, nil
}

func seeder(f *fake) *Seeder {
	n := int64(0)
	return &Seeder{Dfs: f, Media: f, User: f, NewFileID: func() int64 { n++; return n }}
}

func TestAssetsAreValid(t *testing.T) {
	for _, a := range Avatars {
		b, err := Asset(a.Photo)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s: %v", a.Photo, err)
		}
		if cfg.Width != cfg.Height || cfg.Width < 640 {
			t.Fatalf("%s: %dx%d, want square >= 640", a.Photo, cfg.Width, cfg.Height)
		}
		if a.Video != "" {
			v, err := Asset(a.Video)
			if err != nil || len(v) < 12 || string(v[4:8]) != "ftyp" {
				t.Fatalf("%s is not an mp4: %v", a.Video, err)
			}
		}
	}
	if len(Avatars) != 6 || Find(777001) == nil || Find(777001).Video == "" || Find(1) != nil {
		t.Fatal("unexpected avatar table")
	}
}

func TestRunUploadsAllAndIsIdempotent(t *testing.T) {
	f := newFake()
	s := seeder(f)
	res, err := s.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 6 || len(f.uploads) != 6 {
		t.Fatalf("results %d uploads %d", len(res), len(f.uploads))
	}
	for i, u := range f.uploads {
		a := Avatars[i]
		if u.OwnerId != a.UserID {
			t.Fatalf("owner %d", u.OwnerId)
		}
		want, _ := Asset(a.Photo)
		if !bytes.Equal(f.assemble(t, u.OwnerId, u.File), want) {
			t.Fatalf("%d photo bytes differ", a.UserID)
		}
		if (u.Video != nil) != (a.Video != "") || res[i].Video != (a.Video != "") {
			t.Fatalf("%d video mismatch", a.UserID)
		}
		if u.Video != nil {
			wantV, _ := Asset(a.Video)
			if !bytes.Equal(f.assemble(t, u.OwnerId, u.Video), wantV) {
				t.Fatal("video bytes differ")
			}
		}
		if f.photos[a.UserID][0] != res[i].PhotoID || res[i].Skipped {
			t.Fatalf("%d photo id not set", a.UserID)
		}
	}
	res, err = s.Run(context.Background(), nil)
	if err != nil || len(f.uploads) != 6 {
		t.Fatalf("second run uploaded again: %v %d", err, len(f.uploads))
	}
	for _, r := range res {
		if !r.Skipped {
			t.Fatalf("%d not skipped", r.UserID)
		}
	}
}

func TestForceAndSelection(t *testing.T) {
	f := newFake()
	f.photos[777001] = []int64{5}
	s := seeder(f)
	s.Force = true
	res, err := s.Run(context.Background(), []int64{777001})
	if err != nil || len(res) != 1 || len(f.uploads) != 1 || f.photos[777001][0] == 5 {
		t.Fatalf("force: %v %+v", err, res)
	}
	if _, err = s.Run(context.Background(), []int64{42}); err == nil {
		t.Fatal("want error for non-official id")
	}
}

func TestMultiPartAndErrors(t *testing.T) {
	f := newFake()
	s := seeder(f)
	in, err := s.upload(context.Background(), 7, "assets/777001.mp4", "a.mp4")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Asset("assets/777001.mp4")
	if int(in.Parts) != (len(b)+PartSize-1)/PartSize {
		t.Fatalf("parts %d", in.Parts)
	}
	f.assemble(t, 7, in)
	f.failMed = true
	if _, err = s.Run(context.Background(), []int64{777000}); err == nil {
		t.Fatal("want media error")
	}
	if len(f.photos[777000]) != 0 {
		t.Fatal("photo set after failure")
	}
}
