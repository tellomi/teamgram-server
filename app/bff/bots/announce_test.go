package bots

import (
	"testing"

	"github.com/teamgram/teamgram-server/pkg/announce"
	"github.com/zeromicro/go-zero/zrpc"
)

func TestStartAnnounceOff(t *testing.T) {
	t.Setenv(announce.EnvEnabled, "")
	t.Setenv(announce.EnvKey, "")
	StartAnnounce(zrpc.RpcClientConf{}, "")
}

func TestAnnounceMessageShape(t *testing.T) {
	notice := announce.Outbound{
		From: announce.TellomiAIUserID,
		Text: "Tellomi AI 正式上线",
		Entities: []announce.Entity{{
			Type: "bold", Offset: 0, Length: 11,
		}},
	}
	push := buildAnnounceMessage(notice, 42, 7)
	if push.UserId != announce.TellomiAIUserID || push.PeerId != 42 || push.PeerType != 2 || push.PushType != 0 {
		t.Fatalf("%+v", push)
	}
	msg := push.GetMessage().GetMessage()
	if msg.GetMessage() != notice.Text || msg.GetFromId().GetUserId() != announce.TellomiAIUserID {
		t.Fatalf("%+v", msg)
	}
	if len(msg.GetEntities()) != 1 || msg.GetEntities()[0].GetPredicateName() != "messageEntityBold" || msg.GetEntities()[0].GetLength() != 11 {
		t.Fatalf("entities %+v", msg.GetEntities())
	}
	post := buildNewsPost(notice, 8)
	if post.UserId != announce.NewsPosterID || post.PeerId != announce.NewsChannelID || post.PeerType != 4 {
		t.Fatalf("%+v", post)
	}
	body := post.GetMessage()[0].GetMessage()
	if !body.GetPost() || body.GetMessage() != notice.Text || len(body.GetEntities()) != 1 {
		t.Fatalf("%+v", body)
	}
}
