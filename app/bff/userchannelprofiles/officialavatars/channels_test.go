package officialavatars

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

type fakeChannels struct {
	photo    map[int64]int64
	applied  []int64
	actor    int64
	failRead bool
	failSet  bool
}

func (c *fakeChannels) ChannelPhotoID(_ context.Context, id int64) (int64, error) {
	if c.failRead {
		return 0, errors.New("read")
	}
	return c.photo[id], nil
}

func (c *fakeChannels) ApplyChannelPhoto(_ context.Context, actor, id int64, p *mtproto.Photo) error {
	if c.failSet {
		return errors.New("set")
	}
	c.actor = actor
	c.photo[id] = p.GetId()
	c.applied = append(c.applied, id)
	return nil
}

func TestChannelAssets(t *testing.T) {
	for _, a := range ChannelAvatars {
		b, err := Asset(a.Photo)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
		if err != nil || cfg.Width != 640 || cfg.Height != 640 {
			t.Fatalf("%s: %v %dx%d", a.Photo, err, cfg.Width, cfg.Height)
		}
	}
}

func TestRunChannels(t *testing.T) {
	f := newFake()
	ch := &fakeChannels{photo: map[int64]int64{1073800002: 55}}
	s := &Seeder{Dfs: f, Media: f, User: f}
	res, err := s.RunChannels(context.Background(), ch, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 4 || !res[1].Skipped || res[1].PhotoID != 55 || len(ch.applied) != 3 {
		t.Fatalf("res %+v applied %v", res, ch.applied)
	}
	if ch.actor != ChannelOwner {
		t.Fatalf("actor %d", ch.actor)
	}
	for _, u := range f.uploads {
		if u.OwnerId != ChannelOwner || u.Video != nil {
			t.Fatalf("upload %+v", u)
		}
		f.assemble(t, ChannelOwner, u.File)
	}
	// Force replaces, ids filter.
	s.Force = true
	res, err = s.RunChannels(context.Background(), ch, []int64{1073800002})
	if err != nil || len(res) != 1 || res[0].Skipped || ch.photo[1073800002] == 55 {
		t.Fatalf("force: %+v %v", res, err)
	}
}

func TestRunChannelsErrors(t *testing.T) {
	f := newFake()
	s := &Seeder{Dfs: f, Media: f, User: f}
	if _, err := s.RunChannels(context.Background(), &fakeChannels{photo: map[int64]int64{}}, []int64{1}); err == nil {
		t.Fatal("unknown id accepted")
	}
	if _, err := s.RunChannels(context.Background(), &fakeChannels{failRead: true}, nil); err == nil {
		t.Fatal("read error ignored")
	}
	if _, err := s.RunChannels(context.Background(), &fakeChannels{photo: map[int64]int64{}, failSet: true}, nil); err == nil {
		t.Fatal("set error ignored")
	}
	f.failMed = true
	if _, err := s.RunChannels(context.Background(), &fakeChannels{photo: map[int64]int64{}}, nil); err == nil {
		t.Fatal("media error ignored")
	}
	if FindChannel(1073800001) == nil || FindChannel(2) != nil {
		t.Fatal("FindChannel")
	}
}
