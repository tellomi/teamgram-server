package push

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

// sendConcurrency is how many device pushes for one message run at once.
// Each APNs round trip is about 220ms. Sending them one after another left
// the phone that just registered waiting behind older devices. One failure
// does not cancel the others: a dead token must not drop the live phone.
const sendConcurrency = 4

// SettingsReader loads per-chat and global notify settings.
type SettingsReader interface {
	NotifySettings(ctx context.Context, userID int64, peerType int32, peerID int64) (NotifySettings, error)
}

// Directory fills in a sender or chat name when the update did not carry one.
type Directory interface {
	UserName(ctx context.Context, id int64) (string, error)
	ChatTitle(ctx context.Context, id int64) (string, error)
}

// Service turns new-message updates into APNs pushes and, when that sender
// is installed, FCM, Huawei, and Xiaomi pushes.
type Service struct {
	store  Store
	cfg    Config
	apns   *APNS
	fcm    FCMSender
	huawei HuaweiSender
	xiaomi XiaomiSender
	now    func() time.Time
}

func NewService(store Store, cfg Config) *Service {
	s := &Service{
		store:  store,
		cfg:    cfg,
		fcm:    fcmUnconfigured{},
		huawei: huaweiUnconfigured{},
		xiaomi: xiaomiUnconfigured{},
		now:    time.Now,
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

// SetHuawei installs a Huawei Push Kit sender. Until then token type 13 is stored and not delivered.
func (s *Service) SetHuawei(sender HuaweiSender) {
	if sender == nil {
		s.huawei = huaweiUnconfigured{}
		return
	}
	s.huawei = sender
}

// SetXiaomi installs a Xiaomi Push sender. Until then token type 14 is stored and not delivered.
func (s *Service) SetXiaomi(sender XiaomiSender) {
	if sender == nil {
		s.xiaomi = xiaomiUnconfigured{}
		return
	}
	s.xiaomi = sender
}

// Deliver sends a push for each new incoming message in updates, and a PushKit
// ring for phoneCallRequested. Telegram delivers pushes even when another
// session is online; a locked iOS app is often still marked online, so the
// VoIP push is sent as well as updatePhoneCall. Muted chats are skipped
// unless the message mentions the recipient. Call rings are not muted here.
func (s *Service) Deliver(ctx context.Context, userID int64, updates *mtproto.Updates) {
	if s == nil || s.store == nil || userID == 0 {
		return
	}
	messages := Messages(userID, updates)
	call, skip := prepareCall(userID, updates)
	if len(messages) == 0 && phoneRequested(updates) == nil {
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
	if skip != "" {
		logx.WithContext(ctx).Errorf("push: voip skipped user=%d: %s", userID, skip)
		return
	}
	if call.CallID != 0 {
		s.sendVoIP(ctx, devices, call)
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
	if len(devices) == 0 {
		return
	}
	devices = append([]Device(nil), devices...)
	sortDevicesNewestFirst(devices)
	var wg sync.WaitGroup
	sem := make(chan struct{}, sendConcurrency)
	for _, dev := range devices {
		sem <- struct{}{}
		wg.Add(1)
		go func(dev Device) {
			defer wg.Done()
			defer func() { <-sem }()
			s.sendDevice(ctx, dev, m, d)
		}(dev)
	}
	wg.Wait()
}

func (s *Service) sendDevice(ctx context.Context, dev Device, m Incoming, d Decision) {
	// account.updateDeviceLocked: a passcode hides the message text.
	if dev.LockedPeriod > 0 {
		d.Preview = false
	}
	switch dev.TokenType {
	case TokenAPNS:
		s.sendAPNS(ctx, dev, m, d)
	case TokenFCM:
		s.sendFCM(ctx, dev, m, d)
	case TokenHuawei:
		s.sendHuawei(ctx, dev, m, d)
	case TokenXiaomi:
		s.sendXiaomi(ctx, dev, m, d)
	case TokenAPNSVoIP:
		// Message alerts never use a PushKit token. Incoming calls go through sendVoIP.
	default:
	}
}

func (s *Service) sendAPNS(ctx context.Context, dev Device, m Incoming, d Decision) {
	if s.apns == nil {
		return
	}
	alert, err := BuildAlert(m, d, dev.Secret, dev.NotificationLang())
	if err != nil {
		logx.WithContext(ctx).Errorf("push: build apns user=%d msg=%d: %v", m.UserID, m.MsgID, err)
		return
	}
	err = s.apns.Send(ctx, dev.Token, dev.AppSandbox, alert.Outer)
	status, reason := apnsStatus(err)
	// key_id is the first 8 bytes of the id carried at the front of p.
	// The secret and the rest of p are not logged.
	logx.WithContext(ctx).Infof("push: apns user=%d msg=%d key_id=%s bytes=%d status=%d reason=%s",
		m.UserID, m.MsgID, apnsKeyIDHex(alert.Outer), len(alert.Outer), status, reason)
	if err == nil {
		return
	}
	s.forgetIfUnregistered(ctx, dev, err)
	logx.WithContext(ctx).Errorf("push: apns user=%d msg=%d token=%s: %v", m.UserID, m.MsgID, tokenTail(dev.Token), err)
}

func apnsStatus(err error) (int, string) {
	if err == nil {
		return http.StatusOK, ""
	}
	var ae *APNSError
	if errors.As(err, &ae) {
		return ae.Status, ae.Reason
	}
	return 0, err.Error()
}

// apnsKeyIDHex is the first 8 bytes of the notification key id inside p,
// hex-encoded. p is key id (8) || msg_key (16) || ciphertext. An alert with
// no p returns "-".
func apnsKeyIDHex(outer []byte) string {
	var wrap struct {
		P string `json:"p"`
	}
	if err := json.Unmarshal(outer, &wrap); err != nil || wrap.P == "" {
		return "-"
	}
	raw, err := decodeP(wrap.P)
	if err != nil || len(raw) < 8 {
		return "-"
	}
	return hex.EncodeToString(raw[:8])
}

// sendVoIP rings token_type 9 devices, and token types 2 and 13 when those
// senders are installed. A locked iOS session is often still marked online,
// so this runs even when sync also delivered updatePhoneCall. The client
// dedupes CallKit by call id. Android reads the same call from p.
func (s *Service) sendVoIP(ctx context.Context, devices []Device, c Call) {
	if s.apns != nil {
		for _, dev := range devices {
			if dev.TokenType != TokenAPNSVoIP {
				continue
			}
			body, err := BuildCall(c, dev.Secret)
			if err != nil {
				logx.WithContext(ctx).Errorf("push: build voip user=%d call=%d token=%s: %v", dev.UserID, c.CallID, tokenTail(dev.Token), err)
				continue
			}
			err = s.apns.SendVoIP(ctx, dev.Token, dev.AppSandbox, body)
			if err == nil {
				continue
			}
			s.forgetIfUnregistered(ctx, dev, err)
			logx.WithContext(ctx).Errorf("push: voip user=%d call=%d token=%s: %v", dev.UserID, c.CallID, tokenTail(dev.Token), err)
		}
	}
	s.sendAndroidCall(ctx, devices, c)
}

func (s *Service) sendAndroidCall(ctx context.Context, devices []Device, c Call) {
	for _, dev := range devices {
		switch dev.TokenType {
		case TokenFCM:
			s.sendFCMCall(ctx, dev, c)
		case TokenHuawei:
			s.sendHuaweiCall(ctx, dev, c)
		}
	}
}

func (s *Service) forgetIfUnregistered(ctx context.Context, dev Device, err error) {
	var ae *APNSError
	if !errors.As(err, &ae) || !ae.DropDevice() {
		return
	}
	if delErr := s.store.Delete(ctx, dev.UserID, dev.TokenType, dev.Token, nil); delErr != nil {
		logx.WithContext(ctx).Errorf("push: drop token user=%d: %v", dev.UserID, delErr)
	}
}

func (s *Service) sendFCM(ctx context.Context, dev Device, m Incoming, d Decision) {
	alert, err := BuildAlert(m, d, dev.Secret, dev.NotificationLang())
	if err != nil {
		logx.WithContext(ctx).Errorf("push: build fcm user=%d msg=%d: %v", m.UserID, m.MsgID, err)
		return
	}
	s.dispatchFCM(ctx, dev, m.UserID, int64(m.MsgID), "msg", alert.Android)
}

func (s *Service) sendFCMCall(ctx context.Context, dev Device, c Call) {
	if _, off := s.fcm.(fcmUnconfigured); off {
		return
	}
	payload, _, err := BuildAndroidCall(c, dev.Secret)
	if err != nil {
		logx.WithContext(ctx).Errorf("push: build fcm user=%d call=%d token=%s: %v", dev.UserID, c.CallID, tokenTail(dev.Token), err)
		return
	}
	s.dispatchFCM(ctx, dev, dev.UserID, c.CallID, "call", payload)
}

// dispatchFCM hands the payload to the installed sender. The HTTP client runs
// on its own goroutine with its own deadline: a blackholed FCM dial must not
// wait in front of APNs, Huawei, Xiaomi, or the rest of Deliver. Test doubles
// stay on the caller goroutine so a Deliver assertion can read them immediately.
func (s *Service) dispatchFCM(ctx context.Context, dev Device, userID, ref int64, kind string, payload []byte) {
	if real, ok := s.fcm.(*FCM); ok {
		select {
		case fcmInFlight <- struct{}{}:
		default:
			logx.WithContext(ctx).Errorf("push: fcm dropped user=%d %s=%d: busy", userID, kind, ref)
			return
		}
		payload = append([]byte(nil), payload...)
		go func() {
			defer func() { <-fcmInFlight }()
			s.finishFCM(dev, userID, ref, kind, payload, real)
		}()
		return
	}
	err := s.fcm.Send(ctx, dev.Token, payload)
	s.noteFCM(context.WithoutCancel(ctx), dev, userID, ref, kind, err)
}

func (s *Service) finishFCM(dev Device, userID, ref int64, kind string, payload []byte, fcm *FCM) {
	ctx, cancel := context.WithTimeout(context.Background(), fcmAttemptTimeout)
	defer cancel()
	err := fcm.Send(ctx, dev.Token, payload)
	s.noteFCM(context.Background(), dev, userID, ref, kind, err)
}

func (s *Service) noteFCM(ctx context.Context, dev Device, userID, ref int64, kind string, err error) {
	if err == nil || errors.Is(err, ErrFCMNotConfigured) {
		return
	}
	var fe *FCMError
	if errors.As(err, &fe) && fe.Drop() {
		if delErr := s.store.Delete(ctx, dev.UserID, dev.TokenType, dev.Token, nil); delErr != nil {
			logx.WithContext(ctx).Errorf("push: drop fcm token user=%d: %v", dev.UserID, delErr)
		}
	}
	logx.WithContext(ctx).Errorf("push: fcm user=%d %s=%d: %v", userID, kind, ref, err)
}

func (s *Service) sendHuawei(ctx context.Context, dev Device, m Incoming, d Decision) {
	alert, err := BuildAlert(m, d, dev.Secret, dev.NotificationLang())
	if err != nil {
		logx.WithContext(ctx).Errorf("push: build huawei user=%d msg=%d: %v", m.UserID, m.MsgID, err)
		return
	}
	if alert.Huawei == "" {
		// The client decrypts the whole data string. Without a secret there is nothing to send.
		return
	}
	s.dispatchHuawei(ctx, dev, m.UserID, int64(m.MsgID), "msg", alert.Huawei, false)
}

func (s *Service) sendHuaweiCall(ctx context.Context, dev Device, c Call) {
	if _, off := s.huawei.(huaweiUnconfigured); off {
		return
	}
	_, data, err := BuildAndroidCall(c, dev.Secret)
	if err != nil {
		logx.WithContext(ctx).Errorf("push: build huawei user=%d call=%d token=%s: %v", dev.UserID, c.CallID, tokenTail(dev.Token), err)
		return
	}
	s.dispatchHuawei(ctx, dev, dev.UserID, c.CallID, "call", data, true)
}

func (s *Service) dispatchHuawei(ctx context.Context, dev Device, userID, ref int64, kind, data string, call bool) {
	hctx, cancel := context.WithTimeout(ctx, huaweiAttemptTimeout)
	defer cancel()
	var err error
	if call {
		if hw, ok := s.huawei.(*Huawei); ok {
			err = hw.SendCall(hctx, dev.Token, data)
		} else {
			err = s.huawei.Send(hctx, dev.Token, data)
		}
	} else {
		err = s.huawei.Send(hctx, dev.Token, data)
	}
	s.noteHuawei(context.WithoutCancel(ctx), dev, userID, ref, kind, err)
}

func (s *Service) noteHuawei(ctx context.Context, dev Device, userID, ref int64, kind string, err error) {
	if err == nil || errors.Is(err, ErrHuaweiNotConfigured) {
		return
	}
	var he *HuaweiError
	if errors.As(err, &he) && he.Drop() {
		if delErr := s.store.Delete(ctx, dev.UserID, dev.TokenType, dev.Token, nil); delErr != nil {
			logx.WithContext(ctx).Errorf("push: drop huawei token user=%d: %v", dev.UserID, delErr)
		}
	}
	logx.WithContext(ctx).Errorf("push: huawei user=%d %s=%d: %v", userID, kind, ref, err)
}

func (s *Service) sendXiaomi(ctx context.Context, dev Device, m Incoming, d Decision) {
	alert, err := BuildAlert(m, d, dev.Secret, dev.NotificationLang())
	if err != nil {
		logx.WithContext(ctx).Errorf("push: build xiaomi user=%d msg=%d: %v", m.UserID, m.MsgID, err)
		return
	}
	msg := XiaomiMessage{
		Title:       alert.Title,
		Description: alert.Body,
		Payload:     alert.Huawei,
		Silent:      alert.Silent,
	}
	if msg.Payload == "" {
		// No device secret: the notification bar still shows the text.
		// Payload is the FCM-shaped JSON so a client can read loc_key.
		msg.Payload = string(alert.Android)
	}
	if msg.Title == "" {
		msg.Title = "Tellomi"
	}
	err = s.xiaomi.Send(ctx, dev.Token, msg)
	if err == nil || errors.Is(err, ErrXiaomiNotConfigured) {
		return
	}
	var xe *XiaomiError
	if errors.As(err, &xe) && xe.Drop() {
		if delErr := s.store.Delete(ctx, dev.UserID, dev.TokenType, dev.Token, nil); delErr != nil {
			logx.WithContext(ctx).Errorf("push: drop xiaomi token user=%d: %v", dev.UserID, delErr)
		}
	}
	logx.WithContext(ctx).Errorf("push: xiaomi user=%d msg=%d: %v", m.UserID, m.MsgID, err)
}

func tokenTail(token string) string {
	token = strings.TrimSpace(token)
	if len(token) <= 6 {
		return token
	}
	return token[len(token)-6:]
}
