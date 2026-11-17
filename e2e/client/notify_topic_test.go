package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestForumTopicNotifySettings sticks a mute on one topic of a megagroup
// and delivers updateNotifySettings, with notifyForumTopic, to the user's
// other session. The topic id is not the dialog: getNotifySettings for the
// channel itself stays unset.
func TestForumTopicNotifySettings(t *testing.T) {
	cfg := requireE2E(t)
	android, _ := loginNamed(t, cfg, ProfileAndroid, "topic_a", "TopicA")
	ios, _ := loginNamed(t, cfg, ProfileIOS, "topic_a", "TopicA")
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()

	created := invokeOK(t, android, &mtproto.TLChannelsCreateChannel{
		Constructor: mtproto.CRC32_channels_createChannel_91006707,
		Megagroup:   true,
		Forum:       true,
		Title:       "Topics",
		About:       "",
	})
	var chID, chHash int64
	for _, chat := range chatsOf(created) {
		if chat.GetId() != 0 {
			chID, chHash = chat.GetId(), chatAccessHash(chat)
		}
	}
	if chID == 0 {
		t.Fatalf("create channel %T", created)
	}
	const topicID int32 = 15
	topic := mtproto.MakeTLInputNotifyForumTopic(&mtproto.InputNotifyPeer{
		Peer:     inputPeerChannel(chID, chHash),
		TopMsgId: topicID,
	}).To_InputNotifyPeer()
	dialog := mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{
		Peer: inputPeerChannel(chID, chHash),
	}).To_InputNotifyPeer()

	_, err := android.Invoke(ctx, &mtproto.TLAccountUpdateNotifySettings{
		Constructor: mtproto.CRC32_account_updateNotifySettings,
		Peer: mtproto.MakeTLInputNotifyForumTopic(&mtproto.InputNotifyPeer{
			Peer: inputPeerChannel(chID, chHash),
		}).To_InputNotifyPeer(),
		Settings: mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
			MuteUntil: wrapperspb.Int32(1),
		}).To_InputPeerNotifySettings(),
	})
	rpc, ok := asRPCError(err)
	if err == nil || !ok || !strings.Contains(rpc.Message, "TOPIC_ID_INVALID") {
		t.Fatalf("topic id 0: %v", err)
	}

	if _, _, _, err := ios.State(ctx); err != nil {
		t.Fatal(err)
	}
	invokeOK(t, android, &mtproto.TLAccountUpdateNotifySettings{
		Constructor: mtproto.CRC32_account_updateNotifySettings,
		Peer:        topic,
		Settings: mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
			MuteUntil: wrapperspb.Int32(2147483647),
		}).To_InputPeerNotifySettings(),
	})

	got := notifySettings(t, ctx, android, topic)
	if got.GetMuteUntil().GetValue() != 2147483647 {
		t.Fatalf("topic getNotifySettings %+v", got)
	}
	dialogSettings := notifySettings(t, ctx, android, dialog)
	if dialogSettings.GetMuteUntil().GetValue() == 2147483647 {
		t.Fatalf("dialog inherited the topic mute: %+v", dialogSettings)
	}

	if err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		for _, obj := range ios.Pushed() {
			for _, upd := range updatesOf(obj) {
				if upd.GetPredicateName() != mtproto.Predicate_updateNotifySettings {
					continue
				}
				np := upd.GetPeer_NOTIFYPEER()
				if np.GetPredicateName() == mtproto.Predicate_notifyForumTopic &&
					np.GetTopMsgId() == topicID &&
					upd.GetNotifySettings().GetMuteUntil().GetValue() == 2147483647 {
					return true, nil
				}
			}
		}
		return false, nil
	}); err != nil {
		t.Fatalf("other session did not receive notifyForumTopic: %v; pushed=%s", err, pushedSummary(ios.Pushed()))
	}

	again := notifySettings(t, ctx, ios, topic)
	if again.GetMuteUntil().GetValue() != 2147483647 {
		t.Fatalf("other session getNotifySettings %+v", again)
	}
}
