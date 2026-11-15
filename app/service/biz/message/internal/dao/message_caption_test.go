package dao

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
	"github.com/zeromicro/go-zero/core/jsonx"
)

func TestMakeMessageBoxRestoresCaptionFromColumn(t *testing.T) {
	body := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      4,
		Message: "",
		Media:   mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia(),
	}).To_Message()
	raw, err := jsonx.MarshalToString(body)
	if err != nil {
		t.Fatal(err)
	}
	box := (&Dao{}).MakeMessageBox(context.Background(), 7, &dataobject.MessagesDO{
		UserId:           7,
		UserMessageBoxId: 4,
		MessageData:      raw,
		Message:          "lane-photo",
	})
	if box.GetMessage().GetMessage() != "lane-photo" {
		t.Fatalf("caption %q", box.GetMessage().GetMessage())
	}

	kept := mtproto.MakeTLMessage(&mtproto.Message{Id: 5, Message: "already"}).To_Message()
	raw, err = jsonx.MarshalToString(kept)
	if err != nil {
		t.Fatal(err)
	}
	box = (&Dao{}).MakeMessageBox(context.Background(), 7, &dataobject.MessagesDO{
		UserMessageBoxId: 5,
		MessageData:      raw,
		Message:          "column",
	})
	if box.GetMessage().GetMessage() != "already" {
		t.Fatalf("json caption overwritten: %q", box.GetMessage().GetMessage())
	}
}
