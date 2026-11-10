package core

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/crashreport"
	"github.com/zeromicro/go-zero/core/logx"
)

// saveCrash is the collector. Tests replace it.
var saveCrash = crashreport.Capture

// noteCrashReports stores a private message sent to the crash-report bot.
// Scheduled messages are not passed here. A collector that is not installed
// returns nil and the send still succeeds.
func noteCrashReports(ctx context.Context, from int64, peer *mtproto.PeerUtil, fallback *mtproto.Message, sent *mtproto.Updates) {
	if from == 0 || from == crashreport.AccountID || peer == nil || !peer.IsUser() || peer.PeerId != crashreport.AccountID {
		return
	}
	msgs := crashMessages(fallback, sent)
	if len(msgs) == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for _, m := range msgs {
		if err := saveCrash(ctx, incomingCrash(from, m)); err != nil {
			logx.WithContext(ctx).Errorf("crash report: store failed: %v", err)
		}
	}
}

func crashMessages(fallback *mtproto.Message, sent *mtproto.Updates) []*mtproto.Message {
	var out []*mtproto.Message
	if sent != nil {
		for _, u := range sent.GetUpdates() {
			if m := u.GetMessage_MESSAGE(); m != nil && m.GetId() != 0 {
				out = append(out, m)
			}
		}
	}
	if len(out) == 0 && fallback != nil {
		if fallback.GetId() == 0 {
			if id := sentMessageID(sent); id != 0 {
				fallback.Id = id
			}
		}
		if fallback.GetId() != 0 {
			out = append(out, fallback)
		}
	}
	return out
}

func incomingCrash(from int64, m *mtproto.Message) crashreport.Incoming {
	in := crashreport.Incoming{
		SenderID: from,
		MsgID:    m.GetId(),
		When:     time.Unix(int64(m.GetDate()), 0).UTC(),
		Text:     m.GetMessage(),
	}
	if doc := m.GetMedia().GetDocument(); doc != nil && doc.GetId() != 0 {
		in.DocumentID = doc.GetId()
		in.Mime = doc.GetMimeType()
		in.Filename = documentFileName(doc)
	}
	return in
}

func documentFileName(doc *mtproto.Document) string {
	for _, attr := range doc.GetAttributes() {
		if name := attr.GetFileName(); name != "" {
			return name
		}
	}
	return ""
}
