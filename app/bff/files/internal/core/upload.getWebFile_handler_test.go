package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/location"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestUploadGetWebFileGeo(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 9, 9, 9, 9, 9}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(png)
	}))
	defer srv.Close()
	ctx := location.WithClient(context.Background(), location.New(location.Options{
		AMapKey: "k", AMapBase: srv.URL, QPS: 100, Daily: 100, UserPerMin: 100,
	}))
	c := &FilesCore{ctx: ctx, Logger: logx.WithContext(ctx)}
	got, err := c.UploadGetWebFile(&mtproto.TLUploadGetWebFile{
		Location: mtproto.MakeTLInputWebFileGeoPointLocation(&mtproto.InputWebFileLocation{
			GeoPoint: mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9, Long: 116.4}).To_InputGeoPoint(),
			W:        100,
			H:        80,
			Zoom:     14,
			Scale:    2,
		}).To_InputWebFileLocation(),
		Offset: 2,
		Limit:  4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetMimeType() != "image/png" || got.GetSize2() != int32(len(png)) || string(got.GetBytes()) != string(png[2:6]) {
		t.Fatalf("mime %s size %d bytes %v", got.GetMimeType(), got.GetSize2(), got.GetBytes())
	}
	if got.GetFileType().GetPredicateName() != mtproto.Predicate_storage_filePng {
		t.Fatal(got.GetFileType().GetPredicateName())
	}
}

func TestUploadGetWebFileGeoNoKey(t *testing.T) {
	ctx := location.WithClient(context.Background(), location.New(location.Options{}))
	c := &FilesCore{ctx: ctx, Logger: logx.WithContext(ctx)}
	_, err := c.UploadGetWebFile(&mtproto.TLUploadGetWebFile{
		Location: mtproto.MakeTLInputWebFileGeoPointLocation(&mtproto.InputWebFileLocation{
			GeoPoint: mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9, Long: 116.4}).To_InputGeoPoint(),
			W:        10,
			H:        10,
			Zoom:     3,
			Scale:    1,
		}).To_InputWebFileLocation(),
	})
	st, ok := status.FromError(err)
	if !ok || st.Message() != "WEBFILE_NOT_AVAILABLE" {
		t.Fatalf("err %v", err)
	}
}
