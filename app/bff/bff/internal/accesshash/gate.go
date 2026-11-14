// Package accesshash is the bff gate for TELLOMI_ACCESS_HASH_ENFORCE.
// Every client RPC registered on this process is walked. A request that
// carries inputUser, inputPeerUser, inputChannel, or inputPeerChannel is
// checked against the access_hash this server already returned.
package accesshash

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/botadmin"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	ah "github.com/teamgram/teamgram-server/pkg/accesshash"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func init() {
	// Handlers resolve input*FromMessage only while enforcement is on.
	// The interceptor has already required the message to contain the id.
	// Off and log keep the historical "drop the id" behavior.
	mtproto.ResolveFromMessage = func() bool {
		return ah.Active() == ah.ModeEnforce
	}
}

// These are the hashes catalog.VenueBotUser and catalog.GifBotUser put on
// the user objects clients cache. They are not rows in users.
var (
	venueAccess = catalog.VenueBotUser().GetAccessHash().GetValue()
	gifAccess   = catalog.GifBotUser().GetAccessHash().GetValue()
)

type userAPI interface {
	UserGetMutableUsers(ctx context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error)
}

type messageAPI interface {
	MessageGetUserMessage(ctx context.Context, in *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error)
}

type liveBook struct {
	users    userAPI
	messages messageAPI
}

// Unary rejects mismatched access hashes when enforcement is on.
// The default mode returns before any lookup.
func Unary(users userAPI, messages messageAPI) grpc.UnaryServerInterceptor {
	book := &liveBook{users: users, messages: messages}
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if ah.Active() == ah.ModeOff {
			return handler(ctx, req)
		}
		var msg proto.Message
		if m, ok := req.(proto.Message); ok {
			msg = m
		}
		var viewer int64
		if md := metadata.RpcMetadataFromIncoming(ctx); md != nil {
			viewer = md.GetUserId()
		}
		method := ""
		if info != nil {
			method = info.FullMethod
		}
		if err := ah.Check(ctx, viewer, method, msg, book, func(line string) {
			logx.WithContext(ctx).Errorf("%s", line)
		}); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func (b *liveBook) UserHashes(ctx context.Context, viewer int64, ids []int64) (map[int64][]int64, error) {
	out := map[int64][]int64{}
	if b != nil && b.users != nil && len(ids) > 0 {
		got, err := b.users.UserGetMutableUsers(ctx, &userpb.TLUserGetMutableUsers{
			Id: ids,
			To: []int64{viewer},
		})
		if err != nil {
			return nil, err
		}
		if got != nil {
			for _, id := range ids {
				u, ok := got.GetImmutableUser(id)
				if ok && u != nil && u.User != nil && u.AccessHash() != 0 {
					out[id] = append(out[id], u.AccessHash())
				}
			}
		}
	}
	for _, id := range ids {
		if h, ok := botadmin.IssuedAccessHash(id); ok && h != 0 {
			out[id] = append(out[id], h)
		}
		if h, ok := chatbox.Shared().CachedAccessHash(id); ok && h != 0 {
			out[id] = append(out[id], h)
		}
		if h, ok := builtinHash(id); ok && h != 0 {
			out[id] = append(out[id], h)
		}
	}
	return out, nil
}

func builtinHash(id int64) (int64, bool) {
	switch id {
	case mtproto.BotVenueId:
		return venueAccess, true
	case mtproto.BotGifId:
		return gifAccess, true
	default:
		return 0, false
	}
}

func (b *liveBook) ChannelHashes(ctx context.Context, ids []int64) (map[int64][]int64, error) {
	out := map[int64][]int64{}
	svc := channel.Shared()
	for _, id := range ids {
		h, ok, err := svc.IssuedAccessHash(ctx, id)
		if err != nil {
			return nil, err
		}
		if ok && h != 0 {
			out[id] = []int64{h}
		}
	}
	return out, nil
}

func (b *liveBook) MessageHasUser(ctx context.Context, viewer int64, peer *mtproto.InputPeer, msgID int32, userID int64) (bool, error) {
	if userID == 0 || msgID == 0 {
		return false, nil
	}
	switch ah.InputPeerClass(peer) {
	case ah.KindPeerUser, ah.KindSelf, ah.KindChat:
		box, err := b.userBox(ctx, viewer, msgID)
		if err != nil || box == nil {
			return false, err
		}
		if !boxMatches(box, viewer, peer) {
			return false, nil
		}
		return ah.MessageContainsUser(box.GetMessage(), box.GetSenderUserId(), userID), nil
	case ah.KindPeerChannel:
		body, ok, err := channel.Shared().VisibleMessage(ctx, viewer, peer.GetChannelId(), msgID)
		if err != nil || !ok {
			return false, err
		}
		// The stored sender of an anonymous post is not shown to the client.
		return ah.MessageContainsUser(body, 0, userID), nil
	default:
		return false, nil
	}
}

func (b *liveBook) MessageHasChannel(ctx context.Context, viewer int64, peer *mtproto.InputPeer, msgID int32, channelID int64) (bool, error) {
	if channelID == 0 || msgID == 0 {
		return false, nil
	}
	switch ah.InputPeerClass(peer) {
	case ah.KindPeerUser, ah.KindSelf, ah.KindChat:
		box, err := b.userBox(ctx, viewer, msgID)
		if err != nil || box == nil {
			return false, err
		}
		if !boxMatches(box, viewer, peer) {
			return false, nil
		}
		return ah.MessageContainsChannel(box.GetMessage(), channelID), nil
	case ah.KindPeerChannel:
		body, ok, err := channel.Shared().VisibleMessage(ctx, viewer, peer.GetChannelId(), msgID)
		if err != nil || !ok {
			return false, err
		}
		return ah.MessageContainsChannel(body, channelID), nil
	default:
		return false, nil
	}
}

func (b *liveBook) userBox(ctx context.Context, viewer int64, msgID int32) (*mtproto.MessageBox, error) {
	if b == nil || b.messages == nil {
		return nil, mtproto.ErrInternalServerError
	}
	box, err := b.messages.MessageGetUserMessage(ctx, &messagepb.TLMessageGetUserMessage{
		UserId: viewer,
		Id:     msgID,
	})
	if err != nil {
		if status.Convert(err).Message() == "MESSAGE_ID_INVALID" {
			return nil, nil
		}
		return nil, err
	}
	if box == nil || box.GetMessageId() == 0 {
		return nil, nil
	}
	return box, nil
}

// boxMatches is the same dialog check as reactions_list boxPeerMatches.
// A private message row stores the dialog as the sorted pair of user ids.
// The recipient's copy can have peer_id equal to the recipient, so peer_id
// alone does not name the chat for an incoming message.
func boxMatches(box *mtproto.MessageBox, viewer int64, peer *mtproto.InputPeer) bool {
	if box == nil || peer == nil {
		return false
	}
	var wantType int32
	var wantID int64
	switch ah.InputPeerClass(peer) {
	case ah.KindSelf:
		wantType = mtproto.PEER_USER
		wantID = viewer
		if box.GetPeerType() == mtproto.PEER_SELF && (box.GetPeerId() == viewer || box.GetPeerId() == 0) {
			return true
		}
	case ah.KindPeerUser:
		wantType = mtproto.PEER_USER
		wantID = peer.GetUserId()
	case ah.KindChat:
		wantType = mtproto.PEER_CHAT
		wantID = peer.GetChatId()
	default:
		return false
	}
	if wantType == mtproto.PEER_USER {
		if a, b := box.GetDialogId1(), box.GetDialogId2(); a != 0 || b != 0 {
			x, y := viewer, wantID
			if x > y {
				x, y = y, x
			}
			return a == x && b == y
		}
		if box.GetPeerType() == mtproto.PEER_USER && box.GetPeerId() == viewer && box.GetSenderUserId() == wantID {
			return true
		}
	}
	return box.GetPeerType() == wantType && box.GetPeerId() == wantID
}

// RegisterLookups installs the user and channel row readers used when the
// flag is log or enforce. The lookup must not call back into chatbox: poll
// and bot issuance call it while the chatbox lock is held.
func RegisterLookups(users userAPI) {
	ah.SetUserLookup(func(ctx context.Context, ids []int64) (map[int64]int64, error) {
		if users == nil || len(ids) == 0 {
			return nil, nil
		}
		got, err := users.UserGetMutableUsers(ctx, &userpb.TLUserGetMutableUsers{Id: ids})
		if err != nil || got == nil {
			return nil, err
		}
		out := map[int64]int64{}
		for _, id := range ids {
			u, ok := got.GetImmutableUser(id)
			if ok && u != nil && u.User != nil && u.AccessHash() != 0 {
				out[id] = u.AccessHash()
			}
		}
		return out, nil
	})
	ah.SetChannelLookup(func(ctx context.Context, ids []int64) (map[int64]int64, error) {
		out := map[int64]int64{}
		svc := channel.Shared()
		for _, id := range ids {
			h, ok, err := svc.IssuedAccessHash(ctx, id)
			if err != nil {
				return nil, err
			}
			if ok && h != 0 {
				out[id] = h
			}
		}
		return out, nil
	})
}
