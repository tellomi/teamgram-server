package push

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

// SettingsReader loads per-chat and global notify settings.
type SettingsReader interface {
	NotifySettings(ctx context.Context, userID int64, peerType int32, peerID int64) (NotifySettings, error)
}

// Directory fills in a sender or chat name when the update did not carry one.
type Directory interface {
	UserName(ctx context.Context, id int64) (string, error)
	ChatTitle(ctx context.Context, id int64) (string, error)
}

// Service turns new-message updates into APNs (and, if configured, FCM) pushes.
type Service struct {
	store Store
	cfg   Config
	apns  *APNS
	fcm   FCMSender
	now   func() time.Time
}

func NewService(store Store, cfg Config) *Service {
	s := &Service{
		store: store,
		cfg:   cfg,
		fcm:   fcmUnconfigured{},
		now:   time.Now,
	}
	if cfg.Enabled() {
		client, err := NewAPNS(cfg)
		if err != nil {
			logx.Errorf("push: APNs disabled: %v", err)
		} else {
			s.apns = client
		}
	}
	return s
}

// SetFCM installs a Firebase sender. Until then token type 2 is stored and not delivered.
func (s *Service) SetFCM(sender FCMSender) {
	if sender == nil {
		s.fcm = fcmUnconfigured{}
		return
	}
	s.fcm = sender
}

// Deliver sends a push for each new incoming message in updates.
// Telegram delivers pushes even when another session is online; the iOS client
// suppresses the banner while the app is in the foreground. Muted chats are skipped
// unless the message mentions the recipient.
func (s *Service) Deliver(ctx context.Context, userID int64, updates *mtproto.Updates) {
	if s == nil || s.store == nil || userID == 0 {
		return
	}
	messages := Messages(userID, updates)
	if len(messages) == 0 {
		return
	}
	devices, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		logx.WithContext(ctx).Errorf("push: list devices user=%d: %v", userID, err)
		return
	}
	if len(devices) == 0 {
		return
	}
	for i := range messages {
		s.fillNames(ctx, &messages[i])
		decision := s.decide(ctx, messages[i])
		if !decision.Send {
			continue
		}
		s.send(ctx, devices, messages[i], decision)
	}
}

func (s *Service) decide(ctx context.Context, m Incoming) Decision {
	reader, ok := s.store.(SettingsReader)
	if !ok {
		return Decide(s.now().Unix(), NotifySettings{}, NotifySettings{}, m.Mentioned, m.Silent)
	}
	peer, err := reader.NotifySettings(ctx, m.UserID, m.PeerType, m.PeerID)
	if err != nil {
		logx.WithContext(ctx).Errorf("push: notify settings user=%d peer=%d: %v", m.UserID, m.PeerID, err)
	}
	var global NotifySettings
	if g := globalPeerType(m.PeerType); g != 0 {
		global, err = reader.NotifySettings(ctx, m.UserID, g, 0)
		if err != nil {
			logx.WithContext(ctx).Errorf("push: global notify settings user=%d: %v", m.UserID, err)
		}
	}
	return Decide(s.now().Unix(), peer, global, m.Mentioned, m.Silent)
}

func (s *Service) fillNames(ctx context.Context, m *Incoming) {
	dir, ok := s.store.(Directory)
	if !ok {
		return
	}
	if m.SenderName == "" && m.SenderID != 0 {
		if name, err := dir.UserName(ctx, m.SenderID); err == nil {
			m.SenderName = name
		}
	}
	if m.ChatTitle == "" && (m.ChatID != 0 || m.ChannelID != 0) {
		id := m.ChatID
		if id == 0 {
			id = m.ChannelID
		}
		if title, err := dir.ChatTitle(ctx, id); err == nil {
			m.ChatTitle = title
		}
	}
}

func (s *Service) send(ctx context.Context, devices []Device, m Incoming, d Decision) {
	for _, dev := range devices {
		switch dev.TokenType {
		case TokenAPNS:
			s.sendAPNS(ctx, dev, m, d)
		case TokenFCM:
			s.sendFCM(ctx, dev, m, d)
		case TokenAPNSVoIP:
			// VoIP tokens are stored for calls. Message alerts use token type 1.
		default:
		}
	}
}

func (s *Service) sendAPNS(ctx context.Context, dev Device, m Incoming, d Decision) {
	if s.apns == nil {
		return
	}
	alert, err := BuildAlert(m, d, dev.Secret)
	if err != nil {
		logx.WithContext(ctx).Errorf("push: build apns user=%d msg=%d: %v", m.UserID, m.MsgID, err)
		return
	}
	err = s.apns.Send(ctx, dev.Token, dev.AppSandbox, alert.Outer)
	if err == nil {
		return
	}
	var ae *APNSError
	if errors.As(err, &ae) && ae.DropDevice() {
		if delErr := s.store.Delete(ctx, dev.UserID, dev.TokenType, dev.Token, nil); delErr != nil {
			logx.WithContext(ctx).Errorf("push: drop token user=%d: %v", dev.UserID, delErr)
		}
	}
	logx.WithContext(ctx).Errorf("push: apns user=%d msg=%d token=%s: %v", m.UserID, m.MsgID, tokenTail(dev.Token), err)
}

func (s *Service) sendFCM(ctx context.Context, dev Device, m Incoming, d Decision) {
	alert, err := BuildAlert(m, d, dev.Secret)
	if err != nil {
		logx.WithContext(ctx).Errorf("push: build fcm user=%d msg=%d: %v", m.UserID, m.MsgID, err)
		return
	}
	err = s.fcm.Send(ctx, dev.Token, alert.Outer)
	if err == nil {
		return
	}
	if errors.Is(err, ErrFCMNotConfigured) {
		return
	}
	logx.WithContext(ctx).Errorf("push: fcm user=%d msg=%d: %v", m.UserID, m.MsgID, err)
}

func tokenTail(token string) string {
	token = strings.TrimSpace(token)
	if len(token) <= 6 {
		return token
	}
	return token[len(token)-6:]
}
