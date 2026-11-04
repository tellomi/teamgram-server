package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestSponsoredMessagesAreEmpty(t *testing.T) {
	c := &SponsoredMessagesCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
	got, err := c.MessagesGetSponsoredMessages(&mtproto.TLMessagesGetSponsoredMessages{})
	if err != nil || got.GetPredicateName() != mtproto.Predicate_messages_sponsoredMessagesEmpty {
		t.Fatalf("get %v %#v", err, got)
	}
	ch, err := c.ChannelsGetSponsoredMessages(&mtproto.TLChannelsGetSponsoredMessages{})
	if err != nil || ch.GetPredicateName() != mtproto.Predicate_messages_sponsoredMessagesEmpty {
		t.Fatalf("channel get %v %#v", err, ch)
	}
	if ok, err := c.MessagesViewSponsoredMessage(&mtproto.TLMessagesViewSponsoredMessage{}); err != nil || !mtproto.FromBool(ok) {
		t.Fatal(err)
	}
	if ok, err := c.MessagesClickSponsoredMessage(&mtproto.TLMessagesClickSponsoredMessage{}); err != nil || !mtproto.FromBool(ok) {
		t.Fatal(err)
	}
	if ok, err := c.AccountToggleSponsoredMessages(&mtproto.TLAccountToggleSponsoredMessages{Enabled: mtproto.BoolFalse}); err != nil || !mtproto.FromBool(ok) {
		t.Fatal(err)
	}
	report, err := c.MessagesReportSponsoredMessage(&mtproto.TLMessagesReportSponsoredMessage{})
	if err != nil || report.GetPredicateName() != mtproto.Predicate_channels_sponsoredMessageReportResultReported {
		t.Fatalf("report %v %#v", err, report)
	}
}
