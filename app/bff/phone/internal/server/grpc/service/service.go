package service

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/engine"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/svc"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/phonecall"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

type Service struct {
	mtproto.UnimplementedRPCVoipCallsServer
	mtproto.UnimplementedRPCGroupCallsServer
	mtproto.UnimplementedRPCConferenceCallsServer
	mtproto.UnimplementedRPCSecretChatsServer
	svc        *svc.ServiceContext
	eng        *engine.Engine
	secretPush func(userID int64, upd *mtproto.Updates)
}

func New(sc *svc.ServiceContext) *Service {
	if sc == nil {
		sc = &svc.ServiceContext{Config: config.Config{}.Normalized()}
	}
	s := &Service{svc: sc}
	s.eng = engine.New(engine.Options{
		Config:  sc.Config,
		Relay:   sc.Relay,
		Media:   sc.SFU,
		Push:    s.push,
		OnCall:  s.onCall,
		OnGroup: s.onGroup,
		OnBind: func(b engine.Bind) {
			phonecall.Publish(b.PeerType, b.PeerID, b.CallID, b.Access, b.Active, b.NotEmpty)
		},
		Allow:   s.allow,
		Users:   s.loadUsers,
		Chats:   s.loadChats,
		Members: s.members,
	})
	channel.LookupGroupCall = func(ctx context.Context, channelID int64) channel.GroupCallAttach {
		id, access, active, notEmpty, ok := phonecall.Lookup(mtproto.PEER_CHANNEL, channelID)
		if !ok || !active {
			return channel.GroupCallAttach{}
		}
		return channel.GroupCallAttach{
			Call:     mtproto.MakeTLInputGroupCall(&mtproto.InputGroupCall{Id: id, AccessHash: access}).To_InputGroupCall(),
			Active:   true,
			NotEmpty: notEmpty,
		}
	}
	return s
}

// NewWithEngine is for tests that drive handlers without opening UDP sockets.
func NewWithEngine(eng *engine.Engine) *Service {
	return &Service{eng: eng, svc: &svc.ServiceContext{Config: config.Config{}.Normalized()}}
}

// LiveCallAlive reports whether a stories.startLive room is still running.
func (s *Service) LiveCallAlive(callID int64) bool {
	if s == nil || s.eng == nil || callID == 0 {
		return false
	}
	return s.eng.LiveCallAlive(callID)
}

// StartLiveStory opens the group call for stories.startLive.
func (s *Service) StartLiveStory(host, auth int64, rtmp bool, random int32, audience []int64) (int64, int64, *mtproto.Updates, error) {
	if s == nil || s.eng == nil {
		return 0, 0, nil, mtproto.ErrGroupcallInvalid
	}
	return s.eng.CreateLiveStory(engine.CreateLiveStory{
		Host: host, Auth: auth, RTMP: rtmp, Random: random, Audience: audience,
	})
}

// SetLiveEnd receives discard, leave-timeout, and quiet-media ends.
// callID is the group call that just went off air.
func (s *Service) SetLiveEnd(fn func(callID int64)) {
	if s == nil || s.eng == nil {
		return
	}
	s.eng.SetOnLive(func(ev engine.LiveEnd) {
		if fn != nil && ev.CallID != 0 {
			fn(ev.CallID)
		}
	})
}

func (s *Service) md(ctx context.Context) (*rpcmd.RpcMetadata, error) {
	md := rpcmd.RpcMetadataFromIncoming(ctx)
	if md == nil || md.GetUserId() == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	return md, nil
}

func (s *Service) push(n engine.Note) {
	if s.svc == nil || s.svc.Sync == nil || n.Updates == nil {
		return
	}
	ctx := context.Background()
	if len(n.Exclude) > 0 {
		_, _ = s.svc.Sync.SyncPushUpdatesIfNot(ctx, &sync.TLSyncPushUpdatesIfNot{
			UserId: n.UserID, Excludes: n.Exclude, Updates: n.Updates,
		})
		return
	}
	_, _ = s.svc.Sync.SyncPushUpdates(ctx, &sync.TLSyncPushUpdates{UserId: n.UserID, Updates: n.Updates})
}

func (s *Service) allow(caller, callee int64) (bool, bool) {
	if s.svc == nil || s.svc.Users == nil {
		return true, true
	}
	ctx := context.Background()
	ok, err := s.svc.Users.UserCheckPrivacy(ctx, &user.TLUserCheckPrivacy{
		UserId: callee, KeyType: int32(mtproto.PHONE_CALL), PeerId: caller,
	})
	if err == nil && !mtproto.FromBool(ok) {
		return false, false
	}
	p2p, err := s.svc.Users.UserCheckPrivacy(ctx, &user.TLUserCheckPrivacy{
		UserId: callee, KeyType: int32(mtproto.PHONE_P2P), PeerId: caller,
	})
	if err != nil || mtproto.FromBool(p2p) {
		return true, true
	}
	return true, false
}

func (s *Service) loadUsers(viewer int64, ids []int64) []*mtproto.User {
	if s.svc == nil || s.svc.Users == nil || len(ids) == 0 {
		return nil
	}
	got, err := s.svc.Users.UserGetMutableUsers(context.Background(), &user.TLUserGetMutableUsers{Id: ids, To: []int64{viewer}})
	if err != nil || got == nil {
		return nil
	}
	return got.GetUserListByIdList(viewer, ids...)
}

func (s *Service) loadChats(viewer int64, peerType int32, peerID int64) []*mtproto.Chat {
	if peerType != mtproto.PEER_CHANNEL {
		return nil
	}
	ch := channel.Shared().VisibleChat(context.Background(), viewer, peerID)
	if ch == nil {
		return nil
	}
	return []*mtproto.Chat{ch}
}

func (s *Service) members(peerType int32, peerID int64) []int64 {
	ctx := context.Background()
	switch peerType {
	case mtproto.PEER_CHANNEL:
		ids, err := channel.Shared().MemberIDs(ctx, peerID)
		if err != nil {
			return nil
		}
		return ids
	case mtproto.PEER_CHAT:
		if s.svc == nil || s.svc.Chat == nil {
			return nil
		}
		mc, err := s.svc.Chat.ChatGetMutableChat(ctx, &chat.TLChatGetMutableChat{ChatId: peerID})
		if err != nil || mc == nil {
			return nil
		}
		return mc.ParticipantIdList()
	default:
		return nil
	}
}

func (s *Service) onCall(ev engine.CallEnd) {
	if s.svc == nil || s.svc.Msg == nil {
		return
	}
	action := mtproto.MakeMessageActionPhoneCall(ev.Video, ev.CallID, ev.Reason, ev.Duration)
	s.sendMessage(context.Background(), ev.Admin, 0, mtproto.PEER_USER, ev.Participant, action, ev.CallID)
}

func (s *Service) onGroup(ev engine.GroupEnd) {
	if ev.Action == nil {
		return
	}
	if ev.PeerType == mtproto.PEER_USER {
		for _, uid := range ev.Action.GetUsers() {
			s.sendMessage(context.Background(), ev.From, 0, mtproto.PEER_USER, uid, ev.Action, ev.RandomID)
		}
		return
	}
	if ev.PeerType == mtproto.PEER_CHANNEL {
		_, _ = channel.Shared().Send(context.Background(), ev.From, ev.PeerID, 0, []channel.Outgoing{{
			RandomID: ev.RandomID,
			Message: mtproto.MakeTLMessageService(&mtproto.Message{
				FromId: mtproto.MakePeerUser(ev.From),
				PeerId: mtproto.MakePeerChannel(ev.PeerID),
				Date:   int32(time.Now().Unix()),
				Action: ev.Action,
			}).To_Message(),
		}})
		return
	}
	s.sendMessage(context.Background(), ev.From, 0, ev.PeerType, ev.PeerID, ev.Action, ev.RandomID)
}

func (s *Service) sendMessage(ctx context.Context, from, auth int64, peerType int32, peerID int64, action *mtproto.MessageAction, randomID int64) {
	if s.svc == nil || s.svc.Msg == nil || action == nil || peerID == 0 {
		return
	}
	msg := mtproto.MakeTLMessageService(&mtproto.Message{
		Out: true, FromId: mtproto.MakePeerUser(from), PeerId: peerOf(peerType, peerID),
		Date: int32(time.Now().Unix()), Action: action,
	}).To_Message()
	_, _ = s.svc.Msg.MsgSendMessageV2(ctx, &msgpb.TLMsgSendMessageV2{
		UserId: from, AuthKeyId: auth, PeerType: peerType, PeerId: peerID,
		Message: []*msgpb.OutboxMessage{
			msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{RandomId: randomID, Message: msg}).To_OutboxMessage(),
		},
	})
}

func peerOf(peerType int32, id int64) *mtproto.Peer {
	switch peerType {
	case mtproto.PEER_CHAT:
		return mtproto.MakePeerChat(id)
	case mtproto.PEER_CHANNEL:
		return mtproto.MakePeerChannel(id)
	default:
		return mtproto.MakePeerUser(id)
	}
}

// callRoomAccess reports whether user may enter a voice chat in peer.
// public is true for a channel that has a username. blocked is a kick or a
// view_messages ban, which denies entry even when public is true. A basic
// chat with no chat client stays open, the same fallback perm uses.
func (s *Service) callRoomAccess(ctx context.Context, user int64, peerType int32, peerID int64) (member, public, blocked bool, err error) {
	switch peerType {
	case mtproto.PEER_CHANNEL:
		member, _, public, blocked, err = channel.Shared().CallJoin(ctx, user, peerID)
		return member, public, blocked, err
	case mtproto.PEER_CHAT:
		if s == nil || s.svc == nil || s.svc.Chat == nil {
			return true, false, false, nil
		}
		mc, err := s.svc.Chat.ChatGetMutableChat(ctx, &chat.TLChatGetMutableChat{ChatId: peerID})
		if err != nil || mc == nil {
			return false, false, false, mtproto.ErrPeerIdInvalid
		}
		p, ok := mc.GetImmutableChatParticipant(user)
		if !ok || !p.IsChatMemberStateNormal() {
			return false, false, false, nil
		}
		return true, false, false, nil
	default:
		return false, false, false, mtproto.ErrPeerIdInvalid
	}
}

// joinAsAllowed reports whether user may appear in a call as join.
// Manage is the join-as peer's call-admin bit, which is what a self join
// used to leave false and a channel join used to copy.
func (s *Service) joinAsAllowed(ctx context.Context, user int64, join *mtproto.PeerUtil) (bool, error) {
	if join == nil {
		return false, mtproto.ErrJoinAsPeerInvalid
	}
	switch join.PeerType {
	case mtproto.PEER_USER:
		if join.PeerId == user {
			return false, nil
		}
		return false, mtproto.ErrJoinAsPeerInvalid
	case mtproto.PEER_CHANNEL, mtproto.PEER_CHAT:
		member, manage, err := s.perm(ctx, user, join)
		if err != nil {
			return false, err
		}
		if !member {
			return false, mtproto.ErrJoinAsPeerInvalid
		}
		return manage, nil
	default:
		return false, mtproto.ErrJoinAsPeerInvalid
	}
}

func (s *Service) perm(ctx context.Context, user int64, peer *mtproto.PeerUtil) (member, manage bool, err error) {
	if peer == nil {
		return false, false, mtproto.ErrPeerIdInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_CHANNEL:
		return channel.Shared().CallAccess(ctx, user, peer.PeerId, peer.AccessHash)
	case mtproto.PEER_CHAT:
		if s.svc == nil || s.svc.Chat == nil {
			return true, true, nil
		}
		mc, err := s.svc.Chat.ChatGetMutableChat(ctx, &chat.TLChatGetMutableChat{ChatId: peer.PeerId})
		if err != nil || mc == nil {
			return false, false, mtproto.ErrPeerIdInvalid
		}
		p, ok := mc.GetImmutableChatParticipant(user)
		if !ok || !p.IsChatMemberStateNormal() {
			return false, false, mtproto.ErrPeerIdInvalid
		}
		manage = p.IsChatMemberCreator() || p.IsChatMemberAdmin()
		if p.IsChatMemberAdmin() && p.GetAdminRights() != nil {
			manage = p.GetAdminRights().CanManageCall()
		}
		return true, manage, nil
	default:
		return false, false, mtproto.ErrPeerIdInvalid
	}
}

func boolp(b *mtproto.Bool) *bool {
	if b == nil {
		return nil
	}
	v := mtproto.FromBool(b)
	return &v
}

// withUser builds an incoming RPC context. Tests use it.
func withUser(userID, auth int64) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID, PermAuthKeyId: auth}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}
