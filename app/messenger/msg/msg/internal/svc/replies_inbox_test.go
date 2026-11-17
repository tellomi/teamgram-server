package svc

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestRepliesPrivacyMissingClientFailsClosed(t *testing.T) {
	_, _, err := repliesPrivacy(context.Background(), nil, channel.RepliesNotice{Recipient: 2, Commenter: 3})
	if err == nil {
		t.Fatal("nil client treated as not blocked")
	}
}

func TestRepliesPrivacyUsesBlockQuery(t *testing.T) {
	users := &privacyUsers{blocked: true}
	blocked, settings, err := repliesPrivacy(context.Background(), users, channel.RepliesNotice{
		Recipient: 2,
		Commenter: 8,
		GroupID:   40,
	})
	if err != nil || !blocked || users.blockCalls != 1 || users.blockUser != 2 || users.blockPeer != 8 {
		t.Fatalf("blocked %v calls %d user %d peer %d err %v", blocked, users.blockCalls, users.blockUser, users.blockPeer, err)
	}
	if len(settings) != 2 {
		t.Fatalf("settings %d", len(settings))
	}
	pol := channel.RepliesDeliveryOf(blocked, 100, settings...)
	if pol.Write || pol.Push {
		t.Fatalf("blocked delivery %#v", pol)
	}
}

func TestRepliesPrivacyQueryFailureDoesNotDeliver(t *testing.T) {
	users := &privacyUsers{blockErr: errors.New("user rpc down")}
	blocked, _, err := repliesPrivacy(context.Background(), users, channel.RepliesNotice{Recipient: 2, Commenter: 8, GroupID: 40})
	if err == nil || blocked {
		t.Fatalf("blocked %v err %v", blocked, err)
	}
	users.blockErr = nil
	users.settingsErr = errors.New("notify rpc down")
	if _, _, err = repliesPrivacy(context.Background(), users, channel.RepliesNotice{Recipient: 2, Commenter: 8, GroupID: 40}); err == nil {
		t.Fatal("settings error treated as deliverable")
	}
}

func TestRepliesPrivacyPreviewOffStillPushes(t *testing.T) {
	users := &privacyUsers{previewOff: true}
	blocked, settings, err := repliesPrivacy(context.Background(), users, channel.RepliesNotice{Recipient: 2, Commenter: 8, GroupID: 40})
	if err != nil || blocked {
		t.Fatal(err)
	}
	pol := channel.RepliesDeliveryOf(blocked, 100, settings...)
	if !pol.Write || !pol.Push || pol.Silent {
		t.Fatalf("preview %#v", pol)
	}
}

type privacyUsers struct {
	blocked     bool
	blockErr    error
	settingsErr error
	previewOff  bool
	blockCalls  int
	blockUser   int64
	blockPeer   int64
}

func (p *privacyUsers) UserIsBlockedByUser(_ context.Context, in *user.TLUserIsBlockedByUser) (*mtproto.Bool, error) {
	p.blockCalls++
	p.blockUser = in.GetUserId()
	p.blockPeer = in.GetPeerUserId()
	if p.blockErr != nil {
		return nil, p.blockErr
	}
	return mtproto.ToBool(p.blocked), nil
}

func (p *privacyUsers) UserGetNotifySettings(context.Context, *user.TLUserGetNotifySettings) (*mtproto.PeerNotifySettings, error) {
	if p.settingsErr != nil {
		return nil, p.settingsErr
	}
	st := &mtproto.PeerNotifySettings{}
	if p.previewOff {
		st.ShowPreviews = mtproto.ToBool(false)
	}
	return mtproto.MakeTLPeerNotifySettings(st).To_PeerNotifySettings(), nil
}
