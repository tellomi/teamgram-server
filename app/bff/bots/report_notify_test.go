package bots

import (
	"testing"

	"github.com/teamgram/teamgram-server/app/service/biz/report"
	"github.com/teamgram/teamgram-server/pkg/announce"
)

func TestReportMessageFromAdmin(t *testing.T) {
	if report.AdminUserID != announce.ServiceUserID {
		t.Fatalf("admin id %d", report.AdminUserID)
	}
	msg := buildReportMessage(9, "ticket", 3)
	if msg.UserId != report.AdminUserID || msg.PeerId != 9 || msg.PeerType != 2 {
		t.Fatalf("%+v", msg)
	}
	body := msg.GetMessage().GetMessage()
	if body.GetMessage() != "ticket" || body.GetFromId().GetUserId() != report.AdminUserID {
		t.Fatalf("%+v", body)
	}
}
