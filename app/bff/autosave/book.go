// Package autosave stores account.getAutoSaveSettings.
//
// https://core.telegram.org/method/account.getAutoSaveSettings
// https://core.telegram.org/method/account.saveAutoSaveSettings
// https://core.telegram.org/method/account.deleteAutoSaveExceptions
package autosave

import (
	"sync"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// A fresh account saves photos and videos in every chat, with no size cap.
// That is the setting Telegram mobile applies when the server has not stored
// an exception yet: both media flags set, video_max_size omitted.
func defaultSettings() *mtproto.AutoSaveSettings {
	return mtproto.MakeTLAutoSaveSettings(&mtproto.AutoSaveSettings{
		Photos: true,
		Videos: true,
	}).To_AutoSaveSettings()
}

type peerKey struct {
	kind string
	id   int64
}

type account struct {
	users      *mtproto.AutoSaveSettings
	chats      *mtproto.AutoSaveSettings
	broadcasts *mtproto.AutoSaveSettings
	except     map[peerKey]*mtproto.AutoSaveSettings
	order      []peerKey
}

type book struct {
	mu    sync.Mutex
	users map[int64]*account
}

var shared = &book{users: map[int64]*account{}}

// Reset clears process state. Tests call it.
func Reset() {
	shared.mu.Lock()
	shared.users = map[int64]*account{}
	shared.mu.Unlock()
}

func (b *book) acc(id int64) *account {
	a := b.users[id]
	if a == nil {
		a = &account{
			users:      defaultSettings(),
			chats:      defaultSettings(),
			broadcasts: defaultSettings(),
			except:     map[peerKey]*mtproto.AutoSaveSettings{},
		}
		b.users[id] = a
	}
	return a
}

// Get returns the stored autosave settings for self.
func Get(self int64) (*mtproto.Account_AutoSaveSettings, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	shared.mu.Lock()
	defer shared.mu.Unlock()
	a := shared.acc(self)
	exceptions := make([]*mtproto.AutoSaveException, 0, len(a.order))
	var users []*mtproto.User
	var chats []*mtproto.Chat
	for _, key := range a.order {
		settings := a.except[key]
		if settings == nil {
			continue
		}
		peer := peerOf(key)
		exceptions = append(exceptions, mtproto.MakeTLAutoSaveException(&mtproto.AutoSaveException{
			Peer:     peer,
			Settings: copySettings(settings),
		}).To_AutoSaveException())
		switch key.kind {
		case "user":
			users = append(users, mtproto.MakeTLUser(&mtproto.User{
				Id:  key.id,
				Min: true,
			}).To_User())
		case "chat":
			chats = append(chats, mtproto.MakeTLChat(&mtproto.Chat{Id: key.id}).To_Chat())
		case "channel":
			chats = append(chats, mtproto.MakeTLChannel(&mtproto.Chat{Id: key.id}).To_Chat())
		}
	}
	if users == nil {
		users = []*mtproto.User{}
	}
	if chats == nil {
		chats = []*mtproto.Chat{}
	}
	return mtproto.MakeTLAccountAutoSaveSettings(&mtproto.Account_AutoSaveSettings{
		UsersSettings:      copySettings(a.users),
		ChatsSettings:      copySettings(a.chats),
		BroadcastsSettings: copySettings(a.broadcasts),
		Exceptions:         exceptions,
		Chats:              chats,
		Users:              users,
	}).To_Account_AutoSaveSettings(), nil
}

// Save applies settings to the categories named by the flags, or to one peer.
// https://core.telegram.org/method/account.saveAutoSaveSettings
func Save(self int64, in *mtproto.TLAccountSaveAutoSaveSettings) (*mtproto.Bool, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	settings, err := cleanSettings(in.GetSettings())
	if err != nil {
		return nil, err
	}
	shared.mu.Lock()
	defer shared.mu.Unlock()
	a := shared.acc(self)
	if in.GetPeer() != nil {
		key, err := peerKeyOf(self, in.GetPeer())
		if err != nil {
			return nil, err
		}
		if _, ok := a.except[key]; !ok {
			a.order = append(a.order, key)
		}
		a.except[key] = settings
		return mtproto.BoolTrue, nil
	}
	if !in.GetUsers() && !in.GetChats() && !in.GetBroadcasts() {
		// The method page's only 400 is PEER_ID_INVALID. A save that names
		// neither a peer nor a users/chats/broadcasts scope has no target.
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetUsers() {
		a.users = copySettings(settings)
	}
	if in.GetChats() {
		a.chats = copySettings(settings)
	}
	if in.GetBroadcasts() {
		a.broadcasts = copySettings(settings)
	}
	return mtproto.BoolTrue, nil
}

// DeleteExceptions removes every per-peer override.
// https://core.telegram.org/method/account.deleteAutoSaveExceptions
func DeleteExceptions(self int64) (*mtproto.Bool, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	shared.mu.Lock()
	defer shared.mu.Unlock()
	a := shared.acc(self)
	a.except = map[peerKey]*mtproto.AutoSaveSettings{}
	a.order = nil
	return mtproto.BoolTrue, nil
}

func cleanSettings(in *mtproto.AutoSaveSettings) (*mtproto.AutoSaveSettings, error) {
	if in == nil {
		return mtproto.MakeTLAutoSaveSettings(&mtproto.AutoSaveSettings{}).To_AutoSaveSettings(), nil
	}
	out := mtproto.MakeTLAutoSaveSettings(&mtproto.AutoSaveSettings{
		Photos: in.GetPhotos(),
		Videos: in.GetVideos(),
	}).To_AutoSaveSettings()
	if in.GetVideoMaxSize() != nil {
		n := in.GetVideoMaxSize().GetValue()
		if n < 0 {
			return nil, mtproto.ErrMediaInvalid
		}
		out.VideoMaxSize = wrapperspb.Int64(n)
	}
	return out, nil
}

func copySettings(in *mtproto.AutoSaveSettings) *mtproto.AutoSaveSettings {
	if in == nil {
		return defaultSettings()
	}
	out := mtproto.MakeTLAutoSaveSettings(&mtproto.AutoSaveSettings{
		Photos: in.GetPhotos(),
		Videos: in.GetVideos(),
	}).To_AutoSaveSettings()
	if in.GetVideoMaxSize() != nil {
		out.VideoMaxSize = wrapperspb.Int64(in.GetVideoMaxSize().GetValue())
	}
	return out
}

func peerKeyOf(self int64, in *mtproto.InputPeer) (peerKey, error) {
	if in == nil {
		return peerKey{}, mtproto.ErrPeerIdInvalid
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		if self == 0 {
			return peerKey{}, mtproto.ErrPeerIdInvalid
		}
		return peerKey{kind: "user", id: self}, nil
	case mtproto.Predicate_inputPeerUser:
		if in.GetUserId() == 0 {
			return peerKey{}, mtproto.ErrPeerIdInvalid
		}
		return peerKey{kind: "user", id: in.GetUserId()}, nil
	case mtproto.Predicate_inputPeerChat:
		if in.GetChatId() == 0 {
			return peerKey{}, mtproto.ErrPeerIdInvalid
		}
		return peerKey{kind: "chat", id: in.GetChatId()}, nil
	case mtproto.Predicate_inputPeerChannel:
		if in.GetChannelId() == 0 {
			return peerKey{}, mtproto.ErrPeerIdInvalid
		}
		return peerKey{kind: "channel", id: in.GetChannelId()}, nil
	default:
		return peerKey{}, mtproto.ErrPeerIdInvalid
	}
}

func peerOf(key peerKey) *mtproto.Peer {
	switch key.kind {
	case "chat":
		return mtproto.MakePeerChat(key.id)
	case "channel":
		return mtproto.MakePeerChannel(key.id)
	default:
		return mtproto.MakePeerUser(key.id)
	}
}
