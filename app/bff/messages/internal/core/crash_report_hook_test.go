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

func TestNoteCrashReportCapturesIOSZipShape(t *testing.T) {
	var got crashreport.Incoming
	var calls int
	prev := saveCrash
	saveCrash = func(_ context.Context, in crashreport.Incoming) error {
		calls++
		got = in
		return nil
	}
	t.Cleanup(func() { saveCrash = prev })

	const (
		name    = "Tellomi-iOS-14.0.3-crash.zip"
		caption = "app=14.0.3 device=iPhone15,2"
	)
	doc := mtproto.MakeTLDocument(&mtproto.Document{
		Id:       55,
		MimeType: "application/zip",
		Attributes: []*mtproto.DocumentAttribute{
			mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: name}).To_DocumentAttribute(),
		},
	}).To_Document()
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      7,
		Date:    1700000000,
		Message: caption,
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Document: doc,
		}).To_MessageMedia(),
	}).To_Message()
	noteCrashReports(context.Background(), 9, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: crashreport.AccountID}, msg, nil)
	if calls != 1 || got.SenderID != 9 || got.MsgID != 7 || got.DocumentID != 55 || got.Filename != name || got.Mime != "application/zip" || got.Text != caption {
		t.Fatalf("calls %d %+v", calls, got)
	}
}

func TestAlbumSendsAreCapturedEvenWhenStoreFails(t *testing.T) {
	var ids []int32
	prev := saveCrash
	saveCrash = func(_ context.Context, in crashreport.Incoming) error {
		ids = append(ids, in.MsgID)
		if in.MsgID == 1 {
			return errCrashStore
		}
		return nil
	}
	t.Cleanup(func() { saveCrash = prev })
	sent := &mtproto.Updates{Updates: []*mtproto.Update{
		{Message_MESSAGE: &mtproto.Message{Id: 1, Message: "a.zip"}},
		{Message_MESSAGE: &mtproto.Message{Id: 2, Message: "b.zip"}},
	}}
	noteCrashReports(context.Background(), 9, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: crashreport.AccountID}, nil, sent)
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Fatalf("album ids %v", ids)
	}
}

var errCrashStore = errString("store failed")

type errString string

func (e errString) Error() string { return string(e) }
