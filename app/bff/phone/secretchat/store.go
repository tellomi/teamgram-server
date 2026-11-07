// Package secretchat stores secret-chat state for the layer-232 encrypted RPCs.
//
// The server never sees the Diffie-Hellman secret and does not decrypt
// payloads. It checks g_a / g_b, remembers who the two users are, and
// forwards ciphertext. Chats and the qts queue are shared: a restart or
// another process reads the same backend. Live pushes still go out, and
// updates.getDifference returns anything the client has not applied.
//
// https://core.telegram.org/api/end-to-end
// https://core.telegram.org/api/updates
package secretchat

import (
	"crypto/rand"
	"encoding/binary"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

// MaxData is the largest encrypted blob messages.sendEncrypted* accepts.
// Larger payloads are DATA_TOO_LONG.
const MaxData = 64 * 1024

// Event is one update to push to a participant.
type Event struct {
	UserID int64
	Update *mtproto.Update
}

type phase int

const (
	phaseWaiting phase = iota
	phaseAccepted
	phaseDiscarded
)

type chat struct {
	id          int32
	access      int64
	date        int32
	admin       int64
	participant int64
	ga          []byte
	gb          []byte
	fingerprint int64
	phase       phase
	historyGone bool
}

type sentRec struct {
	date    int32
	file    *mtproto.EncryptedFile
	service bool
}

type sentKey struct {
	chat   int32
	random int64
}

// keptFile is an EncryptedFile produced by messages.uploadEncryptedFile.
// The bytes live in DFS; this record is what sendEncryptedFile attaches so
// the peer sees size, dc_id, and key_fingerprint.
type keptFile struct {
	file   *mtproto.EncryptedFile
	chatID int32
}

// Store is the secret-chat table for one backend. Two stores that share a
// backend are two processes: a request on one is visible to accept on the other.
type Store struct {
	mu      sync.Mutex
	be      Backend
	reports map[int32][]int64
}

var (
	gate    sync.Mutex
	active  Backend = newMem()
	current *Store
)

func newStore(be Backend) *Store {
	if be == nil {
		be = newMem()
	}
	return &Store{be: be, reports: map[int32][]int64{}}
}

func init() {
	current = newStore(active)
}

// New binds a store to backend. Tests use it for a second process.
func New(be Backend) *Store {
	return newStore(be)
}

// Default is the store the bff handlers share.
func Default() *Store {
	gate.Lock()
	defer gate.Unlock()
	if current == nil {
		current = newStore(active)
	}
	return current
}

// Reset drops in-memory chats. It does not delete MySQL rows.
func Reset() {
	gate.Lock()
	defer gate.Unlock()
	if m, ok := active.(*memBackend); ok {
		m.clear()
	}
	current = newStore(active)
}

// Request starts a secret chat. randomID is the chat id.
// https://core.telegram.org/method/messages.requestEncryption
func (s *Store) Request(admin, participant int64, randomID int32, ga []byte, now int32) (*mtproto.EncryptedChat, []Event, error) {
	if randomID == 0 || admin == 0 || participant == 0 || admin == participant {
		return nil, nil, mtproto.ErrChatIdInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.be.Chat(randomID); ok {
		return nil, nil, status.Error(mtproto.ErrBadRequest, "RANDOM_ID_DUPLICATE")
	}
	c := &chat{
		id: randomID, access: newAccess(), date: now,
		admin: admin, participant: participant, ga: clone(ga), phase: phaseWaiting,
	}
	if err := s.be.SaveChat(c); err != nil {
		return nil, nil, err
	}
	requested := mtproto.MakeTLEncryptedChatRequested(&mtproto.EncryptedChat{
		Id: c.id, AccessHash: c.access, Date: c.date,
		AdminId: c.admin, ParticipantId: c.participant, GA: clone(c.ga),
	}).To_EncryptedChat()
	ev, err := s.enqueue(participant, mtproto.MakeTLUpdateEncryption(&mtproto.Update{
		Chat: requested, Date_INT32: now,
	}).To_Update())
	if err != nil {
		_ = s.be.DeleteChat(c.id)
		return nil, nil, err
	}
	return waiting(c), []Event{ev}, nil
}

// Accept confirms the chat. The caller must be the participant.
// https://core.telegram.org/method/messages.acceptEncryption
func (s *Store) Accept(self int64, peer *mtproto.InputEncryptedChat, gb []byte, fingerprint int64, now int32) (*mtproto.EncryptedChat, []Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.member(self, peer)
	if err != nil {
		return nil, nil, err
	}
	if self != c.participant {
		return nil, nil, mtproto.ErrChatIdInvalid
	}
	switch c.phase {
	case phaseAccepted:
		return nil, nil, mtproto.ErrEncryptionAlreadyAccepted
	case phaseDiscarded:
		return nil, nil, mtproto.ErrEncryptionAlreadyDeclined
	}
	c.phase = phaseAccepted
	c.gb = clone(gb)
	c.fingerprint = fingerprint
	c.date = now
	if err = s.be.SaveChat(c); err != nil {
		return nil, nil, err
	}
	forAdmin := accepted(c, c.gb)
	ev, err := s.enqueue(c.admin, mtproto.MakeTLUpdateEncryption(&mtproto.Update{
		Chat: forAdmin, Date_INT32: now,
	}).To_Update())
	if err != nil {
		return nil, nil, err
	}
	return accepted(c, c.ga), []Event{ev}, nil
}

// Discard closes the chat for both participants.
// https://core.telegram.org/method/messages.discardEncryption
func (s *Store) Discard(self int64, chatID int32, deleteHistory bool, now int32) ([]Event, error) {
	if chatID == 0 {
		return nil, mtproto.ErrChatIdEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.be.Chat(chatID)
	if !ok || (self != c.admin && self != c.participant) {
		return nil, mtproto.ErrEncryptionIdInvalid
	}
	if c.phase == phaseDiscarded {
		return nil, mtproto.ErrEncryptionAlreadyDeclined
	}
	c.phase = phaseDiscarded
	c.historyGone = deleteHistory
	if err := s.be.SaveChat(c); err != nil {
		return nil, err
	}
	if deleteHistory {
		_ = s.be.DeleteSent(chatID)
	}
	other := c.participant
	if self == c.participant {
		other = c.admin
	}
	discarded := mtproto.MakeTLEncryptedChatDiscarded(&mtproto.EncryptedChat{
		Id: c.id, HistoryDeleted: deleteHistory,
	}).To_EncryptedChat()
	ev, err := s.enqueue(other, mtproto.MakeTLUpdateEncryption(&mtproto.Update{
		Chat: discarded, Date_INT32: now,
	}).To_Update())
	if err != nil {
		return nil, err
	}
	return []Event{ev}, nil
}

// DiscardAll closes every secret chat this user is in and drops the
// ciphertext the process still holds. Already-discarded chats are skipped,
// so a retried account deletion is safe. File ids are the encrypted-file
// objects those chats uploaded; the caller may trash them when nothing else
// references them.
func (s *Store) DiscardAll(userID int64, now int32) ([]Event, []int64) {
	if s == nil || userID == 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var events []Event
	var files []int64
	seen := map[int64]bool{}
	for _, c := range s.be.ChatsFor(userID) {
		if c == nil || c.phase == phaseDiscarded {
			continue
		}
		c.phase = phaseDiscarded
		c.historyGone = true
		if err := s.be.SaveChat(c); err != nil {
			continue
		}
		for _, id := range s.be.SentFiles(c.id) {
			if !seen[id] {
				seen[id] = true
				files = append(files, id)
			}
		}
		_ = s.be.DeleteSent(c.id)
		other := c.participant
		if userID == c.participant {
			other = c.admin
		}
		discarded := mtproto.MakeTLEncryptedChatDiscarded(&mtproto.EncryptedChat{
			Id: c.id, HistoryDeleted: true,
		}).To_EncryptedChat()
		ev, err := s.enqueue(other, mtproto.MakeTLUpdateEncryption(&mtproto.Update{
			Chat: discarded, Date_INT32: now,
		}).To_Update())
		if err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, files
}

// Typing tells the other participant that self is typing.
// https://core.telegram.org/method/messages.setEncryptedTyping
func (s *Store) Typing(self int64, peer *mtproto.InputEncryptedChat, typing bool) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.live(self, peer)
	if err != nil {
		return nil, err
	}
	if !typing {
		return nil, nil
	}
	return []Event{{
		UserID: c.other(self),
		Update: mtproto.MakeTLUpdateEncryptedChatTyping(&mtproto.Update{
			ChatId_INT32: c.id,
		}).To_Update(),
	}}, nil
}

// Read marks ciphertext up to maxDate as read and tells the other side.
// https://core.telegram.org/method/messages.readEncryptedHistory
func (s *Store) Read(self int64, peer *mtproto.InputEncryptedChat, maxDate, now int32) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.live(self, peer)
	if err != nil {
		return nil, err
	}
	ev, err := s.enqueue(c.other(self), mtproto.MakeTLUpdateEncryptedMessagesRead(&mtproto.Update{
		ChatId_INT32: c.id, MaxDate: maxDate, Date_INT32: now,
	}).To_Update())
	if err != nil {
		return nil, err
	}
	return []Event{ev}, nil
}

// Send forwards an encrypted payload. service selects encryptedMessageService.
// file is set for messages.sendEncryptedFile.
// https://core.telegram.org/method/messages.sendEncrypted
func (s *Store) Send(self int64, peer *mtproto.InputEncryptedChat, randomID int64, data []byte, file *mtproto.EncryptedFile, service bool, now int32) (*mtproto.Messages_SentEncryptedMessage, []Event, error) {
	if randomID == 0 {
		return nil, nil, mtproto.ErrRandomIdEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.member(self, peer)
	if err != nil {
		return nil, nil, err
	}
	if c.phase == phaseDiscarded {
		return nil, nil, mtproto.ErrEncryptionDeclined
	}
	if c.phase != phaseAccepted {
		return nil, nil, mtproto.ErrChatIdInvalid
	}
	key := sentKey{chat: c.id, random: randomID}
	if prev, ok := s.be.Sent(key); ok {
		return prev.reply(), nil, nil
	}
	rec := sentRec{date: now, file: cloneFile(file), service: service}
	if err = s.be.SaveSent(key, rec); err != nil {
		return nil, nil, err
	}
	msg := encryptedMessage(c.id, randomID, now, data, file, service)
	ev, err := s.enqueue(c.other(self), mtproto.MakeTLUpdateNewEncryptedMessage(&mtproto.Update{
		Message_ENCRYPTEDMESSAGE: msg,
	}).To_Update())
	if err != nil {
		return nil, nil, err
	}
	return rec.reply(), []Event{ev}, nil
}

// RememberEncrypted keeps the EncryptedFile returned by
// messages.uploadEncryptedFile. chatID is the secret chat it was bound to.
func (s *Store) RememberEncrypted(chatID int32, file *mtproto.EncryptedFile) {
	if s == nil || chatID == 0 || file == nil || file.GetId() == 0 {
		return
	}
	cloned := cloneFile(file)
	if cloned == nil {
		return
	}
	s.mu.Lock()
	_ = s.be.SaveFile(file.GetId(), keptFile{file: cloned, chatID: chatID})
	s.mu.Unlock()
}

// LookupEncrypted returns a stored encrypted file and the chat it belongs to.
func (s *Store) LookupEncrypted(id int64) (*mtproto.EncryptedFile, int32, bool) {
	if s == nil || id == 0 {
		return nil, 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.be.File(id)
	if !ok || rec.file == nil {
		return nil, 0, false
	}
	return cloneFile(rec.file), rec.chatID, true
}

// PeerUser is the other member. The chat may still be waiting.
func (s *Store) PeerUser(self int64, peer *mtproto.InputEncryptedChat) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.member(self, peer)
	if err != nil {
		return 0, err
	}
	return c.other(self), nil
}

// Counterpart is the other member of an accepted chat.
func (s *Store) Counterpart(self int64, peer *mtproto.InputEncryptedChat) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.member(self, peer)
	if err != nil {
		return 0, err
	}
	if c.phase == phaseDiscarded {
		return 0, mtproto.ErrEncryptionDeclined
	}
	if c.phase != phaseAccepted {
		return 0, mtproto.ErrChatIdInvalid
	}
	return c.other(self), nil
}

// Report records a spam report from a participant.
// https://core.telegram.org/method/messages.reportEncryptedSpam
func (s *Store) Report(self int64, peer *mtproto.InputEncryptedChat) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.member(self, peer)
	if err != nil {
		return err
	}
	for _, id := range s.reports[c.id] {
		if id == self {
			return nil
		}
	}
	s.reports[c.id] = append(s.reports[c.id], self)
	return nil
}

func (s *Store) member(self int64, peer *mtproto.InputEncryptedChat) (*chat, error) {
	if peer == nil || peer.GetChatId() == 0 || self == 0 {
		return nil, mtproto.ErrChatIdInvalid
	}
	c, ok := s.be.Chat(peer.GetChatId())
	if !ok || c.access != peer.GetAccessHash() || (self != c.admin && self != c.participant) {
		return nil, mtproto.ErrChatIdInvalid
	}
	return c, nil
}

func (s *Store) enqueue(userID int64, upd *mtproto.Update) (Event, error) {
	if _, err := s.be.Append(userID, upd); err != nil {
		return Event{}, err
	}
	return Event{UserID: userID, Update: upd}, nil
}

func (s *Store) live(self int64, peer *mtproto.InputEncryptedChat) (*chat, error) {
	c, err := s.member(self, peer)
	if err != nil {
		return nil, err
	}
	if c.phase != phaseAccepted {
		return nil, mtproto.ErrChatIdInvalid
	}
	return c, nil
}

func (c *chat) other(self int64) int64 {
	if self == c.admin {
		return c.participant
	}
	return c.admin
}

func (r sentRec) reply() *mtproto.Messages_SentEncryptedMessage {
	if r.file != nil && !r.service {
		return mtproto.MakeTLMessagesSentEncryptedFile(&mtproto.Messages_SentEncryptedMessage{
			Date: r.date, File: r.file,
		}).To_Messages_SentEncryptedMessage()
	}
	return mtproto.MakeTLMessagesSentEncryptedMessage(&mtproto.Messages_SentEncryptedMessage{
		Date: r.date,
	}).To_Messages_SentEncryptedMessage()
}

func waiting(c *chat) *mtproto.EncryptedChat {
	return mtproto.MakeTLEncryptedChatWaiting(&mtproto.EncryptedChat{
		Id: c.id, AccessHash: c.access, Date: c.date,
		AdminId: c.admin, ParticipantId: c.participant,
	}).To_EncryptedChat()
}

func accepted(c *chat, gAOrB []byte) *mtproto.EncryptedChat {
	return mtproto.MakeTLEncryptedChat(&mtproto.EncryptedChat{
		Id: c.id, AccessHash: c.access, Date: c.date,
		AdminId: c.admin, ParticipantId: c.participant,
		GAOrB: clone(gAOrB), KeyFingerprint: c.fingerprint,
	}).To_EncryptedChat()
}

func encryptedMessage(chatID int32, randomID int64, date int32, data []byte, file *mtproto.EncryptedFile, service bool) *mtproto.EncryptedMessage {
	if service {
		return mtproto.MakeTLEncryptedMessageService(&mtproto.EncryptedMessage{
			RandomId: randomID, ChatId: chatID, Date: date, Bytes: clone(data),
		}).To_EncryptedMessage()
	}
	if file == nil {
		file = mtproto.MakeTLEncryptedFileEmpty(&mtproto.EncryptedFile{}).To_EncryptedFile()
	}
	return mtproto.MakeTLEncryptedMessage(&mtproto.EncryptedMessage{
		RandomId: randomID, ChatId: chatID, Date: date, Bytes: clone(data), File: file,
	}).To_EncryptedMessage()
}

func clone(b []byte) []byte {
	if len(b) == 0 {
		return []byte{}
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func cloneFile(f *mtproto.EncryptedFile) *mtproto.EncryptedFile {
	if f == nil || f.GetPredicateName() == mtproto.Predicate_encryptedFileEmpty {
		return nil
	}
	return mtproto.MakeTLEncryptedFile(&mtproto.EncryptedFile{
		Id: f.GetId(), AccessHash: f.GetAccessHash(), DcId: f.GetDcId(),
		KeyFingerprint: f.GetKeyFingerprint(),
		Size2_INT64:    f.GetSize2_INT64(),
		Size2_INT32:    f.GetSize2_INT32(),
	}).To_EncryptedFile()
}

func newAccess() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	v := int64(binary.BigEndian.Uint64(b[:]))
	if v == 0 {
		return 1
	}
	return v
}
