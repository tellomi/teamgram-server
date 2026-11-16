package core

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/notification/plugin"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestGetNotifyExceptionsFiltersByPeerScope(t *testing.T) {
	prev := channel.Shared()
	chSvc := channel.New(channel.NewMemoryStore())
	channel.Use(chSvc)
	t.Cleanup(func() { channel.Use(prev) })

	ctx := context.Background()
	mega, err := chSvc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	feed, err := chSvc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "Feed", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	megaID := mega.Chats[0].GetId()
	feedID := feed.Chats[0].GetId()
	if megaID == 0 || feedID == 0 || megaID == feedID {
		t.Fatalf("channels mega=%d feed=%d", megaID, feedID)
	}
	megaFlag, megaOK, err := chSvc.IsBroadcast(ctx, megaID)
	if err != nil || !megaOK || megaFlag {
		t.Fatalf("megagroup broadcast=%v ok=%v err=%v", megaFlag, megaOK, err)
	}
	feedFlag, feedOK, err := chSvc.IsBroadcast(ctx, feedID)
	if err != nil || !feedOK || !feedFlag {
		t.Fatalf("broadcast flag=%v ok=%v err=%v", feedFlag, feedOK, err)
	}

	mute := mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		MuteUntil: wrapperspb.Int32(2147483647),
	}).To_PeerNotifySettings()
	sound := mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		AndroidSound: mtproto.MakeTLNotificationSoundRingtone(&mtproto.NotificationSound{Id: 9}).To_NotificationSound(),
	}).To_PeerNotifySettings()
	row := func(peerType int32, peerID int64, top int32, settings *mtproto.PeerNotifySettings) *user.PeerPeerNotifySettings {
		return user.MakeTLPeerPeerNotifySettings(&user.PeerPeerNotifySettings{
			PeerType: peerType,
			PeerId:   peerID,
			TopMsgId: top,
			Settings: settings,
		}).To_PeerPeerNotifySettings()
	}
	users := &notifyUserFake{all: &user.Vector_PeerPeerNotifySettings{
		Datas: []*user.PeerPeerNotifySettings{
			row(mtproto.PEER_USER, 21, 0, mute),
			row(mtproto.PEER_USER, 22, 0, sound),
			row(mtproto.PEER_CHAT, 31, 0, mute),
			row(mtproto.PEER_CHAT, 32, 0, sound),
			row(mtproto.PEER_CHANNEL, megaID, 0, mute),
			row(mtproto.PEER_CHANNEL, megaID, 9, mute),
			row(mtproto.PEER_CHANNEL, feedID, 0, mute),
			row(mtproto.PEER_CHANNEL, feedID, 4, mute),
			row(mtproto.PEER_CHANNEL, 999, 0, mute),
		},
	}}
	c := notifyCore(users, &notifySyncFake{}, "android")
	c.svcCtx.Dao.ChatClient = notifyChatFake{}

	all := []string{
		"user:21:0", "user:22:0",
		"chat:31:0", "chat:32:0",
		fmt.Sprintf("channel:%d:0", megaID), fmt.Sprintf("channel:%d:9", megaID),
		fmt.Sprintf("channel:%d:0", feedID), fmt.Sprintf("channel:%d:4", feedID),
		"channel:999:0",
	}
	usersOnly := []string{"user:21:0", "user:22:0"}
	chatsOnly := []string{
		"chat:31:0", "chat:32:0",
		fmt.Sprintf("channel:%d:0", megaID), fmt.Sprintf("channel:%d:9", megaID),
	}
	feedsOnly := []string{
		fmt.Sprintf("channel:%d:0", feedID), fmt.Sprintf("channel:%d:4", feedID),
	}

	check := func(t *testing.T) {
		t.Helper()
		assertExceptionKeys(t, c, nil, false, all)
		assertExceptionKeys(t, c, inputNotifyUsers(), false, usersOnly)
		assertExceptionKeys(t, c, inputNotifyUsers(), true, []string{"user:22:0"})
		assertExceptionKeys(t, c, inputNotifyChats(), false, chatsOnly)
		assertExceptionKeys(t, c, inputNotifyChats(), true, []string{"chat:32:0"})
		assertExceptionKeys(t, c, inputNotifyBroadcasts(), false, feedsOnly)
		assertExceptionKeys(t, c, inputNotifyBroadcasts(), true, nil)
		assertExceptionKeys(t, c, inputNotifyOne(mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: megaID}).To_InputPeer()), false, []string{
			fmt.Sprintf("channel:%d:0", megaID), fmt.Sprintf("channel:%d:9", megaID),
		})
		assertExceptionKeys(t, c, inputNotifyOne(mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 21}).To_InputPeer()), false, []string{"user:21:0"})
		assertExceptionKeys(t, c, inputNotifyOne(mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: feedID}).To_InputPeer()), false, feedsOnly)
		assertExceptionKeys(t, c, inputNotifyOne(mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 31}).To_InputPeer()), false, []string{"chat:31:0"})
		assertExceptionKeys(t, c, mtproto.MakeTLInputNotifyForumTopic(&mtproto.InputNotifyPeer{
			Peer:     mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: megaID}).To_InputPeer(),
			TopMsgId: 9,
		}).To_InputNotifyPeer(), false, []string{fmt.Sprintf("channel:%d:9", megaID)})
		assertExceptionKeys(t, c, mtproto.MakeTLInputNotifyForumTopic(&mtproto.InputNotifyPeer{
			Peer:     mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: megaID}).To_InputPeer(),
			TopMsgId: 3,
		}).To_InputNotifyPeer(), false, nil)

		up, err := c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{Peer: inputNotifyBroadcasts()})
		if err != nil {
			t.Fatal(err)
		}
		var sawFeed, sawMega bool
		for _, chat := range up.GetChats() {
			if chat.GetId() == feedID {
				sawFeed = chat.GetBroadcast()
			}
			if chat.GetId() == megaID {
				sawMega = true
			}
		}
		if !sawFeed || sawMega {
			t.Fatalf("broadcast cards feed=%v mega=%v chats=%v", sawFeed, sawMega, up.GetChats())
		}
		up, err = c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{Peer: inputNotifyChats()})
		if err != nil {
			t.Fatal(err)
		}
		var sawRoom, roomBroadcast, sawFeedInChats bool
		for _, chat := range up.GetChats() {
			if chat.GetId() == megaID {
				sawRoom = true
				roomBroadcast = chat.GetBroadcast()
			}
			if chat.GetId() == feedID {
				sawFeedInChats = true
			}
		}
		if !sawRoom || roomBroadcast || sawFeedInChats {
			t.Fatalf("chat cards room=%v broadcast=%v feed=%v", sawRoom, roomBroadcast, sawFeedInChats)
		}

		_, err = c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{
			Peer: mtproto.MakeTLInputNotifyForumTopic(&mtproto.InputNotifyPeer{
				Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: megaID}).To_InputPeer(),
			}).To_InputNotifyPeer(),
		})
		if err != mtproto.ErrTopicIdInvalid {
			t.Fatalf("topic 0: %v", err)
		}
		_, err = c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{
			Peer: mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{}).To_InputNotifyPeer(),
		})
		if err != mtproto.ErrPeerIdInvalid {
			t.Fatalf("empty peer: %v", err)
		}
		_, err = c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{
			Peer: &mtproto.InputNotifyPeer{PredicateName: "inputNotifyNope"},
		})
		if err != mtproto.ErrPeerIdInvalid {
			t.Fatalf("unknown peer: %v", err)
		}
	}

	t.Run("store", check)
	c.svcCtx.Plugin = scopeChannelPlugin{}
	t.Run("plugin", check)
}

func assertExceptionKeys(t *testing.T, c *NotificationCore, peer *mtproto.InputNotifyPeer, compareSound bool, want []string) {
	t.Helper()
	up, err := c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{
		Peer:         peer,
		CompareSound: compareSound,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := exceptionKeys(up)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if len(got) == 0 {
		got = nil
	}
	if len(want) == 0 {
		want = nil
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("peer %s compare_sound=%v\n got %v\nwant %v", peer.GetPredicateName(), compareSound, got, want)
	}
}

func exceptionKeys(up *mtproto.Updates) []string {
	keys := make([]string, 0, len(up.GetUpdates()))
	for _, upd := range up.GetUpdates() {
		np := upd.GetPeer_NOTIFYPEER()
		p := np.GetPeer()
		var kind string
		var id int64
		switch p.GetPredicateName() {
		case mtproto.Predicate_peerUser:
			kind, id = "user", p.GetUserId()
		case mtproto.Predicate_peerChat:
			kind, id = "chat", p.GetChatId()
		case mtproto.Predicate_peerChannel:
			kind, id = "channel", p.GetChannelId()
		default:
			kind = p.GetPredicateName()
		}
		keys = append(keys, fmt.Sprintf("%s:%d:%d", kind, id, np.GetTopMsgId()))
	}
	sort.Strings(keys)
	return keys
}

func inputNotifyUsers() *mtproto.InputNotifyPeer {
	return mtproto.MakeTLInputNotifyUsers(&mtproto.InputNotifyPeer{}).To_InputNotifyPeer()
}

func inputNotifyChats() *mtproto.InputNotifyPeer {
	return mtproto.MakeTLInputNotifyChats(&mtproto.InputNotifyPeer{}).To_InputNotifyPeer()
}

func inputNotifyBroadcasts() *mtproto.InputNotifyPeer {
	return mtproto.MakeTLInputNotifyBroadcasts(&mtproto.InputNotifyPeer{}).To_InputNotifyPeer()
}

func inputNotifyOne(peer *mtproto.InputPeer) *mtproto.InputNotifyPeer {
	return mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{Peer: peer}).To_InputNotifyPeer()
}

type scopeChannelPlugin struct{}

func (scopeChannelPlugin) GetChannelById(ctx context.Context, selfID, id int64) (*mtproto.Chat, error) {
	chats := channel.Shared().ChatsFor(ctx, selfID, id)
	if len(chats) == 0 || chats[0].GetId() == 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	return chats[0], nil
}

func (scopeChannelPlugin) GetChannelListByIdList(ctx context.Context, selfID int64, ids ...int64) []*mtproto.Chat {
	return channel.Shared().ChatsFor(ctx, selfID, ids...)
}

var _ plugin.NotificationPlugin = scopeChannelPlugin{}

type notifyChatFake struct {
	chatclient.ChatClient
}

func (notifyChatFake) ChatGetChatListByIdList(context.Context, *chatpb.TLChatGetChatListByIdList) (*chatpb.Vector_MutableChat, error) {
	return &chatpb.Vector_MutableChat{}, nil
}
