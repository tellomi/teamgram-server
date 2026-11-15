package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stories/internal/svc"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

func TestStoryMentionSendsAPrivateStory(t *testing.T) {
	fake := &mentionMsg{}
	s := &Service{svcCtx: &svc.ServiceContext{Msg: fake}}
	up := mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{
			mtproto.MakeTLUpdateStoryID(&mtproto.Update{Id_INT32: 9, RandomId: 1}).To_Update(),
		},
	}).To_Updates()
	entities := []*mtproto.MessageEntity{
		mtproto.MakeTLInputMessageEntityMentionName(&mtproto.MessageEntity{
			Offset: 0, Length: 2,
			UserId_INPUTUSER: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 22}).To_InputUser(),
		}).To_MessageEntity(),
		mtproto.MakeTLMessageEntityMentionName(&mtproto.MessageEntity{
			Offset: 3, Length: 2, UserId_INT64: 22,
		}).To_MessageEntity(),
	}
	s.notifyStoryMentions(context.Background(), 21, 4, mtproto.MakePeerUser(21), storyIDOf(up), "ab cd", entities)
	if len(fake.sent) != 1 {
		t.Fatalf("sends %d", len(fake.sent))
	}
	got := fake.sent[0]
	if got.GetPeerId() != 22 {
		t.Fatalf("peer %d", got.GetPeerId())
	}
	media := got.GetMessage()[0].GetMessage().GetMedia()
	if !media.GetViaMention() || media.GetId() != 9 || media.GetPeer().GetUserId() != 21 {
		t.Fatalf("media %#v", media)
	}
	if names := mentionUsernames("hi @ada", []*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityMention(&mtproto.MessageEntity{Offset: 3, Length: 4}).To_MessageEntity(),
	}); len(names) != 1 || names[0] != "ada" {
		t.Fatalf("names %#v", names)
	}
}

type mentionMsg struct {
	msg_client.MsgClient
	sent []*msgpb.TLMsgSendMessageV2
}

func (m *mentionMsg) MsgSendMessageV2(_ context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	m.sent = append(m.sent, in)
	return mtproto.MakeTLUpdates(&mtproto.Updates{}).To_Updates(), nil
}
