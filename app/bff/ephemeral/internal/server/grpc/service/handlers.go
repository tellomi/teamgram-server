package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/ephemeral/internal/box"
)

func (s *Service) EphemeralSendMessage(ctx context.Context, in *mtproto.TLEphemeralSendMessage) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := peerOf(id, in.GetPeer())
	if err != nil {
		return nil, err
	}
	receiver, err := userOf(id, in.GetReceiverId())
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Box.Send(box.Send{
		Owner:      id,
		Peer:       peer,
		Receiver:   receiver,
		RandomID:   in.GetRandomId(),
		Text:       in.GetMessage(),
		Entities:   in.GetEntities(),
		Markup:     in.GetReplyMarkup(),
		Reply:      in.GetReplyTo(),
		Welcome:    in.GetWelcome(),
		Invert:     in.GetInvertMedia(),
		NoForwards: in.GetNoforwards(),
		Anchor:     in.GetAnchor(),
		QueryID:    in.GetQueryId(),
		Media:      in.GetMedia(),
		Rich:       in.GetRichMessage(),
	})
}

func (s *Service) EphemeralEditMessage(ctx context.Context, in *mtproto.TLEphemeralEditMessage) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := peerOf(id, in.GetPeer())
	if err != nil {
		return nil, err
	}
	receiver, err := userOf(id, in.GetReceiverId())
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Box.Edit(id, peer, receiver, in.GetId(), in.GetMessage(), in.GetEntities(), in.GetReplyMarkup(), in.GetMedia(), in.GetRichMessage(), in.GetWelcome(), in.GetInvertMedia())
}

func (s *Service) EphemeralDeleteMessage(ctx context.Context, in *mtproto.TLEphemeralDeleteMessage) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := peerOf(id, in.GetPeer())
	if err != nil {
		return nil, err
	}
	receiver, err := userOf(id, in.GetReceiverId())
	if err != nil {
		return nil, err
	}
	if err := s.svcCtx.Box.Delete(id, peer, receiver, in.GetId()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) EphemeralDeleteWelcomeMessage(ctx context.Context, in *mtproto.TLEphemeralDeleteWelcomeMessage) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := peerOf(id, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err := s.svcCtx.Box.DeleteWelcome(id, peer, in.GetId()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) EphemeralDeleteAllWelcomeMessages(ctx context.Context, in *mtproto.TLEphemeralDeleteAllWelcomeMessages) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := peerOf(id, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err := s.svcCtx.Box.DeleteAllWelcome(id, peer); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) EphemeralGetWelcomeMessages(ctx context.Context, in *mtproto.TLEphemeralGetWelcomeMessages) (*mtproto.Ephemeral_WelcomeMessages, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := peerOf(id, in.GetPeer())
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Box.Welcome(id, peer, in.GetHash())
}

func (s *Service) EphemeralGetCallbackAnswer(ctx context.Context, in *mtproto.TLEphemeralGetCallbackAnswer) (*mtproto.Messages_BotCallbackAnswer, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := peerOf(id, in.GetPeer())
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Box.Callback(id, peer, in.GetId(), in.GetData())
}

func (s *Service) EphemeralReportMessage(ctx context.Context, in *mtproto.TLEphemeralReportMessage) (*mtproto.ReportResult, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := peerOf(id, in.GetPeer())
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Box.Report(id, peer, in.GetId(), in.GetOption(), in.GetMessage())
}
