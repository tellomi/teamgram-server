package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/crashreport"
)

func TestNoteCrashReportCapturesBotPeer(t *testing.T) {
	var got crashreport.Incoming
	var calls int
	prev := saveCrash
	saveCrash = func(_ context.Context, in crashreport.Incoming) error {
		calls++
		got = in
		return nil
	}
	t.Cleanup(func() { saveCrash = prev })

	doc := mtproto.MakeTLDocument(&mtproto.Document{
		Id:       55,
		MimeType: "application/zip",
		Attributes: []*mtproto.DocumentAttribute{
			mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: "ios-crash.zip"}).To_DocumentAttribute(),
		},
	}).To_Document()
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      7,
		Date:    1700000000,
		Message: "app=14.0.1",
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Document: doc,
		}).To_MessageMedia(),
	}).To_Message()
	noteCrashReports(context.Background(), 9, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: crashreport.AccountID}, msg, nil)
	if calls != 1 || got.SenderID != 9 || got.MsgID != 7 || got.DocumentID != 55 || got.Filename != "ios-crash.zip" || got.Text != "app=14.0.1" {
		t.Fatalf("calls %d %+v", calls, got)
	}
	noteCrashReports(context.Background(), 9, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: 8}, msg, nil)
	if calls != 1 {
		t.Fatalf("captured a message that was not sent to the bot: %d", calls)
	}
}
