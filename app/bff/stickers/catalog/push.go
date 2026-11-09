package catalog

import (
	"context"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	pushBatch = 40
	pushGap   = 25 * time.Millisecond
)

// UserPush delivers one updates payload to every session of a user.
// SyncBroadcastUpdates only fans out to chat members, so the catalog
// pushes per user instead.
type UserPush interface {
	PushUser(ctx context.Context, userID int64, updates *mtproto.Updates) error
}

// SetUserPush installs the sync client used when a reload changes packs.
// A nil push leaves the catalog readable and skips the broadcast.
func (s *Service) SetUserPush(p UserPush) {
	if s == nil {
		return
	}
	s.broadcastMu.Lock()
	s.userPush = p
	s.broadcastMu.Unlock()
}

func (s *Service) noteCatalogChange(kinds kindFlags) {
	if s == nil || !kinds.any() {
		return
	}
	s.broadcastMu.Lock()
	push := s.userPush
	if push == nil {
		s.broadcastMu.Unlock()
		return
	}
	s.pendingKinds.add(kinds)
	start := !s.broadcasting
	s.broadcasting = true
	s.broadcastMu.Unlock()
	if start {
		go s.broadcastLoop()
	}
}

func (s *Service) broadcastLoop() {
	ctx := context.Background()
	for {
		s.broadcastMu.Lock()
		kinds := s.pendingKinds
		s.pendingKinds = kindFlags{}
		push := s.userPush
		if !kinds.any() || push == nil {
			s.broadcasting = false
			s.broadcastMu.Unlock()
			return
		}
		s.broadcastMu.Unlock()
		users, err := s.listRecentUsers(ctx)
		if err != nil {
			logx.Errorf("stickers recent sessions: %v", err)
			time.Sleep(time.Second)
			users, err = s.listRecentUsers(ctx)
		}
		if err != nil {
			logx.Errorf("stickers recent sessions: %v", err)
			continue
		}
		if len(users) == 0 {
			continue
		}
		s.broadcastKinds(ctx, push, kinds, users)
	}
}

func (s *Service) listRecentUsers(ctx context.Context) ([]int64, error) {
	if s != nil && s.recentUserFn != nil {
		return s.recentUserFn(ctx)
	}
	mp, ok := s.mysql()
	if !ok {
		return nil, nil
	}
	since := time.Now().Add(-recentSessions).Unix()
	rows, err := mp.db.QueryContext(ctx, `
SELECT user_id FROM auth_users
WHERE deleted = 0 AND user_id > 0 AND date_active >= ?
GROUP BY user_id
ORDER BY MAX(date_active) DESC
LIMIT ?`, since, recentUserCap)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "date_active") {
		rows, err = mp.db.QueryContext(ctx, `
SELECT user_id FROM auth_users
WHERE deleted = 0 AND user_id > 0 AND date_actived >= ?
GROUP BY user_id
ORDER BY MAX(date_actived) DESC
LIMIT ?`, since, recentUserCap)
	}
	if err != nil {
		if isMissingTable(err, "auth_users") {
			logx.Infof("stickers recent sessions: auth_users is not in this database")
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		if id > 0 {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

func isMissingTable(err error, table string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "does not exist") {
		return true
	}
	return strings.Contains(msg, table) && strings.Contains(msg, "1146")
}

func (s *Service) broadcastKinds(ctx context.Context, push UserPush, kinds kindFlags, users []int64) {
	if push == nil || len(users) == 0 {
		return
	}
	var updates []*mtproto.Updates
	if kinds.regular {
		updates = append(updates, StickerSetsChanged(false, false))
	}
	if kinds.masks {
		updates = append(updates, StickerSetsChanged(true, false))
	}
	if kinds.emojis {
		updates = append(updates, StickerSetsChanged(false, true))
	}
	logx.Infof("stickers push updateStickerSets (%d) to %d recent users", len(updates), len(users))
	fanOutUsers(ctx, push, users, updates, pushBatch, pushGap)
}

func fanOutUsers(ctx context.Context, push UserPush, users []int64, updates []*mtproto.Updates, batch int, gap time.Duration) {
	if push == nil || batch < 1 {
		return
	}
	n := 0
	for _, up := range updates {
		if up == nil {
			continue
		}
		for _, id := range users {
			if id == 0 {
				continue
			}
			if err := push.PushUser(ctx, id, up); err != nil {
				logx.Errorf("stickers push user %d: %v", id, err)
			}
			n++
			if gap > 0 && n%batch == 0 {
				time.Sleep(gap)
			}
		}
	}
}

// StickerInstallUpdate is updateNewStickerSet for the user's other sessions.
func StickerInstallUpdate(set *mtproto.Messages_StickerSet) *mtproto.Updates {
	if set == nil {
		return nil
	}
	up := mtproto.MakeTLUpdateNewStickerSet(&mtproto.Update{})
	up.SetStickerset_MESSAGES_STICKERSET(set)
	return mtproto.MakeUpdatesByUpdates(up.To_Update())
}

// StickerSetsChanged is updateStickerSets. masks and emojis select the list
// the client refetches. Both false is the regular sticker list.
func StickerSetsChanged(masks, emojis bool) *mtproto.Updates {
	up := mtproto.MakeTLUpdateStickerSets(&mtproto.Update{})
	up.SetMasks(masks)
	up.SetEmojis(emojis)
	return mtproto.MakeUpdatesByUpdates(up.To_Update())
}

// StickerOrderUpdate is updateStickerSetsOrder.
func StickerOrderUpdate(masks, emojis bool, order []int64) *mtproto.Updates {
	if len(order) == 0 {
		return nil
	}
	up := mtproto.MakeTLUpdateStickerSetsOrder(&mtproto.Update{})
	up.SetMasks(masks)
	up.SetEmojis(emojis)
	up.SetOrder_VECTORINT64(append([]int64(nil), order...))
	return mtproto.MakeUpdatesByUpdates(up.To_Update())
}

// NotifyInstall builds updateNewStickerSet after a successful install.
// System sets are not installed into the user list, so they produce no update.
func (s *Service) NotifyInstall(userID int64, in *mtproto.InputStickerSet) *mtproto.Updates {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.lookup(in)
	if err != nil || p.system() {
		return nil
	}
	on, archived, _ := s.installed(userID, p)
	if !on {
		return nil
	}
	return StickerInstallUpdate(p.full(on, archived))
}

// NotifySets builds one updateStickerSets per list touched by the packs.
func (s *Service) NotifySets(sets []*mtproto.InputStickerSet) []*mtproto.Updates {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kinds kindFlags
	for _, in := range sets {
		p, err := s.lookup(in)
		if err != nil {
			continue
		}
		regular, masks, emojis := listKind(p)
		if regular {
			kinds.regular = true
		}
		if masks {
			kinds.masks = true
		}
		if emojis {
			kinds.emojis = true
		}
	}
	return kinds.updates()
}

func (k kindFlags) updates() []*mtproto.Updates {
	var out []*mtproto.Updates
	if k.regular {
		out = append(out, StickerSetsChanged(false, false))
	}
	if k.masks {
		out = append(out, StickerSetsChanged(true, false))
	}
	if k.emojis {
		out = append(out, StickerSetsChanged(false, true))
	}
	return out
}
