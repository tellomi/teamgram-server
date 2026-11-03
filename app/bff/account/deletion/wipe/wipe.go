// Package wipe removes an account the way Telegram does: private history
// disappears on both sides, the user leaves groups and channels, profile
// fields and personal settings are cleared, and group messages stay so the
// sender shows as a deleted account.
//
// Every step is safe to run again. Message and membership updates are short
// batches, so a large account does not hold a table lock. Object bytes are
// not deleted here; they are queued for the garbage collector in gc.go.
package wipe

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/jsonx"
)

const DefaultBatch = 200

// Peer is one dialog. Type is mtproto.PEER_*.
type Peer struct {
	Type int32
	ID   int64
}

// Box is one message in a user's box.
type Box struct {
	ID         int32
	PhotoID    int64
	DocumentID int64
}

// LeftNotice is one basic group or channel the user just left.
type LeftNotice struct {
	ChatID      int64
	Channel     bool
	Version     int32
	Members     []int64
	Deactivated bool
	Successor   int64
}

// Object is one stored file to consider for trash.
type Object struct {
	Kind  string // photo, document, avatar, encrypted
	ID    int64
	Class string
	Path  string
}

// SecretEvent is an update for the other participant of a secret chat.
type SecretEvent struct {
	UserID int64
	Update *mtproto.Update
}

// Store is the database side of a wipe. Implementations must be retriable:
// deleting an already-deleted row is success.
type Store interface {
	PrivatePeers(ctx context.Context, userID int64) ([]Peer, error)
	Boxes(ctx context.Context, userID int64, peer Peer, after int32, limit int) ([]Box, error)
	CommitDelete(ctx context.Context, userID int64, ids []int32, pts, ptsCount, updateType int32, updateJSON []byte) error
	DeleteDialog(ctx context.Context, userID int64, peer Peer) error
	AllocPts(ctx context.Context, userID int64, n int32) (int32, error)

	BasicGroups(ctx context.Context, userID int64) ([]int64, error)
	LeaveBasic(ctx context.Context, userID, chatID, now int64) (LeftNotice, error)
	LeaveChannels(ctx context.Context, userID, now int64) ([]LeftNotice, error)

	ClearProfile(ctx context.Context, userID int64, username string) (avatarPhotos []int64, err error)
	DeleteContacts(ctx context.Context, userID int64) (otherOwners []int64, err error)
	DeleteSettings(ctx context.Context, userID int64) error

	PhotoObjects(ctx context.Context, photoID int64, avatar bool) ([]Object, error)
	DocumentObjects(ctx context.Context, documentID int64) ([]Object, error)
	EncryptedObjects(ctx context.Context, fileID int64) ([]Object, error)
	Enqueue(ctx context.Context, userID int64, objs []Object) error
}

// Notifier pushes updates to online clients.
type Notifier interface {
	Push(ctx context.Context, userID int64, updates []*mtproto.Update) error
}

// Cache drops keys so the next read sees the wiped rows.
type Cache interface {
	Del(keys ...string) (int, error)
}

// Engine runs one account wipe.
type Engine struct {
	Store Store
	// Channels runs the channel service leave when that service is open.
	// The SQL leave still runs afterwards and does nothing if the rows
	// already say the user has left.
	Channels func(ctx context.Context, userID int64) error
	Secrets  func(ctx context.Context, userID int64) ([]SecretEvent, []int64, error)
	Notify   Notifier
	Pts      func(ctx context.Context, userID int64, n int32) (int32, error)
	Cache    Cache
	Batch    int
	Now      func() int64
}

// Request identifies the account. Phone and Username are the values from
// before the row is cleared, so caches and the username table can be dropped.
type Request struct {
	UserID   int64
	Phone    string
	Username string
}

// Result is what one successful wipe changed. Tests assert on it.
type Result struct {
	PrivatePeers int
	Boxes        int
	GroupsLeft   int
	ChannelsLeft int
	Deactivated  int
	Contacts     int
	Avatars      int
	Queued       int
	SecretChats  int
}

func (e *Engine) batch() int {
	if e == nil || e.Batch <= 0 {
		return DefaultBatch
	}
	return e.Batch
}

func (e *Engine) now() int64 {
	if e != nil && e.Now != nil {
		return e.Now()
	}
	return time.Now().Unix()
}

func (e *Engine) alloc(ctx context.Context, userID int64, n int32) (int32, error) {
	if n <= 0 {
		return 0, nil
	}
	if e.Pts != nil {
		end, err := e.Pts(ctx, userID, n)
		if err == nil && end != 0 {
			return end, nil
		}
	}
	return e.Store.AllocPts(ctx, userID, n)
}

// Run wipes userID. It returns an error if a step fails; the caller retries
// the whole wipe. Steps that already finished stay finished.
func (e *Engine) Run(ctx context.Context, req Request) (Result, error) {
	var out Result
	if e == nil || e.Store == nil || req.UserID == 0 {
		return out, fmt.Errorf("account wipe is not configured")
	}
	if e.Secrets != nil {
		events, files, err := e.Secrets(ctx, req.UserID)
		if err != nil {
			return out, err
		}
		out.SecretChats = len(events)
		for _, ev := range events {
			if ev.Update != nil && ev.UserID != 0 {
				_ = e.push(ctx, ev.UserID, ev.Update)
			}
		}
		var objs []Object
		for _, id := range files {
			got, err := e.Store.EncryptedObjects(ctx, id)
			if err != nil {
				return out, err
			}
			objs = append(objs, got...)
		}
		if err = e.queue(ctx, req.UserID, objs, &out); err != nil {
			return out, err
		}
	}
	peers, err := e.Store.PrivatePeers(ctx, req.UserID)
	if err != nil {
		return out, err
	}
	var touched []Peer
	for _, peer := range peers {
		n, err := e.wipePeer(ctx, req.UserID, req.UserID, peer, &out)
		if err != nil {
			return out, err
		}
		out.Boxes += n
		touched = append(touched, peer)
		out.PrivatePeers++
		if peer.Type == mtproto.PEER_USER && peer.ID != req.UserID && peer.ID != 0 {
			pn, err := e.wipePeer(ctx, peer.ID, req.UserID, Peer{Type: mtproto.PEER_USER, ID: req.UserID}, &out)
			if err != nil {
				return out, err
			}
			out.Boxes += pn
			if err = e.Store.DeleteDialog(ctx, peer.ID, Peer{Type: mtproto.PEER_USER, ID: req.UserID}); err != nil {
				return out, err
			}
			touched = append(touched, Peer{Type: mtproto.PEER_USER, ID: peer.ID})
		}
		if err = e.Store.DeleteDialog(ctx, req.UserID, peer); err != nil {
			return out, err
		}
	}
	groups, err := e.Store.BasicGroups(ctx, req.UserID)
	if err != nil {
		return out, err
	}
	var chatIDs []int64
	for _, id := range groups {
		notice, err := e.Store.LeaveBasic(ctx, req.UserID, id, e.now())
		if err != nil {
			return out, err
		}
		if notice.ChatID == 0 {
			continue
		}
		out.GroupsLeft++
		if notice.Deactivated {
			out.Deactivated++
		}
		chatIDs = append(chatIDs, notice.ChatID)
		e.pushLeft(ctx, req.UserID, notice)
		_ = e.Store.DeleteDialog(ctx, req.UserID, Peer{Type: mtproto.PEER_CHAT, ID: id})
	}
	if e.Channels != nil {
		if err = e.Channels(ctx, req.UserID); err != nil {
			return out, err
		}
	}
	channelNotices, err := e.Store.LeaveChannels(ctx, req.UserID, e.now())
	if err != nil {
		return out, err
	}
	for _, notice := range channelNotices {
		out.ChannelsLeft++
		if notice.Deactivated {
			out.Deactivated++
		}
		chatIDs = append(chatIDs, notice.ChatID)
		e.pushLeft(ctx, req.UserID, notice)
		_ = e.Store.DeleteDialog(ctx, req.UserID, Peer{Type: mtproto.PEER_CHANNEL, ID: notice.ChatID})
	}
	photos, err := e.Store.ClearProfile(ctx, req.UserID, req.Username)
	if err != nil {
		return out, err
	}
	out.Avatars = len(photos)
	var avatarObjs []Object
	for _, id := range photos {
		got, err := e.Store.PhotoObjects(ctx, id, true)
		if err != nil {
			return out, err
		}
		avatarObjs = append(avatarObjs, got...)
	}
	if err = e.queue(ctx, req.UserID, avatarObjs, &out); err != nil {
		return out, err
	}
	owners, err := e.Store.DeleteContacts(ctx, req.UserID)
	if err != nil {
		return out, err
	}
	out.Contacts = len(owners)
	if err = e.Store.DeleteSettings(ctx, req.UserID); err != nil {
		return out, err
	}
	if e.Cache != nil {
		keys := cacheKeys(req, touched, owners, chatIDs)
		if len(keys) > 0 {
			_, _ = e.Cache.Del(keys...)
		}
	}
	return out, nil
}

// wipePeer deletes one user's copy of a private dialog in batches.
// deletedUser is the account being removed; media is queued under that id
// before the rows are marked deleted.
func (e *Engine) wipePeer(ctx context.Context, boxOwner, deletedUser int64, peer Peer, out *Result) (int, error) {
	var n int
	after := int32(0)
	for {
		boxes, err := e.Store.Boxes(ctx, boxOwner, peer, after, e.batch())
		if err != nil {
			return n, err
		}
		if len(boxes) == 0 {
			return n, nil
		}
		ids := make([]int32, len(boxes))
		var objs []Object
		for i, b := range boxes {
			ids[i] = b.ID
			after = b.ID
			if b.PhotoID != 0 {
				got, err := e.Store.PhotoObjects(ctx, b.PhotoID, false)
				if err != nil {
					return n, err
				}
				objs = append(objs, got...)
			}
			if b.DocumentID != 0 {
				got, err := e.Store.DocumentObjects(ctx, b.DocumentID)
				if err != nil {
					return n, err
				}
				objs = append(objs, got...)
			}
		}
		if err = e.queue(ctx, deletedUser, objs, out); err != nil {
			return n, err
		}
		end, err := e.alloc(ctx, boxOwner, int32(len(ids)))
		if err != nil {
			return n, err
		}
		upd := mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{
			Messages:  ids,
			Pts_INT32: end,
			PtsCount:  int32(len(ids)),
		}).To_Update()
		raw, err := jsonx.Marshal(upd)
		if err != nil {
			raw, err = json.Marshal(upd)
			if err != nil {
				return n, err
			}
		}
		if err = e.Store.CommitDelete(ctx, boxOwner, ids, end, int32(len(ids)), mtproto.GetUpdateType(upd), raw); err != nil {
			return n, err
		}
		_ = e.push(ctx, boxOwner, upd)
		n += len(ids)
		if len(boxes) < e.batch() {
			return n, nil
		}
	}
}

func (e *Engine) queue(ctx context.Context, userID int64, objs []Object, out *Result) error {
	if len(objs) == 0 {
		return nil
	}
	if err := e.Store.Enqueue(ctx, userID, objs); err != nil {
		return err
	}
	if out != nil {
		out.Queued += len(objs)
	}
	return nil
}

func (e *Engine) push(ctx context.Context, userID int64, updates ...*mtproto.Update) error {
	if e.Notify == nil || userID == 0 || len(updates) == 0 {
		return nil
	}
	return e.Notify.Push(ctx, userID, updates)
}

func (e *Engine) pushLeft(ctx context.Context, userID int64, notice LeftNotice) {
	if notice.ChatID == 0 {
		return
	}
	var upd *mtproto.Update
	if notice.Channel {
		upd = mtproto.MakeTLUpdateChannelParticipant(&mtproto.Update{
			ChannelId:  notice.ChatID,
			Date_INT32: int32(e.now()),
			ActorId:    userID,
			UserId:     userID,
		}).To_Update()
	} else {
		upd = mtproto.MakeTLUpdateChatParticipantDelete(&mtproto.Update{
			ChatId_INT64: notice.ChatID,
			UserId:       userID,
			Version:      notice.Version,
		}).To_Update()
	}
	for _, member := range notice.Members {
		_ = e.push(ctx, member, upd)
	}
}

func cacheKeys(req Request, peers []Peer, owners, chats []int64) []string {
	keys := []string{
		fmt.Sprintf("user_data.2#%d", req.UserID),
		fmt.Sprintf("dialog_id_list.1#%d", req.UserID),
		fmt.Sprintf("pinned_dialog_id_list.1#%d", req.UserID),
	}
	if req.Phone != "" {
		keys = append(keys, "phone_user.1#"+req.Phone)
	}
	seen := map[string]bool{}
	add := func(k string) {
		if k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for _, p := range peers {
		add(fmt.Sprintf("dialog.1#%d_%d", req.UserID, p.ID))
		if p.Type == mtproto.PEER_USER && p.ID != req.UserID {
			add(fmt.Sprintf("user_data.2#%d", p.ID))
			add(fmt.Sprintf("dialog_id_list.1#%d", p.ID))
			add(fmt.Sprintf("dialog.1#%d_%d", p.ID, req.UserID))
			add(fmt.Sprintf("user_contact.2#%d_%d", req.UserID, p.ID))
			add(fmt.Sprintf("user_contact.2#%d_%d", p.ID, req.UserID))
		}
	}
	for _, owner := range owners {
		add(fmt.Sprintf("user_data.2#%d", owner))
		add(fmt.Sprintf("user_contact.2#%d_%d", owner, req.UserID))
		add(fmt.Sprintf("user_contact.2#%d_%d", req.UserID, owner))
	}
	for _, id := range chats {
		add(fmt.Sprintf("chat#%d", id))
		add(fmt.Sprintf("chat_participant.3#%d_%d", id, req.UserID))
	}
	return keys
}
