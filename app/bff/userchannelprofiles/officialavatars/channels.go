package officialavatars

import (
	"context"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
)

// ChannelAvatar is one official broadcast channel and its embedded photo.
type ChannelAvatar struct {
	ChannelID int64
	Name      string
	Photo     string // JPEG asset path, 640x640
}

// ChannelOwner is the account that creates and administers the official
// channels (migrate-20261105-official-accounts.sql). Uploads and the
// "channel photo updated" service message are made as this account.
const ChannelOwner int64 = 777002

// ChannelAvatars lists every seeded official channel.
var ChannelAvatars = []ChannelAvatar{
	{ChannelID: 1073800001, Name: "官方公告 (@news)", Photo: "assets/1073800001.jpg"},
	{ChannelID: 1073800002, Name: "更新日志 (@updates)", Photo: "assets/1073800002.jpg"},
	{ChannelID: 1073800003, Name: "安全中心 (@security)", Photo: "assets/1073800003.jpg"},
	{ChannelID: 1073800004, Name: "开发者 (@dev)", Photo: "assets/1073800004.jpg"},
}

// Channels reads and sets a channel photo. The command implements it with
// the MySQL channel service, i.e. the same ApplyPhoto channels.editPhoto uses.
type Channels interface {
	ChannelPhotoID(ctx context.Context, channelID int64) (int64, error)
	ApplyChannelPhoto(ctx context.Context, actor, channelID int64, photo *mtproto.Photo) error
}

// FindChannel returns the avatar for a channel id, or nil.
func FindChannel(id int64) *ChannelAvatar {
	for i := range ChannelAvatars {
		if ChannelAvatars[i].ChannelID == id {
			return &ChannelAvatars[i]
		}
	}
	return nil
}

// ChannelResult is what happened to one channel.
type ChannelResult struct {
	ChannelID int64
	PhotoID   int64
	Skipped   bool
}

// RunChannels seeds the selected channels (all when ids is empty), one after
// another. A channel that already has a photo is skipped unless Force.
func (s *Seeder) RunChannels(ctx context.Context, ch Channels, ids []int64) ([]ChannelResult, error) {
	want := map[int64]bool{}
	for _, id := range ids {
		if FindChannel(id) == nil {
			return nil, fmt.Errorf("channel %d is not an official channel", id)
		}
		want[id] = true
	}
	var out []ChannelResult
	for _, a := range ChannelAvatars {
		if len(want) > 0 && !want[a.ChannelID] {
			continue
		}
		r, err := s.seedChannel(ctx, ch, a)
		if err != nil {
			return out, fmt.Errorf("channel %d: %w", a.ChannelID, err)
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *Seeder) seedChannel(ctx context.Context, ch Channels, a ChannelAvatar) (ChannelResult, error) {
	res := ChannelResult{ChannelID: a.ChannelID}
	cur, err := ch.ChannelPhotoID(ctx, a.ChannelID)
	if err != nil {
		return res, fmt.Errorf("read channel: %w", err)
	}
	if cur != 0 && !s.Force {
		res.Skipped, res.PhotoID = true, cur
		s.logf("%d %s: already has photo %d, skipped", a.ChannelID, a.Name, cur)
		return res, nil
	}
	file, err := s.upload(ctx, ChannelOwner, a.Photo, "avatar.jpg")
	if err != nil {
		return res, err
	}
	p, err := s.Media.MediaUploadProfilePhotoFile(ctx, &mediapb.TLMediaUploadProfilePhotoFile{OwnerId: ChannelOwner, File: file})
	if err != nil {
		return res, fmt.Errorf("media.uploadProfilePhotoFile: %w", err)
	}
	if p.GetId() == 0 || len(p.GetSizes()) == 0 {
		return res, fmt.Errorf("media.uploadProfilePhotoFile returned no photo")
	}
	if err = ch.ApplyChannelPhoto(ctx, ChannelOwner, a.ChannelID, p); err != nil {
		return res, fmt.Errorf("apply photo: %w", err)
	}
	res.PhotoID = p.GetId()
	s.logf("%d %s: photo %d (%d sizes)", a.ChannelID, a.Name, p.GetId(), len(p.GetSizes()))
	return res, nil
}
