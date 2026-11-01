package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/contacts/book"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	dialogpb "github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// testEnsureUsers, testIsContact, and testDialogHits let handler tests skip
// the user and dialog services.
var (
	testEnsureUsers func(ids []int64) error
	testIsContact   func(self, peer int64) (bool, error)
	testDialogHits  func() []book.Hit
)

func (c *ContactsCore) relBook() *book.Book {
	if c != nil && c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.Book != nil {
		return c.svcCtx.Dao.Book
	}
	return book.Shared()
}

func (c *ContactsCore) savedBook() dao.SavedBook {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil
	}
	return c.svcCtx.Dao.Saved
}

func (c *ContactsCore) requireSelf() error {
	if c.MD == nil || c.MD.UserId == 0 {
		return mtproto.ErrUserIdInvalid
	}
	return nil
}

func userPeer(self int64, in *mtproto.InputUser) (*mtproto.PeerUtil, error) {
	id := mtproto.FromInputUser(self, in)
	if id == nil || !id.IsUser() || id.IsSelf() || id.PeerId == 0 || id.PeerId == self {
		return nil, mtproto.ErrContactIdInvalid
	}
	return id, nil
}

func inputPeerUser(self int64, in *mtproto.InputPeer) (*mtproto.PeerUtil, error) {
	id := mtproto.FromInputPeer2(self, in)
	if id == nil || !id.IsUser() || id.IsSelf() || id.PeerId == 0 || id.PeerId == self {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return id, nil
}

func (c *ContactsCore) ensureUsers(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	if testEnsureUsers != nil {
		return testEnsureUsers(ids)
	}
	got, err := c.svcCtx.Dao.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: ids})
	if err != nil || got == nil {
		return mtproto.ErrContactIdInvalid
	}
	for _, id := range ids {
		imm, ok := got.GetImmutableUser(id)
		if !ok || imm == nil || imm.Deleted() {
			return mtproto.ErrContactIdInvalid
		}
	}
	return nil
}

func (c *ContactsCore) isContact(self, peer int64) (bool, error) {
	if testIsContact != nil {
		return testIsContact(self, peer)
	}
	row, err := c.svcCtx.Dao.UserGetContact(c.ctx, &userpb.TLUserGetContact{UserId: self, Id: peer})
	if err != nil || row == nil || row.GetContactUserId() == 0 {
		return false, nil
	}
	return true, nil
}

func (c *ContactsCore) pushStory(peer book.Peer, stories bool) {
	blocked := false
	if c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.UserClient != nil && peer.Type == mtproto.PEER_USER {
		got, err := c.svcCtx.Dao.UserBlockedByUser(c.ctx, &userpb.TLUserBlockedByUser{
			UserId:     c.MD.UserId,
			PeerUserId: peer.ID,
		})
		if err == nil {
			blocked = mtproto.FromBool(got)
		}
	}
	c.pushBlocked(&mtproto.PeerUtil{PeerType: peer.Type, PeerId: peer.ID}, blocked, stories)
}

func (c *ContactsCore) pushBlocked(peer *mtproto.PeerUtil, blocked, stories bool) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.SyncClient == nil || c.MD == nil || peer == nil {
		return
	}
	upd := mtproto.MakeTLUpdatePeerBlocked(&mtproto.Update{
		Blocked_BOOL:         mtproto.ToBool(blocked),
		Blocked_FLAGBOOLEAN:  blocked,
		BlockedMyStoriesFrom: stories,
		PeerId:               peer.ToPeer(),
	}).To_Update()
	_, _ = c.svcCtx.Dao.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       mtproto.MakeUpdatesByUpdates(upd),
	})
}

func (c *ContactsCore) topHits() []book.Hit {
	if testDialogHits != nil {
		return testDialogHits()
	}
	return c.loadDialogHits()
}

func (c *ContactsCore) loadDialogHits() []book.Hit {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil {
		return nil
	}
	rows, err := c.svcCtx.Dao.DialogGetDialogs(c.ctx, &dialogpb.TLDialogGetDialogs{
		UserId:        c.MD.UserId,
		ExcludePinned: mtproto.BoolFalse,
		FolderId:      0,
	})
	if err != nil || rows == nil {
		return nil
	}
	var hits []book.Hit
	var userIDs []int64
	for _, row := range rows.GetDatas() {
		if row == nil || row.GetDialog() == nil || row.GetDialog().GetPeer() == nil {
			continue
		}
		raw := row.GetDialog().GetPeer()
		switch raw.GetPredicateName() {
		case mtproto.Predicate_peerUser, mtproto.Predicate_peerChat, mtproto.Predicate_peerChannel:
		default:
			continue
		}
		peer := mtproto.FromPeer(raw)
		if peer == nil || peer.PeerId == 0 {
			continue
		}
		hit := book.Hit{Peer: book.Peer{Type: peer.PeerType, ID: peer.PeerId}, Date: row.GetDate()}
		if peer.IsUser() {
			userIDs = append(userIDs, peer.PeerId)
		}
		if peer.IsChannel() {
			if chat := channel.Shared().VisibleChat(c.ctx, c.MD.UserId, peer.PeerId); chat != nil && chat.GetMegagroup() {
				hit.Megagroup = true
			}
		}
		hits = append(hits, hit)
		if len(hits) >= 500 {
			break
		}
	}
	if len(userIDs) > 0 && c.svcCtx.Dao.UserClient != nil {
		got, err := c.svcCtx.Dao.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: userIDs})
		if err == nil && got != nil {
			bots := map[int64]bool{}
			for _, id := range userIDs {
				if imm, ok := got.GetImmutableUser(id); ok && imm != nil && imm.IsBot() {
					bots[id] = true
				}
			}
			for i := range hits {
				if hits[i].Peer.Type == mtproto.PEER_USER && bots[hits[i].Peer.ID] {
					hits[i].Bot = true
				}
			}
		}
	}
	return hits
}

func flagString(v *wrapperspb.StringValue) string {
	if v == nil {
		return ""
	}
	return v.GetValue()
}

func nowUnix() int32 {
	return int32(time.Now().Unix())
}
