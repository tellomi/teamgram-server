package accountrest

import (
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// Usernames associated with the logged-in user.
// https://core.telegram.org/method/account.reorderUsernames
// https://core.telegram.org/method/account.toggleUsername
//
// Active names are ordered. The first active name is the primary link.
// reorder must list every active name. toggle changes one purchased or
// editable name that already belongs to the user.

const maxActiveUsernames = 10

type usernameRec struct {
	name     string
	active   bool
	editable bool
}

type usernameStore struct {
	mu    sync.Mutex
	users map[int64][]usernameRec
}

var usernames = &usernameStore{users: map[int64][]usernameRec{}}

func resetUsernames() {
	usernames.mu.Lock()
	usernames.users = map[int64][]usernameRec{}
	usernames.mu.Unlock()
}

// SeedPrimary records the account's editable username when this process has
// not seen the user yet. Later toggles are not overwritten by a fresh read.
func SeedPrimary(userID int64, username string, active bool) {
	if userID == 0 {
		return
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return
	}
	usernames.mu.Lock()
	defer usernames.mu.Unlock()
	if len(usernames.users[userID]) > 0 {
		return
	}
	usernames.users[userID] = []usernameRec{{name: username, active: active, editable: true}}
}

// RememberPrimary records the editable username written by account.updateUsername.
// An empty name clears that slot.
func RememberPrimary(userID int64, username string) {
	if userID == 0 {
		return
	}
	username = strings.TrimSpace(username)
	usernames.mu.Lock()
	defer usernames.mu.Unlock()
	list := usernames.users[userID]
	next := make([]usernameRec, 0, len(list)+1)
	for _, row := range list {
		if row.editable {
			continue
		}
		next = append(next, row)
	}
	if username != "" {
		next = append([]usernameRec{{name: username, active: true, editable: true}}, next...)
	}
	if len(next) == 0 {
		delete(usernames.users, userID)
		return
	}
	usernames.users[userID] = next
}

// ToggleUsername activates or deactivates one of the user's usernames.
func ToggleUsername(userID int64, username string, active bool) (*mtproto.Bool, error) {
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	username = strings.TrimSpace(username)
	if username == "" || !user.CheckUsernameInvalid(username) {
		return nil, mtproto.ErrUsernameInvalid
	}
	usernames.mu.Lock()
	defer usernames.mu.Unlock()
	list := append([]usernameRec(nil), usernames.users[userID]...)
	idx := -1
	for i, row := range list {
		if strings.EqualFold(row.name, username) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, mtproto.ErrUsernameInvalid
	}
	if list[idx].active == active {
		return nil, mtproto.ErrUsernameNotModified
	}
	if active && activeUsernameCount(list) >= maxActiveUsernames {
		return nil, mtproto.ErrUsernamesActiveTooMuch
	}
	list[idx].active = active
	usernames.users[userID] = list
	return mtproto.BoolTrue, nil
}

// ReorderUsernames sets the order of every active username.
// A partial list is 400 ORDER_INVALID. An unchanged list is 400 USERNAME_NOT_MODIFIED.
func ReorderUsernames(userID int64, order []string) (*mtproto.Bool, error) {
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	usernames.mu.Lock()
	defer usernames.mu.Unlock()
	list := append([]usernameRec(nil), usernames.users[userID]...)
	var active, inactive []usernameRec
	for _, row := range list {
		if row.active {
			active = append(active, row)
		} else {
			inactive = append(inactive, row)
		}
	}
	if len(order) != len(active) {
		return nil, mtproto.ErrOrderInvalid
	}
	used := make([]bool, len(active))
	reordered := make([]usernameRec, 0, len(active))
	for _, name := range order {
		found := -1
		for i, row := range active {
			if !used[i] && strings.EqualFold(row.name, strings.TrimSpace(name)) {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, mtproto.ErrOrderInvalid
		}
		used[found] = true
		reordered = append(reordered, active[found])
	}
	same := true
	for i := range reordered {
		if !strings.EqualFold(reordered[i].name, active[i].name) {
			same = false
			break
		}
	}
	if same {
		return nil, mtproto.ErrUsernameNotModified
	}
	usernames.users[userID] = append(reordered, inactive...)
	return mtproto.BoolTrue, nil
}

func activeUsernameCount(list []usernameRec) int {
	n := 0
	for _, row := range list {
		if row.active {
			n++
		}
	}
	return n
}
