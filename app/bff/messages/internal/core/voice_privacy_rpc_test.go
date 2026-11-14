package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	mediaclient "github.com/teamgram/teamgram-server/app/service/media/client"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestSendVoicePrivacyAllowAndDeny(t *testing.T) {
	users := &voiceUser{allow: false}
	media := &voiceMedia{}
	msgs := &voiceMsg{}
	c := &MessagesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient:  users,
			MediaClient: media,
			MsgClient:   msgs,
		}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1, PermAuthKeyId: 11},
	}
	in := &mtproto.TLMessagesSendMedia{
		Peer:     mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2, AccessHash: 1}).To_InputPeer(),
		RandomId: 41,
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			MimeType: "audio/ogg",
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{Voice: true, Duration: 2}).To_DocumentAttribute(),
			},
		}).To_InputMedia(),
	}
	if _, err := c.MessagesSendMedia(in); status.Convert(err).Message() != "VOICE_MESSAGES_FORBIDDEN" {
		t.Fatalf("deny: %v", err)
	}
	if media.uploads != 0 || msgs.sends != 0 {
		t.Fatalf("denied voice still uploaded %d sent %d", media.uploads, msgs.sends)
	}

	users.allow = true
	in.RandomId = 42
	got, err := c.MessagesSendMedia(in)
	if err != nil || got == nil {
		t.Fatalf("allow: %v %v", got, err)
	}
	if media.uploads != 1 || msgs.sends != 1 {
		t.Fatalf("uploads %d sends %d", media.uploads, msgs.sends)
	}
}

type voiceUser struct {
	userclient.UserClient
	allow bool
}

func (u *voiceUser) UserCheckPrivacy(context.Context, *user.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	if u.allow {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

type voiceMedia struct {
	mediaclient.MediaClient
	uploads int
}

func (m *voiceMedia) MediaUploadedDocumentMedia(context.Context, *mediapb.TLMediaUploadedDocumentMedia) (*mtproto.MessageMedia, error) {
	m.uploads++
	return mtproto.MakeTLMessageMediaEmpty(nil).To_MessageMedia(), nil
}

type voiceMsg struct {
	msgclient.MsgClient
	sends int
}

func (m *voiceMsg) MsgSendMessageV2(context.Context, *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	m.sends++
	return mtproto.MakeEmptyUpdates(), nil
}
