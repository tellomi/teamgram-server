package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/autodownload/presets"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestAutoDownloadSaveRoundTrip(t *testing.T) {
	presets.Install(presets.New(nil))
	t.Cleanup(func() { presets.Install(presets.New(nil)) })
	c := &AutoDownloadCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 7},
	}
	before, err := c.AccountGetAutoDownloadSettings(&mtproto.TLAccountGetAutoDownloadSettings{})
	if err != nil || before.GetLow().GetPhotoSizeMax() != 1048576 || before.GetMedium().GetVideoSizeMax_INT64() != 10485760 {
		t.Fatalf("defaults %+v %v", before, err)
	}
	_, err = c.AccountSaveAutoDownloadSettings(&mtproto.TLAccountSaveAutoDownloadSettings{
		Low: true,
		Settings: mtproto.MakeTLAutoDownloadSettings(&mtproto.AutoDownloadSettings{
			Disabled:           true,
			PhotoSizeMax:       42,
			VideoSizeMax_INT64: 99,
			FileSizeMax_INT64:  7,
			StoriesPreload:     true,
		}).To_AutoDownloadSettings(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.AccountGetAutoDownloadSettings(&mtproto.TLAccountGetAutoDownloadSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.GetLow().GetDisabled() || got.GetLow().GetPhotoSizeMax() != 42 || got.GetLow().GetVideoSizeMax_INT64() != 99 || !got.GetLow().GetStoriesPreload() {
		t.Fatalf("low %+v", got.GetLow())
	}
	if got.GetMedium().GetPhotoSizeMax() != 1048576 || got.GetHigh().GetFileSizeMax_INT64() != 3145728 {
		t.Fatalf("other presets changed medium %d high %d", got.GetMedium().GetPhotoSizeMax(), got.GetHigh().GetFileSizeMax_INT64())
	}

	other := &AutoDownloadCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 8},
	}
	fresh, err := other.AccountGetAutoDownloadSettings(&mtproto.TLAccountGetAutoDownloadSettings{})
	if err != nil || fresh.GetLow().GetPhotoSizeMax() != 1048576 || fresh.GetLow().GetDisabled() {
		t.Fatalf("user 8 saw user 7 %+v %v", fresh.GetLow(), err)
	}
	if _, err := c.AccountSaveAutoDownloadSettings(&mtproto.TLAccountSaveAutoDownloadSettings{High: true}); err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("nil settings: %v", err)
	}
}
