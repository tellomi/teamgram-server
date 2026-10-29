package service

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/stories/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func withUser(userID, auth int64) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID, PermAuthKeyId: auth}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

func TestHandlersSendAndFetch(t *testing.T) {
	story.Use(story.New(story.NewMemoryStore()))
	svc := New(&svc.ServiceContext{})
	ctx := withUser(3, 30)
	up, err := svc.StoriesSendStory(ctx, &mtproto.TLStoriesSendStory{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Media: mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{
			Id_INPUTPHOTO: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: 4, AccessHash: 5}).To_InputPhoto(),
		}).To_InputMedia(),
		PrivacyRules: []*mtproto.InputPrivacyRule{
			mtproto.MakeTLInputPrivacyValueAllowContacts(nil).To_InputPrivacyRule(),
		},
		Caption:  &wrapperspb.StringValue{Value: "from bff"},
		RandomId: 77,
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
	got, err := svc.StoriesGetPeerStories(ctx, &mtproto.TLStoriesGetPeerStories{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
	})
	if err != nil || got.Stories.Stories[0].GetCaption().GetValue() != "from bff" {
		t.Fatalf("peer %+v %v", got, err)
	}
	item := got.Stories.Stories[0]
	if !item.GetContacts() || item.GetMedia().GetPhoto_FLAGPHOTO().GetId() != 4 {
		t.Fatalf("flags/media %+v", item)
	}
	n, err := svc.StoriesCanSendStory30EB63F0(ctx, &mtproto.TLStoriesCanSendStory30EB63F0{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
	})
	if err != nil || n.GetCountRemains() != 99 {
		t.Fatalf("remains %+v %v", n, err)
	}
}

func TestResolveMediaUsesUploadPipeline(t *testing.T) {
	fake := &fakeMedia{}
	in := mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
		File: mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 1, Name: "a.jpg", Parts: 1}).To_InputFile(),
	}).To_InputMedia()
	got, err := ResolveMedia(context.Background(), 10, in, fake)
	if err != nil {
		t.Fatal(err)
	}
	if !fake.photo || got.GetPhoto_FLAGPHOTO().GetId() != 55 {
		t.Fatalf("photo pipeline %+v photo=%v", got, fake.photo)
	}
	docIn := mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
		File:     mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 2, Name: "a.mp4", Parts: 1}).To_InputFile(),
		MimeType: "video/mp4",
	}).To_InputMedia()
	got, err = ResolveMedia(context.Background(), 10, docIn, fake)
	if err != nil || !fake.doc || got.GetDocument().GetId() != 66 {
		t.Fatalf("video pipeline %+v %v doc=%v", got, err, fake.doc)
	}
	if _, err = ResolveMedia(context.Background(), 10, in, nil); err == nil {
		t.Fatal("uploaded photo without the media service should fail")
	}
}

type fakeMedia struct {
	photo bool
	doc   bool
}

func (f *fakeMedia) MediaUploadPhotoFile(ctx context.Context, in *mediapb.TLMediaUploadPhotoFile) (*mtproto.Photo, error) {
	f.photo = true
	return mtproto.MakeTLPhoto(&mtproto.Photo{Id: 55, AccessHash: 1, Date: 1, DcId: 1}).To_Photo(), nil
}

func (f *fakeMedia) MediaGetPhotoSizeList(ctx context.Context, in *mediapb.TLMediaGetPhotoSizeList) (*mediapb.PhotoSizeList, error) {
	return &mediapb.PhotoSizeList{}, nil
}

func (f *fakeMedia) MediaUploadedDocumentMedia(ctx context.Context, in *mediapb.TLMediaUploadedDocumentMedia) (*mtproto.MessageMedia, error) {
	f.doc = true
	return mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
		Document: mtproto.MakeTLDocument(&mtproto.Document{Id: 66, MimeType: "video/mp4", DcId: 1}).To_Document(),
	}).To_MessageMedia(), nil
}

func (f *fakeMedia) MediaGetDocument(ctx context.Context, in *mediapb.TLMediaGetDocument) (*mtproto.Document, error) {
	return nil, nil
}
