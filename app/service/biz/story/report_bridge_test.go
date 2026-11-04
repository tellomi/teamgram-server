package story

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/report"
)

func TestReportReachesUnifiedStore(t *testing.T) {
	report.ResetShared()
	svc := New(NewMemoryStore())
	ctx := context.Background()
	up := send(t, svc, ctx, 1, photoMedia(), rules("all"), 4201, "hello story", false)
	id := storyID(t, up)
	choose, err := svc.Report(ctx, 2, Owner{Kind: KindUser, ID: 1}, 0, []int32{id}, nil, "", "zh-hans")
	if err != nil || choose.GetTitle() != "举报动态" || choose.GetOptions()[0].GetText() != "垃圾信息" {
		t.Fatalf("localized options %+v %v", choose, err)
	}
	res, err := svc.Report(ctx, 2, Owner{Kind: KindUser, ID: 1}, 0, []int32{id}, []byte("spam"), "bad", "zh-hans")
	if err != nil || res.GetPredicateName() != mtproto.Predicate_reportResultReported {
		t.Fatalf("%+v %v", res, err)
	}
	rows, err := report.Shared().List(ctx)
	if err != nil || len(rows) != 1 || rows[0].Source != report.SourceStory || rows[0].Reporter != 2 || rows[0].SubjectUserID != 1 {
		t.Fatalf("%+v %v", rows, err)
	}
	if len(rows[0].ObjectIDs) != 1 || rows[0].ObjectIDs[0] != id || rows[0].Reason != "spam" || rows[0].Message != "bad" || rows[0].Lang != "zh-hans" {
		t.Fatalf("%+v", rows[0])
	}
}
