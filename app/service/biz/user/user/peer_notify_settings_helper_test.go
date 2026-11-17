package user

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestFindPeerNotifySettingsIgnoresTopicRows(t *testing.T) {
	topicMute := mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		MuteUntil: wrapperspb.Int32(2147483647),
	}).To_PeerNotifySettings()
	dialog := mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		MuteUntil: wrapperspb.Int32(0),
	}).To_PeerNotifySettings()
	list := []*PeerPeerNotifySettings{
		MakeTLPeerPeerNotifySettings(&PeerPeerNotifySettings{
			PeerType: mtproto.PEER_CHANNEL,
			PeerId:   9,
			TopMsgId: 3,
			Settings: topicMute,
		}).To_PeerPeerNotifySettings(),
		MakeTLPeerPeerNotifySettings(&PeerPeerNotifySettings{
			PeerType: mtproto.PEER_CHANNEL,
			PeerId:   9,
			Settings: dialog,
		}).To_PeerPeerNotifySettings(),
	}
	got := FindPeerPeerNotifySettings(list, mtproto.MakeChannelPeerUtil(9))
	if got.GetMuteUntil().GetValue() != 0 {
		t.Fatalf("dialog inherited the topic mute: %+v", got)
	}
	onlyTopic := FindPeerPeerNotifySettings(list[:1], mtproto.MakeChannelPeerUtil(9))
	if onlyTopic.GetMuteUntil() != nil {
		t.Fatalf("a topic-only list was applied to the dialog: %+v", onlyTopic)
	}
}
