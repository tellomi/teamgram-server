package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/engine"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

func (s *Service) PhoneCreateConferenceCall7D0444BB(ctx context.Context, in *mtproto.TLPhoneCreateConferenceCall7D0444BB) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	params := ""
	if in.GetParams() != nil {
		params = in.GetParams().GetData()
	}
	return s.eng.CreateConference(engine.CreateConference{
		UserID: md.GetUserId(), Auth: md.GetPermAuthKeyId(), RandomID: in.GetRandomId(),
		Muted: in.GetMuted(), VideoStopped: in.GetVideoStopped(), Join: in.GetJoin(),
		PublicKey: in.GetPublicKey(), Block: in.GetBlock(), Params: params,
	})
}

func (s *Service) PhoneCreateConferenceCallDFC909AB(ctx context.Context, in *mtproto.TLPhoneCreateConferenceCallDFC909AB) (*mtproto.Phone_PhoneCall, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	if in.GetPeer() == nil {
		return nil, mtproto.ErrCallPeerInvalid
	}
	return s.eng.ConferenceFromCall(md.GetUserId(), in.GetPeer().GetId(), in.GetPeer().GetAccessHash(), in.GetKeyFingerprint())
}

func (s *Service) PhoneDeleteConferenceCallParticipants(ctx context.Context, in *mtproto.TLPhoneDeleteConferenceCallParticipants) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	id, access, err := s.callIDs(in.GetCall())
	if err != nil {
		return nil, err
	}
	return s.eng.DeleteConferenceParticipants(md.GetUserId(), id, access, in.GetOnlyLeft(), in.GetKick(), in.GetIds(), in.GetBlock())
}

func (s *Service) PhoneSendConferenceCallBroadcast(ctx context.Context, in *mtproto.TLPhoneSendConferenceCallBroadcast) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	id, access, err := s.callIDs(in.GetCall())
	if err != nil {
		return nil, err
	}
	return s.eng.BroadcastBlock(md.GetUserId(), id, access, in.GetBlock())
}

func (s *Service) PhoneInviteConferenceCallParticipant(ctx context.Context, in *mtproto.TLPhoneInviteConferenceCallParticipant) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	id, access, err := s.callIDs(in.GetCall())
	if err != nil {
		return nil, err
	}
	who := mtproto.FromInputUser(md.GetUserId(), in.GetUserId())
	if who.PeerType != mtproto.PEER_USER || who.PeerId == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	rec, err := s.eng.InviteConference(md.GetUserId(), md.GetPermAuthKeyId(), id, access, who.PeerId, in.GetVideo())
	if err != nil {
		return nil, err
	}
	if stored := s.persistConferenceInvite(ctx, md.GetUserId(), md.GetPermAuthKeyId(), rec); stored != 0 {
		s.eng.AliasInvite(rec.MsgID, stored)
	}
	return rec.Updates, nil
}

func (s *Service) persistConferenceInvite(ctx context.Context, from, auth int64, rec *engine.ConferenceInvite) int32 {
	if s == nil || s.svc == nil || s.svc.Msg == nil || rec == nil || rec.Message == nil || rec.To == 0 {
		return 0
	}
	up, err := s.svc.Msg.MsgSendMessageV2(ctx, &msgpb.TLMsgSendMessageV2{
		UserId: from, AuthKeyId: auth, PeerType: mtproto.PEER_USER, PeerId: rec.To,
		Message: []*msgpb.OutboxMessage{
			msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
				RandomId: int64(rec.MsgID), Message: rec.Message,
			}).To_OutboxMessage(),
		},
	})
	if err != nil {
		return 0
	}
	return messageID(up)
}

func messageID(up *mtproto.Updates) int32 {
	if up == nil {
		return 0
	}
	for _, u := range up.GetUpdates() {
		if m := u.GetMessage_MESSAGE(); m != nil && m.GetId() != 0 {
			return m.GetId()
		}
	}
	return 0
}

func (s *Service) PhoneDeclineConferenceCallInvite(ctx context.Context, in *mtproto.TLPhoneDeclineConferenceCallInvite) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.DeclineConference(md.GetUserId(), in.GetMsgId())
}

func (s *Service) PhoneGetGroupCallChainBlocks(ctx context.Context, in *mtproto.TLPhoneGetGroupCallChainBlocks) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	id, access, err := s.callIDs(in.GetCall())
	if err != nil {
		return nil, err
	}
	return s.eng.ChainBlocks(md.GetUserId(), id, access, in.GetSubChainId(), in.GetOffset(), in.GetLimit())
}

func (s *Service) PhoneSendGroupCallEncryptedMessage(ctx context.Context, in *mtproto.TLPhoneSendGroupCallEncryptedMessage) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	id, access, err := s.callIDs(in.GetCall())
	if err != nil {
		return nil, err
	}
	return s.eng.SendGroupEncrypted(md.GetUserId(), id, access, in.GetEncryptedMessage())
}

func (s *Service) PhoneSendGroupCallMessageB1D11410(ctx context.Context, in *mtproto.TLPhoneSendGroupCallMessageB1D11410) (*mtproto.Updates, error) {
	return s.sendGroupCallMessage(ctx, in.GetCall(), in.GetRandomId(), in.GetMessage(), in.GetAllowPaidStars().GetValue(), in.GetAllowPaidStars() != nil, in.GetSendAs())
}

func (s *Service) PhoneSendGroupCallMessage87893014(ctx context.Context, in *mtproto.TLPhoneSendGroupCallMessage87893014) (*mtproto.Bool, error) {
	if _, err := s.sendGroupCallMessage(ctx, in.GetCall(), in.GetRandomId(), in.GetMessage(), 0, false, nil); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) sendGroupCallMessage(ctx context.Context, call *mtproto.InputGroupCall, randomID int64, text *mtproto.TextWithEntities, allow int64, hasAllow bool, sendAs *mtproto.InputPeer) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	id, access, err := s.callIDs(call)
	if err != nil {
		return nil, err
	}
	var allowPtr *int64
	if hasAllow {
		allowPtr = &allow
	}
	var as *mtproto.Peer
	admin := false
	if sendAs != nil {
		p := mtproto.FromInputPeer2(md.GetUserId(), sendAs)
		if p.PeerType == mtproto.PEER_SELF || p.PeerId == 0 {
			p.PeerType = mtproto.PEER_USER
			p.PeerId = md.GetUserId()
		}
		as = peerOf(p.PeerType, p.PeerId)
		if p.PeerType == mtproto.PEER_CHANNEL || p.PeerType == mtproto.PEER_CHAT {
			_, admin, _ = s.perm(ctx, md.GetUserId(), p)
		}
	}
	up, _, err := s.eng.SendGroupText(engine.GroupText{
		UserID: md.GetUserId(), CallID: id, Access: access, RandomID: randomID,
		Text: text, Allow: allowPtr, SendAs: as, Admin: admin,
	})
	return up, err
}

func (s *Service) PhoneDeleteGroupCallMessages(ctx context.Context, in *mtproto.TLPhoneDeleteGroupCallMessages) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	id, access, err := s.callIDs(in.GetCall())
	if err != nil {
		return nil, err
	}
	pt, pid, peerErr := s.eng.GroupPeer(id, access)
	admin := false
	if peerErr == nil && pid != 0 {
		_, admin, _ = s.perm(ctx, md.GetUserId(), &mtproto.PeerUtil{PeerType: pt, PeerId: pid})
	}
	_ = in.GetReportSpam()
	return s.eng.DeleteGroupMessages(md.GetUserId(), id, access, admin, in.GetMessages())
}

func (s *Service) PhoneDeleteGroupCallParticipantMessages(ctx context.Context, in *mtproto.TLPhoneDeleteGroupCallParticipantMessages) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	id, access, err := s.callIDs(in.GetCall())
	if err != nil {
		return nil, err
	}
	who := mtproto.FromInputPeer2(md.GetUserId(), in.GetParticipant())
	if who.PeerType == mtproto.PEER_SELF || who.PeerId == 0 {
		who.PeerType = mtproto.PEER_USER
		who.PeerId = md.GetUserId()
	}
	if who.PeerId == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	pt, pid, peerErr := s.eng.GroupPeer(id, access)
	admin := false
	if peerErr == nil && pid != 0 {
		_, admin, _ = s.perm(ctx, md.GetUserId(), &mtproto.PeerUtil{PeerType: pt, PeerId: pid})
	}
	_ = in.GetReportSpam()
	return s.eng.DeleteParticipantMessages(md.GetUserId(), id, access, admin, peerOf(who.PeerType, who.PeerId))
}

func (s *Service) PhoneGetGroupCallStars(ctx context.Context, in *mtproto.TLPhoneGetGroupCallStars) (*mtproto.Phone_GroupCallStars, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	id, access, err := s.callIDs(in.GetCall())
	if err != nil {
		return nil, err
	}
	return s.eng.GroupStars(md.GetUserId(), id, access)
}

func preferPeer(peers []*mtproto.Peer, saved *mtproto.Peer) []*mtproto.Peer {
	if saved == nil {
		return peers
	}
	idx := -1
	for i, p := range peers {
		if p.GetPredicateName() == saved.GetPredicateName() && p.GetUserId() == saved.GetUserId() && p.GetChatId() == saved.GetChatId() && p.GetChannelId() == saved.GetChannelId() {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return peers
	}
	peers[0], peers[idx] = peers[idx], peers[0]
	return peers
}
