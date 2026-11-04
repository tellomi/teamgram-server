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
	push := buildAnnounceMessage(42, "Tellomi 已上线", 7)
	if push.UserId != announce.ServiceUserID || push.PeerId != 42 || push.PeerType != 2 || push.PushType != 1 {
		t.Fatalf("%+v", push)
	}
	msg := push.GetMessage().GetMessage()
	if msg.GetMessage() != "Tellomi 已上线" || msg.GetFromId().GetUserId() != announce.ServiceUserID {
		t.Fatalf("%+v", msg)
	}
	if len(msg.GetEntities()) != 0 {
		t.Fatal("announcement must not spoiler or label the text")
	}
	post := buildNewsPost("Tellomi 已上线", 8)
	if post.UserId != announce.NewsPosterID || post.PeerId != announce.NewsChannelID || post.PeerType != 4 {
		t.Fatalf("%+v", post)
	}
	body := post.GetMessage()[0].GetMessage()
	if !body.GetPost() || body.GetMessage() != "Tellomi 已上线" {
		t.Fatalf("%+v", body)
	}
}
