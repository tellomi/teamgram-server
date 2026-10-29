package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/engine"
)

func (s *Service) PhoneGetCallConfig(ctx context.Context, _ *mtproto.TLPhoneGetCallConfig) (*mtproto.DataJSON, error) {
	if _, err := s.md(ctx); err != nil {
		return nil, err
	}
	return s.eng.CallConfig(), nil
}

func (s *Service) PhoneRequestCall(ctx context.Context, in *mtproto.TLPhoneRequestCall) (*mtproto.Phone_PhoneCall, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	peer := mtproto.FromInputUser(md.GetUserId(), in.GetUserId())
	if peer.PeerType != mtproto.PEER_USER || peer.PeerId == 0 || peer.PeerId == md.GetUserId() {
		return nil, mtproto.ErrCallPeerInvalid
	}
	return s.eng.RequestCall(engine.Request{
		Admin: md.GetUserId(), AdminAuth: md.GetPermAuthKeyId(), Participant: peer.PeerId,
		Video: in.GetVideo(), RandomID: in.GetRandomId(), GAHash: in.GetGAHash(), Protocol: in.GetProtocol(),
	})
}

func (s *Service) PhoneReceivedCall(ctx context.Context, in *mtproto.TLPhoneReceivedCall) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.Received(md.GetUserId(), md.GetPermAuthKeyId(), in.GetPeer().GetId(), in.GetPeer().GetAccessHash())
}

func (s *Service) PhoneAcceptCall(ctx context.Context, in *mtproto.TLPhoneAcceptCall) (*mtproto.Phone_PhoneCall, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.Accept(md.GetUserId(), md.GetPermAuthKeyId(), in.GetPeer().GetId(), in.GetPeer().GetAccessHash(), in.GetGB(), in.GetProtocol())
}

func (s *Service) PhoneConfirmCall(ctx context.Context, in *mtproto.TLPhoneConfirmCall) (*mtproto.Phone_PhoneCall, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.Confirm(md.GetUserId(), md.GetPermAuthKeyId(), in.GetPeer().GetId(), in.GetPeer().GetAccessHash(), in.GetGA(), in.GetKeyFingerprint(), in.GetProtocol())
}

func (s *Service) PhoneDiscardCall(ctx context.Context, in *mtproto.TLPhoneDiscardCall) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.Discard(md.GetUserId(), in.GetPeer().GetId(), in.GetPeer().GetAccessHash(), in.GetDuration(), in.GetReason())
}

func (s *Service) PhoneSetCallRating(ctx context.Context, in *mtproto.TLPhoneSetCallRating) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.Rate(md.GetUserId(), in.GetPeer().GetId(), in.GetPeer().GetAccessHash(), in.GetRating(), in.GetComment())
}

func (s *Service) PhoneSaveCallDebug(ctx context.Context, in *mtproto.TLPhoneSaveCallDebug) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	raw := ""
	if in.GetDebug() != nil {
		raw = in.GetDebug().GetData()
	}
	return s.eng.Debug(md.GetUserId(), in.GetPeer().GetId(), in.GetPeer().GetAccessHash(), raw)
}

func (s *Service) PhoneSendSignalingData(ctx context.Context, in *mtproto.TLPhoneSendSignalingData) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return s.eng.Signaling(md.GetUserId(), in.GetPeer().GetId(), in.GetPeer().GetAccessHash(), in.GetData())
}

func (s *Service) PhoneSaveCallLog(ctx context.Context, in *mtproto.TLPhoneSaveCallLog) (*mtproto.Bool, error) {
	if _, err := s.md(ctx); err != nil {
		return nil, err
	}
	if in.GetPeer() == nil {
		return nil, mtproto.ErrCallPeerInvalid
	}
	return mtproto.BoolTrue, nil
}
