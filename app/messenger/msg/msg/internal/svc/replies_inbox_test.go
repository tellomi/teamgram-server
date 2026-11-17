package svc

import (
	"context"
	"testing"

	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

func TestRepliesPrivacyMissingClientFailsClosed(t *testing.T) {
	_, _, err := repliesPrivacy(context.Background(), nil, channel.RepliesNotice{Recipient: 2, Commenter: 3})
	if err == nil {
		t.Fatal("nil client treated as not blocked")
	}
}
