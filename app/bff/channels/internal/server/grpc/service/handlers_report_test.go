package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/phone/secretchat"
	"github.com/teamgram/teamgram-server/app/service/biz/report"
	"google.golang.org/grpc/metadata"
)

func TestEncryptedSpamFilesReport(t *testing.T) {
	secretchat.Reset()
	t.Cleanup(secretchat.Reset)
	report.ResetShared()
	chat, _, err := secretchat.Default().Request(7, 8, 42, []byte{1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	peer := mtproto.MakeTLInputEncryptedChat(&mtproto.InputEncryptedChat{
		ChatId: chat.GetId(), AccessHash: chat.GetAccessHash(),
	}).To_InputEncryptedChat()
	out, err := rpcmd.RpcMetadataToOutgoing(context.Background(), &rpcmd.RpcMetadata{UserId: 7, PermAuthKeyId: 9, LangCode: "zh-hans"})
	if err != nil {
		t.Fatal(err)
	}
	md, _ := metadata.FromOutgoingContext(out)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	ok, err := (ReportsService{}).MessagesReportEncryptedSpam(ctx, &mtproto.TLMessagesReportEncryptedSpam{Peer: peer})
	if err != nil || ok == nil || !mtproto.FromBool(ok) {
		t.Fatalf("%v %v", ok, err)
	}
	rows, err := report.Shared().List(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Source != report.SourceEncrypted || rows[0].Reporter != 7 || rows[0].SubjectUserID != 8 || rows[0].Reason != "spam" {
		t.Fatalf("%+v %v", rows, err)
	}
	if rows[0].TargetKey != "encrypted:42" || rows[0].Lang != "zh-hans" {
		t.Fatalf("%+v", rows[0])
	}
	ok, err = (ReportsService{}).MessagesReportEncryptedSpam(ctx, &mtproto.TLMessagesReportEncryptedSpam{Peer: peer})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("retry %v %v", ok, err)
	}
	rows, err = report.Shared().List(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatalf("dup %+v %v", rows, err)
	}
}
