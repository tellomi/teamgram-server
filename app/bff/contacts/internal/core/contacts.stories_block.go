package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/contacts/book"
)

// Story blocklist edits for contacts.block, contacts.unblock, and
// contacts.getBlocked when my_stories_from is set.
// https://core.telegram.org/api/block

func (c *ContactsCore) blockFromStories(in *mtproto.TLContactsBlock) (*mtproto.Bool, error) {
	if err := c.requireSelf(); err != nil {
		return nil, err
	}
	peer, err := inputPeerUser(c.MD.UserId, in.GetId())
	if err != nil {
		c.Logger.Errorf("contacts.block - error: %v", err)
		return nil, err
	}
	if err := c.ensureUsers([]int64{peer.PeerId}); err != nil {
		c.Logger.Errorf("contacts.block - error: %v", err)
		return nil, err
	}
	added, err := c.relBook().StoryAdd(c.ctx, c.MD.UserId, book.Peer{Type: mtproto.PEER_USER, ID: peer.PeerId}, nowUnix())
	if err != nil {
		c.Logger.Errorf("contacts.block - error: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	if added {
		c.pushStory(book.Peer{Type: mtproto.PEER_USER, ID: peer.PeerId}, true)
	}
	return mtproto.BoolTrue, nil
}

func (c *ContactsCore) unblockFromStories(in *mtproto.TLContactsUnblock) (*mtproto.Bool, error) {
	if err := c.requireSelf(); err != nil {
		return nil, err
	}
	peer, err := inputPeerUser(c.MD.UserId, in.GetId())
	if err != nil {
		c.Logger.Errorf("contacts.unblock - error: %v", err)
		return nil, err
	}
	removed, err := c.relBook().StoryRemove(c.ctx, c.MD.UserId, book.Peer{Type: mtproto.PEER_USER, ID: peer.PeerId})
	if err != nil {
		c.Logger.Errorf("contacts.unblock - error: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	if removed {
		c.pushStory(book.Peer{Type: mtproto.PEER_USER, ID: peer.PeerId}, false)
	}
	return mtproto.BoolTrue, nil
}

func (c *ContactsCore) storyBlockedPage(in *mtproto.TLContactsGetBlocked) (*mtproto.Contacts_Blocked, error) {
	if err := c.requireSelf(); err != nil {
		return nil, err
	}
	rows := c.relBook().StoryList(c.ctx, c.MD.UserId)
	offset := int(in.GetOffset())
	if offset < 0 {
		offset = 0
	}
	limit := int(in.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	total := len(rows)
	if offset > total {
		offset = total
	}
	page := rows[offset:]
	if len(page) > limit {
		page = page[:limit]
	}
	blocked := make([]*mtproto.PeerBlocked, 0, len(page))
	ids := make([]int64, 0, len(page))
	for _, row := range page {
		blocked = append(blocked, mtproto.MakeTLPeerBlocked(&mtproto.PeerBlocked{
			PeerId: mtPeer(row.Peer),
			Date:   row.Date,
		}).To_PeerBlocked())
		if row.Peer.Type == mtproto.PEER_USER {
			ids = append(ids, row.Peer.ID)
		}
	}
	out := &mtproto.Contacts_Blocked{
		Blocked: blocked,
		Chats:   []*mtproto.Chat{},
		Users:   c.topUsers(ids),
		Count:   int32(total),
	}
	if offset+len(page) < total {
		return mtproto.MakeTLContactsBlockedSlice(out).To_Contacts_Blocked(), nil
	}
	return mtproto.MakeTLContactsBlocked(out).To_Contacts_Blocked(), nil
}
