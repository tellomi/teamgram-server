package service

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/reaction"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func withUser(userID int64) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID, PermAuthKeyId: userID * 10}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

func TestSendReactionUserAndChatAreNotChannels(t *testing.T) {
	reaction.Use(reaction.NewMemory())
	boxes := map[int64]map[int32]*mtproto.MessageBox{
		1: {5: {
			UserId: 1, SenderUserId: 1, PeerType: mtproto.PEER_USER, PeerId: 2,
			MessageId: 5, DialogId1: 1, DialogId2: 2, DialogMessageId: 100,
		}},
		2: {8: {
			UserId: 2, SenderUserId: 1, PeerType: mtproto.PEER_USER, PeerId: 1,
			MessageId: 8, DialogId1: 1, DialogId2: 2, DialogMessageId: 100,
		}},
	}
	var pushed []int64
	svc := NewReactions(ReactionDeps{
		Box: func(ctx context.Context, userID int64, msgID int32) (*mtproto.MessageBox, error) {
			return boxes[userID][msgID], nil
		},
		BoxByDialog: func(ctx context.Context, userID, dialogMessageID int64) (*mtproto.MessageBox, error) {
			for _, b := range boxes[userID] {
				if b.DialogMessageId == dialogMessageID {
					return b, nil
				}
			}
			return nil, nil
		},
		ChatUsers: func(ctx context.Context, chatID int64) ([]int64, error) {
			return []int64{1, 2}, nil
		},
		Push: func(ctx context.Context, userID int64, updates *mtproto.Updates) {
			pushed = append(pushed, userID)
			ups := updates.GetUpdates()
			if len(ups) != 1 || ups[0].GetPredicateName() != mtproto.Predicate_updateMessageReactions || ups[0].GetReactions_MESSAGEREACTIONS() == nil {
				t.Fatalf("push user %d %#v", userID, ups)
			}
			if len(pushed) == 1 && ups[0].GetReactions_MESSAGEREACTIONS().GetResults()[0].GetCount() != 1 {
				t.Fatalf("first push %#v", ups[0].GetReactions_MESSAGEREACTIONS())
			}
		},
	})

	// Bob reacts using Alice's message id. His own box id is 8.
	req := &mtproto.TLMessagesSendReaction{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 1, AccessHash: 9}).To_InputPeer(),
		MsgId: 5,
		Reaction_FLAGVECTORREACTION: []*mtproto.Reaction{
			mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
		},
	}
	if got := reactionEmojis(req); len(got) != 1 || got[0] != "👍" {
		emo := ""
		if len(req.Reaction_FLAGVECTORREACTION) > 0 && req.Reaction_FLAGVECTORREACTION[0] != nil {
			emo = req.Reaction_FLAGVECTORREACTION[0].GetEmoticon()
		}
		t.Fatalf("emojis %#v emoticon %q", got, emo)
	}
	res, err := svc.MessagesSendReaction(withUser(2), req)
	if err != nil {
		t.Fatal(err)
	}
	sum := res.Updates[0].GetReactions_MESSAGEREACTIONS()
	if res.Updates[0].GetMsgId_INT32() != 8 || sum.GetResults()[0].GetChosenOrder() == nil || len(sum.GetRecentReactions()) != 1 {
		t.Fatalf("sender update %#v", res.Updates[0])
	}
	if len(pushed) != 1 || pushed[0] != 1 {
		t.Fatalf("pushed %v", pushed)
	}
	alice := &mtproto.Message{Id: 5, PeerId: mtproto.MakePeerUser(2)}
	bob := &mtproto.Message{Id: 8, PeerId: mtproto.MakePeerUser(1)}
	reaction.AttachMessages(context.Background(), reaction.Shared(), 1, []*mtproto.Message{alice})
	reaction.AttachMessages(context.Background(), reaction.Shared(), 2, []*mtproto.Message{bob})
	if alice.GetReactions().GetResults()[0].GetCount() != 1 || bob.GetReactions().GetResults()[0].GetReaction_REACTION().GetEmoticon() != "👍" {
		t.Fatalf("history alice %#v bob %#v", alice.GetReactions(), bob.GetReactions())
	}

	if _, err = svc.MessagesSendReaction(withUser(2), &mtproto.TLMessagesSendReaction{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 1}).To_InputPeer(),
		MsgId: 8,
	}); err != nil {
		t.Fatal(err)
	}
	bob.Reactions = nil
	reaction.AttachMessages(context.Background(), reaction.Shared(), 2, []*mtproto.Message{bob})
	if bob.GetReactions() != nil {
		t.Fatalf("chip remained %#v", bob.GetReactions())
	}

	// Basic group: each member has a different box id.
	boxes = map[int64]map[int32]*mtproto.MessageBox{
		1: {3: {
			UserId: 1, PeerType: mtproto.PEER_CHAT, PeerId: 40,
			MessageId: 3, DialogId1: -int64(mtproto.PEER_CHAT), DialogId2: 40, DialogMessageId: 70,
		}},
		2: {9: {
			UserId: 2, PeerType: mtproto.PEER_CHAT, PeerId: 40,
			MessageId: 9, DialogId1: -int64(mtproto.PEER_CHAT), DialogId2: 40, DialogMessageId: 70,
		}},
	}
	res, err = svc.MessagesSendReaction(withUser(1), &mtproto.TLMessagesSendReaction{
		Peer:  mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 40}).To_InputPeer(),
		MsgId: 3,
		Reaction_FLAGVECTORREACTION: []*mtproto.Reaction{
			mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "🔥"}).To_Reaction(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updates[0].GetReactions_MESSAGEREACTIONS().GetResults()[0].GetReaction_REACTION().GetEmoticon() != "🔥" {
		t.Fatalf("group %#v", res.Updates[0])
	}
}

func TestSendReactionChannelStillWorks(t *testing.T) {
	svcCh := channel.New(channel.NewMemoryStore())
	channel.Use(svcCh)
	ctx := context.Background()
	up, err := svcCh.Create(ctx, channel.CreateRequest{CreatorID: 4, Title: "C", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	sent, err := svcCh.Send(ctx, 4, ch.Id, 0, []channel.Outgoing{{RandomID: 1, Message: &mtproto.Message{Message: "post"}}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].Message_MESSAGE.Id
	svc := NewReactions(ReactionDeps{})
	res, err := svc.MessagesSendReaction(withUser(4), &mtproto.TLMessagesSendReaction{
		Peer:  mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: ch.Id}).To_InputPeer(),
		MsgId: msgID,
		Reaction_FLAGVECTORREACTION: []*mtproto.Reaction{
			mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updates[0].GetReactions_MESSAGEREACTIONS().GetResults()[0].GetCount() != 1 {
		t.Fatalf("channel %#v", res.Updates[0])
	}
	hist, err := svcCh.History(ctx, 4, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: ch.Id}, channel.HistoryQuery{Limit: 5})
	if err != nil || hist.GetMessages()[0].GetReactions().GetResults()[0].GetCount() != 1 {
		t.Fatalf("channel history %#v %v", hist, err)
	}
}
