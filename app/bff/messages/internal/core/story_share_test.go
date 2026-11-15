package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
	"google.golang.org/grpc/status"
)

func TestStoryShareKeepsTheCard(t *testing.T) {
	prev := story.Shared()
	svc := story.New(story.NewMemoryStore())
	story.Use(svc)
	t.Cleanup(func() { story.Use(prev) })
	ctx := context.Background()
	up, err := svc.Send(ctx, story.SendRequest{
		Actor: 1,
		Owner: story.Owner{Kind: story.KindUser, ID: 1},
		Media: mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
			Photo_FLAGPHOTO: mtproto.MakeTLPhoto(&mtproto.Photo{Id: 9, AccessHash: 8, Date: 1, DcId: 1}).To_Photo(),
		}).To_MessageMedia(),
		Rules:    []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule()},
		RandomID: 42,
		Caption:  "snap",
	})
	if err != nil {
		t.Fatal(err)
	}
	var id int32
	for _, u := range up.Updates {
		if u.GetPredicateName() == mtproto.Predicate_updateStoryID {
			id = u.GetId_INT32()
		}
	}
	if id == 0 {
		t.Fatal("no story id")
	}
	c := &MessagesCore{ctx: ctx, MD: &metadata.RpcMetadata{UserId: 1}}
	media, err := c.storyShareMedia(mtproto.MakeTLInputMediaStory(&mtproto.InputMedia{
		Peer:     mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Id_INT32: id,
	}).To_InputMedia())
	if err != nil {
		t.Fatal(err)
	}
	if media.GetPredicateName() != mtproto.Predicate_messageMediaStory || media.GetId() != id || media.GetStory().GetId() != id {
		t.Fatalf("media %#v", media)
	}
	if media.GetPeer().GetUserId() != 1 {
		t.Fatalf("peer %#v", media.GetPeer())
	}
	if _, err := c.storyShareMedia(mtproto.MakeTLInputMediaStory(&mtproto.InputMedia{
		Peer:     mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Id_INT32: id + 9,
	}).To_InputMedia()); status.Convert(err).Message() != "STORY_ID_INVALID" {
		t.Fatalf("missing story: %v", err)
	}
}
