// Package officialavatars gives the seeded official accounts (777000 to
// 777005, see teamgramd/deploy/sql/migrate-20261105-official-accounts.sql)
// their profile photos.
//
// It uses the same path as photos.uploadProfilePhoto: the bytes are written
// as upload parts with dfs.writeFilePartData, media.uploadProfilePhotoFile
// builds the photo sizes (and the video size for an animated avatar) in
// object storage and the media tables, and user.updateProfilePhoto sets
// users.photo_id and user_profile_photos through the cached user DAO.
// Clients then download the sizes with upload.getFile like any other avatar.
package officialavatars

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"embed"
	"encoding/binary"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	dfspb "github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

//go:embed assets/*
var assetFS embed.FS

// PartSize is the upload part size. It matches what clients send (512 KB).
const PartSize = 512 * 1024

// Avatar is one official account and its embedded files.
type Avatar struct {
	UserID int64
	Name   string
	Photo  string // JPEG asset path, 640x640
	Video  string // optional MP4 asset path, 800x800 (animated avatar)
}

// Avatars lists every seeded official account.
var Avatars = []Avatar{
	{UserID: 777000, Name: "Tellomi (@admin)", Photo: "assets/777000.jpg"},
	{UserID: 777001, Name: "Tellomi AI (@tellomi)", Photo: "assets/777001.jpg", Video: "assets/777001.mp4"},
	{UserID: 777002, Name: "Tellomi (@tellomi_official)", Photo: "assets/777002.jpg"},
	{UserID: 777003, Name: "Tellomi Team (@tellomi_team)", Photo: "assets/777003.jpg"},
	{UserID: 777004, Name: "Tellomi Support (@support)", Photo: "assets/777004.jpg"},
	{UserID: 777005, Name: "发现 (@faxian)", Photo: "assets/777005.jpg"},
}

// Dfs, Media and User are the slices of the service clients this package uses.
type Dfs interface {
	DfsWriteFilePartData(ctx context.Context, in *dfspb.TLDfsWriteFilePartData) (*mtproto.Bool, error)
}

type Media interface {
	MediaUploadProfilePhotoFile(ctx context.Context, in *mediapb.TLMediaUploadProfilePhotoFile) (*mtproto.Photo, error)
}

type User interface {
	UserGetProfilePhotos(ctx context.Context, in *userpb.TLUserGetProfilePhotos) (*userpb.Vector_Long, error)
	UserUpdateProfilePhoto(ctx context.Context, in *userpb.TLUserUpdateProfilePhoto) (*mtproto.Int64, error)
}

// Seeder uploads the avatars.
type Seeder struct {
	Dfs   Dfs
	Media Media
	User  User
	// Force uploads a new photo even when the account already has one.
	Force bool
	// NewFileID returns upload file ids. Defaults to a random positive int64.
	NewFileID func() int64
	Logf      func(format string, args ...any)
}

// Result is what happened to one account.
type Result struct {
	UserID  int64
	PhotoID int64
	Skipped bool
	Video   bool
}

// Run seeds the selected accounts (all when ids is empty). It is safe to run
// again: an account that already has a profile photo is skipped unless Force.
func (s *Seeder) Run(ctx context.Context, ids []int64) ([]Result, error) {
	want := map[int64]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for id := range want {
		if Find(id) == nil {
			return nil, fmt.Errorf("user %d is not an official account", id)
		}
	}
	var out []Result
	for _, a := range Avatars {
		if len(want) > 0 && !want[a.UserID] {
			continue
		}
		r, err := s.seedOne(ctx, a)
		if err != nil {
			return out, fmt.Errorf("user %d: %w", a.UserID, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// Find returns the avatar for a user id, or nil.
func Find(id int64) *Avatar {
	for i := range Avatars {
		if Avatars[i].UserID == id {
			return &Avatars[i]
		}
	}
	return nil
}

func (s *Seeder) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
	}
}

func (s *Seeder) fileID() int64 {
	if s.NewFileID != nil {
		return s.NewFileID()
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	return int64(binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff)
}

func (s *Seeder) seedOne(ctx context.Context, a Avatar) (Result, error) {
	res := Result{UserID: a.UserID}
	if !s.Force {
		ph, err := s.User.UserGetProfilePhotos(ctx, &userpb.TLUserGetProfilePhotos{UserId: a.UserID})
		if err != nil {
			return res, fmt.Errorf("user.getProfilePhotos: %w", err)
		}
		if len(ph.GetDatas()) > 0 {
			res.Skipped = true
			res.PhotoID = ph.GetDatas()[0]
			s.logf("%d %s: already has photo %d, skipped", a.UserID, a.Name, res.PhotoID)
			return res, nil
		}
	}

	// The account id is the upload owner, like PermAuthKeyId for a client.
	owner := a.UserID
	photo, err := s.upload(ctx, owner, a.Photo, "avatar.jpg")
	if err != nil {
		return res, err
	}
	req := &mediapb.TLMediaUploadProfilePhotoFile{OwnerId: owner, File: photo}
	if a.Video != "" {
		video, err := s.upload(ctx, owner, a.Video, "avatar.mp4")
		if err != nil {
			return res, err
		}
		req.Video = video
		req.VideoStartTs = &wrapperspb.DoubleValue{Value: 0}
		res.Video = true
	}
	p, err := s.Media.MediaUploadProfilePhotoFile(ctx, req)
	if err != nil {
		return res, fmt.Errorf("media.uploadProfilePhotoFile: %w", err)
	}
	if p.GetId() == 0 {
		return res, fmt.Errorf("media.uploadProfilePhotoFile returned no photo id")
	}
	if a.Video != "" && len(p.GetVideoSizes()) == 0 {
		return res, fmt.Errorf("photo %d has no video size", p.GetId())
	}
	if _, err = s.User.UserUpdateProfilePhoto(ctx, &userpb.TLUserUpdateProfilePhoto{UserId: a.UserID, Id: p.GetId()}); err != nil {
		return res, fmt.Errorf("user.updateProfilePhoto: %w", err)
	}
	res.PhotoID = p.GetId()
	s.logf("%d %s: photo %d (%d sizes, %d video sizes)", a.UserID, a.Name, p.GetId(), len(p.GetSizes()), len(p.GetVideoSizes()))
	return res, nil
}

// upload writes one embedded asset as upload parts and returns its InputFile.
func (s *Seeder) upload(ctx context.Context, owner int64, path, name string) (*mtproto.InputFile, error) {
	body, err := assetFS.ReadFile(path)
	if err != nil {
		return nil, err
	}
	id := s.fileID()
	parts := 0
	for off := 0; off < len(body); off += PartSize {
		end := off + PartSize
		if end > len(body) {
			end = len(body)
		}
		if _, err = s.Dfs.DfsWriteFilePartData(ctx, &dfspb.TLDfsWriteFilePartData{
			Creator:  owner,
			FileId:   id,
			FilePart: int32(parts),
			Bytes:    body[off:end],
		}); err != nil {
			return nil, fmt.Errorf("dfs.writeFilePartData %s part %d: %w", path, parts, err)
		}
		parts++
	}
	return mtproto.MakeTLInputFile(&mtproto.InputFile{
		Id_INT64:    id,
		Parts:       int32(parts),
		Name:        name,
		Md5Checksum: fmt.Sprintf("%x", md5.Sum(body)),
	}).To_InputFile(), nil
}

// Asset returns the bytes of an embedded asset.
func Asset(path string) ([]byte, error) { return assetFS.ReadFile(path) }
