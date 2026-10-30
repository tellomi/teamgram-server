package service

import (
	"context"
	"time"
	"unicode"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/dh"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/phonehistory"
	"github.com/teamgram/teamgram-server/app/bff/phone/secretchat"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesRequestEncryption
// messages.requestEncryption#f64daf43 user_id:InputUser random_id:int g_a:bytes = EncryptedChat;
// https://core.telegram.org/method/messages.requestEncryption
func (s *Service) MessagesRequestEncryption(ctx context.Context, in *mtproto.TLMessagesRequestEncryption) (*mtproto.EncryptedChat, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	if !dh.ValidPublic(in.GetGA()) {
		return nil, mtproto.ErrDhGAInvalid
	}
	peer, err := s.secretPeer(ctx, md.GetUserId(), in.GetUserId())
	if err != nil {
		return nil, err
	}
	chat, events, err := secretchat.Default().Request(md.GetUserId(), peer, in.GetRandomId(), in.GetGA(), now())
	if err != nil {
		return nil, err
	}
	s.fanout(events)
	return chat, nil
}

// MessagesAcceptEncryption
// messages.acceptEncryption#3dbc0415 peer:InputEncryptedChat g_b:bytes key_fingerprint:long = EncryptedChat;
// https://core.telegram.org/method/messages.acceptEncryption
func (s *Service) MessagesAcceptEncryption(ctx context.Context, in *mtproto.TLMessagesAcceptEncryption) (*mtproto.EncryptedChat, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	// g_b is a DH public value. DH_G_A_INVALID is the documented 400 for a
	// public value outside the prime-order subgroup.
	if !dh.ValidPublic(in.GetGB()) {
		return nil, mtproto.ErrDhGAInvalid
	}
	chat, events, err := secretchat.Default().Accept(md.GetUserId(), in.GetPeer(), in.GetGB(), in.GetKeyFingerprint(), now())
	if err != nil {
		return nil, err
	}
	s.fanout(events)
	return chat, nil
}

// MessagesDiscardEncryption
// messages.discardEncryption#f393aea0 flags:# delete_history:flags.0?true chat_id:int = Bool;
// https://core.telegram.org/method/messages.discardEncryption
func (s *Service) MessagesDiscardEncryption(ctx context.Context, in *mtproto.TLMessagesDiscardEncryption) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	events, err := secretchat.Default().Discard(md.GetUserId(), in.GetChatId(), in.GetDeleteHistory(), now())
	if err != nil {
		return nil, err
	}
	s.fanout(events)
	return mtproto.BoolTrue, nil
}

// MessagesSetEncryptedTyping
// messages.setEncryptedTyping#791451ed peer:InputEncryptedChat typing:Bool = Bool;
// https://core.telegram.org/method/messages.setEncryptedTyping
func (s *Service) MessagesSetEncryptedTyping(ctx context.Context, in *mtproto.TLMessagesSetEncryptedTyping) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	events, err := secretchat.Default().Typing(md.GetUserId(), in.GetPeer(), mtproto.FromBool(in.GetTyping()))
	if err != nil {
		return nil, err
	}
	s.fanout(events)
	return mtproto.BoolTrue, nil
}

// MessagesReadEncryptedHistory
// messages.readEncryptedHistory#7f4b690a peer:InputEncryptedChat max_date:int = Bool;
// https://core.telegram.org/method/messages.readEncryptedHistory
func (s *Service) MessagesReadEncryptedHistory(ctx context.Context, in *mtproto.TLMessagesReadEncryptedHistory) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	events, err := secretchat.Default().Read(md.GetUserId(), in.GetPeer(), in.GetMaxDate(), now())
	if err != nil {
		return nil, err
	}
	s.fanout(events)
	return mtproto.BoolTrue, nil
}

// MessagesSendEncrypted
// messages.sendEncrypted#44fa7a15 flags:# silent:flags.0?true peer:InputEncryptedChat random_id:long data:bytes = messages.SentEncryptedMessage;
// https://core.telegram.org/method/messages.sendEncrypted
func (s *Service) MessagesSendEncrypted(ctx context.Context, in *mtproto.TLMessagesSendEncrypted) (*mtproto.Messages_SentEncryptedMessage, error) {
	return s.sendEncrypted(ctx, in.GetPeer(), in.GetRandomId(), in.GetData(), nil, false)
}

// MessagesSendEncryptedFile
// messages.sendEncryptedFile#5559481d flags:# silent:flags.0?true peer:InputEncryptedChat random_id:long data:bytes file:InputEncryptedFile = messages.SentEncryptedMessage;
// https://core.telegram.org/method/messages.sendEncryptedFile
func (s *Service) MessagesSendEncryptedFile(ctx context.Context, in *mtproto.TLMessagesSendEncryptedFile) (*mtproto.Messages_SentEncryptedMessage, error) {
	if _, err := s.md(ctx); err != nil {
		return nil, err
	}
	file, err := encryptedFile(in.GetFile())
	if err != nil {
		return nil, err
	}
	return s.sendEncrypted(ctx, in.GetPeer(), in.GetRandomId(), in.GetData(), file, false)
}

// MessagesSendEncryptedService
// messages.sendEncryptedService#32d439a4 peer:InputEncryptedChat random_id:long data:bytes = messages.SentEncryptedMessage;
// https://core.telegram.org/method/messages.sendEncryptedService
func (s *Service) MessagesSendEncryptedService(ctx context.Context, in *mtproto.TLMessagesSendEncryptedService) (*mtproto.Messages_SentEncryptedMessage, error) {
	return s.sendEncrypted(ctx, in.GetPeer(), in.GetRandomId(), in.GetData(), nil, true)
}

func (s *Service) sendEncrypted(ctx context.Context, peer *mtproto.InputEncryptedChat, randomID int64, data []byte, file *mtproto.EncryptedFile, service bool) (*mtproto.Messages_SentEncryptedMessage, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	other, err := secretchat.Default().Counterpart(md.GetUserId(), peer)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, mtproto.ErrDataInvalid
	}
	if len(data) > secretchat.MaxData {
		return nil, mtproto.ErrDataTooLong
	}
	if err := s.blockedBy(ctx, md.GetUserId(), other); err != nil {
		return nil, err
	}
	out, events, err := secretchat.Default().Send(md.GetUserId(), peer, randomID, data, file, service, now())
	if err != nil {
		return nil, err
	}
	s.fanout(events)
	return out, nil
}

// MessagesDeletePhoneCallHistory
// messages.deletePhoneCallHistory#f9cbe409 flags:# revoke:flags.0?true = messages.AffectedFoundMessages;
// https://core.telegram.org/method/messages.deletePhoneCallHistory
func (s *Service) MessagesDeletePhoneCallHistory(ctx context.Context, in *mtproto.TLMessagesDeletePhoneCallHistory) (*mtproto.Messages_AffectedFoundMessages, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	var deleter phonehistory.Deleter
	if s.svc != nil {
		deleter = s.svc.Msg
	}
	return phonehistory.Delete(ctx, deleter, md.GetUserId(), md.GetPermAuthKeyId(), in.GetRevoke())
}

func (s *Service) secretPeer(ctx context.Context, self int64, in *mtproto.InputUser) (int64, error) {
	if in == nil {
		return 0, mtproto.ErrUserIdInvalid
	}
	p := mtproto.FromInputUser(self, in)
	if p.PeerType == mtproto.PEER_SELF || p.PeerId == self || p.PeerType != mtproto.PEER_USER || p.PeerId <= 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	if s.svc == nil || s.svc.Users == nil {
		return p.PeerId, nil
	}
	users, err := s.svc.Users.UserGetMutableUsers(ctx, &user.TLUserGetMutableUsers{
		Id: []int64{p.PeerId},
		To: []int64{self},
	})
	if err != nil || users == nil {
		return 0, mtproto.ErrUserIdInvalid
	}
	u, ok := users.GetImmutableUser(p.PeerId)
	if !ok || u == nil || u.Id() == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	if u.Deleted() {
		return 0, mtproto.ErrInputUserDeactivated
	}
	if p.AccessHash != 0 && u.AccessHash() != 0 && p.AccessHash != u.AccessHash() {
		return 0, mtproto.ErrUserIdInvalid
	}
	if err := s.blockedBy(ctx, self, p.PeerId); err != nil {
		return 0, err
	}
	return p.PeerId, nil
}

func (s *Service) blockedBy(ctx context.Context, self, other int64) error {
	if s.svc == nil || s.svc.Users == nil || other == 0 {
		return nil
	}
	blocked, err := s.svc.Users.UserIsBlockedByUser(ctx, &user.TLUserIsBlockedByUser{
		UserId: other, PeerUserId: self,
	})
	if err == nil && mtproto.FromBool(blocked) {
		return mtproto.ErrForbiddenUserIsBlocked
	}
	return nil
}

func (s *Service) fanout(events []secretchat.Event) {
	for _, ev := range events {
		if ev.UserID == 0 || ev.Update == nil {
			continue
		}
		s.pushSecret(ev.UserID, mtproto.MakeUpdatesByUpdates(ev.Update))
	}
}

func (s *Service) pushSecret(userID int64, upd *mtproto.Updates) {
	if s.secretPush != nil {
		s.secretPush(userID, upd)
	}
	if s.svc == nil || s.svc.Sync == nil || upd == nil || userID == 0 {
		return
	}
	_, _ = s.svc.Sync.SyncPushUpdates(context.Background(), &sync.TLSyncPushUpdates{
		UserId: userID, Updates: upd,
	})
}

func now() int32 { return int32(time.Now().Unix()) }

// encryptedFile echoes an uploaded secret-chat file. The bytes themselves
// still depend on messages.uploadEncryptedFile.
func encryptedFile(in *mtproto.InputEncryptedFile) (*mtproto.EncryptedFile, error) {
	if in == nil || in.GetPredicateName() == "" || in.GetPredicateName() == mtproto.Predicate_inputEncryptedFileEmpty || in.GetId() == 0 {
		return nil, mtproto.ErrFileEmtpy
	}
	if in.GetPredicateName() == mtproto.Predicate_inputEncryptedFileUploaded && !md5Hex(in.GetMd5Checksum()) {
		return nil, mtproto.ErrCheckSumInvalid
	}
	return mtproto.MakeTLEncryptedFile(&mtproto.EncryptedFile{
		Id: in.GetId(), AccessHash: in.GetAccessHash(), DcId: 1,
		KeyFingerprint: in.GetKeyFingerprint(),
	}).To_EncryptedFile(), nil
}

func md5Hex(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if !unicode.Is(unicode.ASCII_Hex_Digit, c) {
			return false
		}
	}
	return true
}
