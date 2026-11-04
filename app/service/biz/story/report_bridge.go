package story

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/report"
)

// fileReport writes stories.report into the unified reports table.
// Option text stays in Report; this only stores a chosen option.
func (s *Service) fileReport(ctx context.Context, viewer int64, owner Owner, ids []int32, option, message, lang string) error {
	peerType := int32(mtproto.PEER_USER)
	var subject int64
	if owner.Kind == KindUser {
		subject = owner.ID
	} else {
		peerType = mtproto.PEER_CHANNEL
	}
	var items []report.Content
	for _, id := range ids {
		rec, err := s.store.Get(ctx, owner, id)
		if err != nil {
			return err
		}
		if rec == nil || rec.Item == nil {
			continue
		}
		items = append(items, report.ContentFromStory(rec.Item))
		if subject == 0 && rec.Sender != 0 {
			subject = rec.Sender
		}
	}
	_, err := report.Shared().Submit(ctx, report.Submission{
		Source:        report.SourceStory,
		Reporter:      viewer,
		Lang:          lang,
		PeerType:      peerType,
		PeerID:        owner.ID,
		SubjectUserID: subject,
		Reason:        option,
		Message:       message,
		ObjectIDs:     ids,
		Evidence:      report.Evidence{Items: items},
	})
	return err
}
