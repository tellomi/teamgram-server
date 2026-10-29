package service

import (
	"context"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/engine"
)

func (s *Service) PhoneCreateGroupCall(ctx context.Context, in *mtproto.TLPhoneCreateGroupCall) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	peer := mtproto.FromInputPeer2(md.GetUserId(), in.GetPeer())
	_, manage, err := s.perm(ctx, md.GetUserId(), peer)
	if err != nil {
		return nil, err
	}
	if !manage {
		return nil, mtproto.ErrChatAdminRequired
	}
	title := ""
	if in.GetTitle() != nil {
		title = in.GetTitle().GetValue()
	}
	var schedule int32
	if in.GetScheduleDate() != nil {
		schedule = in.GetScheduleDate().GetValue()
	}
	return s.eng.CreateGroup(engine.CreateGroup{
		UserID: md.GetUserId(), Auth: md.GetPermAuthKeyId(),
		PeerType: peer.PeerType, PeerID: peer.PeerId, RandomID: in.GetRandomId(),
		Title: title, Schedule: schedule, RTMP: in.GetRtmpStream(),
	})
}

func (s *Service) PhoneJoinGroupCall(ctx context.Context, in *mtproto.TLPhoneJoinGroupCall) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	join := mtproto.FromInputPeer2(md.GetUserId(), in.GetJoinAs())
	if join.PeerType == mtproto.PEER_SELF || join.PeerType == mtproto.PEER_EMPTY || join.PeerId == 0 {
		join.PeerType = mtproto.PEER_USER
		join.PeerId = md.GetUserId()
	}
	member, manage := true, false
	if join.PeerType == mtproto.PEER_CHANNEL || join.PeerType == mtproto.PEER_CHAT {
		member, manage, err = s.perm(ctx, md.GetUserId(), join)
		if err != nil {
			return nil, err
		}
	}
	hash := ""
	if in.GetInviteHash() != nil {
		hash = in.GetInviteHash().GetValue()
	}
	params := ""
	if in.GetParams() != nil {
		params = in.GetParams().GetData()
	}
	return s.eng.JoinGroup(engine.JoinGroup{
		UserID: md.GetUserId(), Auth: md.GetPermAuthKeyId(),
		CallID: in.GetCall().GetId(), Access: in.GetCall().GetAccessHash(),
		Muted: in.GetMuted(), VideoStopped: in.GetVideoStopped(),
		PeerType: join.PeerType, PeerID: join.PeerId, InviteHash: hash,
		Params: params, Block: in.GetBlock(), Member: member, Manage: manage,
	})
}

func (s *Service) PhoneLeaveGroupCall(ctx context.Context, in *mtproto.TLPhoneLeaveGroupCall) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.LeaveGroup(md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash(), in.GetSource())
}

func (s *Service) PhoneInviteToGroupCall(ctx context.Context, in *mtproto.TLPhoneInviteToGroupCall) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, u := range in.GetUsers() {
		p := mtproto.FromInputUser(md.GetUserId(), u)
		if p.PeerType == mtproto.PEER_USER && p.PeerId != 0 {
			ids = append(ids, p.PeerId)
		}
	}
	return s.eng.Invite(md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash(), ids)
}

func (s *Service) manageCall(ctx context.Context, user, id, access int64) bool {
	pt, pid, err := s.eng.GroupPeer(id, access)
	if err != nil {
		return false
	}
	_, manage, err := s.perm(ctx, user, &mtproto.PeerUtil{PeerType: pt, PeerId: pid})
	return err == nil && manage
}

func (s *Service) PhoneDiscardGroupCall(ctx context.Context, in *mtproto.TLPhoneDiscardGroupCall) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.DiscardGroup(md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash(), s.manageCall(ctx, md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash()))
}

func (s *Service) PhoneToggleGroupCallSettings(ctx context.Context, in *mtproto.TLPhoneToggleGroupCallSettings) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	var stars *int64
	if in.GetSendPaidMessagesStars() != nil {
		v := in.GetSendPaidMessagesStars().GetValue()
		stars = &v
	}
	return s.eng.ToggleSettings(md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash(), s.manageCall(ctx, md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash()), in.GetResetInviteHash(), boolp(in.GetJoinMuted()), boolp(in.GetMessagesEnabled()), stars)
}

func (s *Service) PhoneGetGroupCall(ctx context.Context, in *mtproto.TLPhoneGetGroupCall) (*mtproto.Phone_GroupCall, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.GetGroup(md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash(), in.GetLimit())
}

func (s *Service) PhoneGetGroupParticipants(ctx context.Context, in *mtproto.TLPhoneGetGroupParticipants) (*mtproto.Phone_GroupParticipants, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	var peers []string
	for _, p := range in.GetIds() {
		u := mtproto.FromInputPeer2(md.GetUserId(), p)
		if u.PeerType == mtproto.PEER_SELF {
			u.PeerType = mtproto.PEER_USER
			u.PeerId = md.GetUserId()
		}
		if u.PeerId != 0 {
			peers = append(peers, fmt.Sprintf("%d:%d", u.PeerType, u.PeerId))
		}
	}
	return s.eng.Participants(md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash(), in.GetSources(), peers, in.GetOffset(), in.GetLimit())
}

func (s *Service) PhoneCheckGroupCall(ctx context.Context, in *mtproto.TLPhoneCheckGroupCall) (*mtproto.Vector_Int, error) {
	if _, err := s.md(ctx); err != nil {
		return nil, err
	}
	sources, err := s.eng.CheckSources(in.GetCall().GetId(), in.GetCall().GetAccessHash(), in.GetSources())
	if err != nil {
		return nil, err
	}
	return &mtproto.Vector_Int{Datas: sources}, nil
}

func (s *Service) PhoneToggleGroupCallRecord(ctx context.Context, in *mtproto.TLPhoneToggleGroupCallRecord) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	title := ""
	if in.GetTitle() != nil {
		title = in.GetTitle().GetValue()
	}
	return s.eng.ToggleRecord(md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash(), s.manageCall(ctx, md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash()), in.GetStart(), in.GetVideo(), title)
}

func (s *Service) PhoneEditGroupCallParticipant(ctx context.Context, in *mtproto.TLPhoneEditGroupCallParticipant) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	p := mtproto.FromInputPeer2(md.GetUserId(), in.GetParticipant())
	if p.PeerType == mtproto.PEER_SELF || p.PeerId == 0 {
		p.PeerType = mtproto.PEER_USER
		p.PeerId = md.GetUserId()
	}
	var volume *int32
	if in.GetVolume() != nil {
		v := in.GetVolume().GetValue()
		volume = &v
	}
	return s.eng.EditParticipant(engine.EditPart{
		UserID: md.GetUserId(), CallID: in.GetCall().GetId(), Access: in.GetCall().GetAccessHash(),
		PeerType: p.PeerType, PeerID: p.PeerId, Manage: s.manageCall(ctx, md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash()),
		Muted: boolp(in.GetMuted()), Volume: volume, Raise: boolp(in.GetRaiseHand()),
		VideoStopped: boolp(in.GetVideoStopped()), VideoPaused: boolp(in.GetVideoPaused()),
		PresentationPaused: boolp(in.GetPresentationPaused()),
	})
}

func (s *Service) PhoneEditGroupCallTitle(ctx context.Context, in *mtproto.TLPhoneEditGroupCallTitle) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.EditTitle(md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash(), s.manageCall(ctx, md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash()), in.GetTitle())
}

func (s *Service) PhoneGetGroupCallJoinAs(ctx context.Context, in *mtproto.TLPhoneGetGroupCallJoinAs) (*mtproto.Phone_JoinAsPeers, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	peer := mtproto.FromInputPeer2(md.GetUserId(), in.GetPeer())
	peers := []*mtproto.Peer{mtproto.MakePeerUser(md.GetUserId())}
	var chats []*mtproto.Chat
	if peer.PeerType == mtproto.PEER_CHANNEL || peer.PeerType == mtproto.PEER_CHAT {
		if member, _, err := s.perm(ctx, md.GetUserId(), peer); err == nil && member {
			peers = append(peers, peerOf(peer.PeerType, peer.PeerId))
			if cs := s.loadChats(md.GetUserId(), peer.PeerType, peer.PeerId); len(cs) > 0 {
				chats = cs
			}
		}
	}
	if chats == nil {
		chats = []*mtproto.Chat{}
	}
	return mtproto.MakeTLPhoneJoinAsPeers(&mtproto.Phone_JoinAsPeers{
		Peers: peers, Chats: chats, Users: []*mtproto.User{},
	}).To_Phone_JoinAsPeers(), nil
}

func (s *Service) PhoneExportGroupCallInvite(ctx context.Context, in *mtproto.TLPhoneExportGroupCallInvite) (*mtproto.Phone_ExportedGroupCallInvite, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	link, err := s.eng.ExportInvite(md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash(), s.manageCall(ctx, md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash()), in.GetCanSelfUnmute())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLPhoneExportedGroupCallInvite(&mtproto.Phone_ExportedGroupCallInvite{Link: link}).To_Phone_ExportedGroupCallInvite(), nil
}

func (s *Service) PhoneToggleGroupCallStartSubscription(ctx context.Context, in *mtproto.TLPhoneToggleGroupCallStartSubscription) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.Subscribe(md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash(), mtproto.FromBool(in.GetSubscribed()))
}

func (s *Service) PhoneStartScheduledGroupCall(ctx context.Context, in *mtproto.TLPhoneStartScheduledGroupCall) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.StartScheduled(md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash(), s.manageCall(ctx, md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash()))
}

func (s *Service) PhoneSaveDefaultGroupCallJoinAs(ctx context.Context, in *mtproto.TLPhoneSaveDefaultGroupCallJoinAs) (*mtproto.Bool, error) {
	if _, err := s.md(ctx); err != nil {
		return nil, err
	}
	if in.GetPeer() == nil || in.GetJoinAs() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) PhoneJoinGroupCallPresentation(ctx context.Context, in *mtproto.TLPhoneJoinGroupCallPresentation) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	params := ""
	if in.GetParams() != nil {
		params = in.GetParams().GetData()
	}
	return s.eng.JoinPresentation(md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash(), params)
}

func (s *Service) PhoneLeaveGroupCallPresentation(ctx context.Context, in *mtproto.TLPhoneLeaveGroupCallPresentation) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.LeavePresentation(md.GetUserId(), in.GetCall().GetId(), in.GetCall().GetAccessHash())
}

func (s *Service) PhoneGetGroupCallStreamChannels(ctx context.Context, in *mtproto.TLPhoneGetGroupCallStreamChannels) (*mtproto.Phone_GroupCallStreamChannels, error) {
	if _, err := s.md(ctx); err != nil {
		return nil, err
	}
	return s.eng.StreamChannels(in.GetCall().GetId(), in.GetCall().GetAccessHash())
}

func (s *Service) PhoneGetGroupCallStreamRtmpUrl(ctx context.Context, in *mtproto.TLPhoneGetGroupCallStreamRtmpUrl) (*mtproto.Phone_GroupCallStreamRtmpUrl, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	peer := mtproto.FromInputPeer2(md.GetUserId(), in.GetPeer())
	url, key := s.eng.RTMP(peer.PeerType, peer.PeerId, mtproto.FromBool(in.GetRevoke()))
	return mtproto.MakeTLPhoneGroupCallStreamRtmpUrl(&mtproto.Phone_GroupCallStreamRtmpUrl{Url: url, Key: key}).To_Phone_GroupCallStreamRtmpUrl(), nil
}

func (s *Service) PhoneSaveDefaultSendAs(ctx context.Context, in *mtproto.TLPhoneSaveDefaultSendAs) (*mtproto.Bool, error) {
	if _, err := s.md(ctx); err != nil {
		return nil, err
	}
	if in.GetCall() == nil || in.GetSendAs() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return mtproto.BoolTrue, nil
}
