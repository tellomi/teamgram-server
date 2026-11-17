package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

const (
	// blockedPageMax is the largest page contacts.getBlocked returns.
	// Official clients ask for up to 100.
	blockedPageMax int32 = 100
	// blockedListMax keeps contacts.setBlocked able to read the whole list.
	blockedListMax int32 = 10000
)

// blockedWindow applies offset/limit. offset is how many rows to skip.
// limit <= 0 uses the default page of 100. limit is capped at max.
func blockedWindow(offset, limit, max int32) (int32, int32) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = blockedPageMax
	}
	if max > 0 && limit > max {
		limit = max
	}
	return offset, limit
}

func peerBlockedRows(rows []dataobject.UserPeerBlocksDO) []*mtproto.PeerBlocked {
	out := make([]*mtproto.PeerBlocked, 0, len(rows))
	for _, do := range rows {
		var peer *mtproto.Peer
		switch do.PeerType {
		case mtproto.PEER_USER:
			peer = mtproto.MakePeerUser(do.PeerId)
		case mtproto.PEER_CHAT:
			peer = mtproto.MakePeerChat(do.PeerId)
		case mtproto.PEER_CHANNEL:
			peer = mtproto.MakePeerChannel(do.PeerId)
		default:
			continue
		}
		out = append(out, mtproto.MakeTLPeerBlocked(&mtproto.PeerBlocked{
			PeerId: peer,
			Date:   int32(do.Date),
		}).To_PeerBlocked())
	}
	return out
}

// UserGetBlockedPage is contacts.getBlocked for the main block list.
// count is the full list size. The page is offset/limit.
func (c *UserCore) UserGetBlockedPage(in *user.TLUserGetBlockedList) (*mtproto.Contacts_Blocked, error) {
	offset, limit := blockedWindow(in.GetOffset(), in.GetLimit(), blockedPageMax)
	rows, total, err := c.svcCtx.Dao.BlockedPage(c.ctx, in.GetUserId(), offset, limit)
	if err != nil {
		return nil, err
	}
	blocked := peerBlockedRows(rows)
	out := &mtproto.Contacts_Blocked{
		Blocked: blocked,
		Chats:   []*mtproto.Chat{},
		Users:   []*mtproto.User{},
		Count:   int32(total),
	}
	// contacts.blockedSlice when this page does not finish the list.
	if int(offset)+len(blocked) < total {
		return mtproto.MakeTLContactsBlockedSlice(out).To_Contacts_Blocked(), nil
	}
	return mtproto.MakeTLContactsBlocked(out).To_Contacts_Blocked(), nil
}

// UserGetFallbackPhoto is the public avatar, or an empty photo when unset.
func (c *UserCore) UserGetFallbackPhoto(in *user.TLUserGetProfilePhotos) (*mtproto.Photo, error) {
	return c.svcCtx.Dao.FallbackPhoto(c.ctx, in.GetUserId())
}
