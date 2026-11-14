package core

import (
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// usernameClaimPlan is how account.updateUsername and channels.updateUsername
// move a peer onto next. The old editable row has to be renamed, not left
// behind, or contacts.resolveUsername and the next claim still see it.
type usernameClaimPlan struct {
	Occupied        bool
	RewriteSpelling bool
	RenameEditable  bool
	InsertIfMissing bool
}

// reservedBotTaken reports a built-in bot name that this peer does not already hold.
// A missing row and a placeholder row are taken. The holder is not.
func reservedBotTaken(username string, peerType int32, peerID int64, row *dataobject.UsernameDO) bool {
	if user.GetBotIdByName(username) == 0 {
		return false
	}
	return row == nil || peerID == 0 || row.PeerType != peerType || row.PeerId != peerID
}

func planUsernameClaim(found, ours bool, stored, next string) usernameClaimPlan {
	if found && !ours {
		return usernameClaimPlan{Occupied: true}
	}
	if found && ours {
		return usernameClaimPlan{RewriteSpelling: stored != next}
	}
	return usernameClaimPlan{RenameEditable: true, InsertIfMissing: true}
}

// claimUsername points peerType/peerID at username and releases that peer's
// previous editable name. A name held by someone else returns boolFalse.
func (c *UserCore) claimUsername(peerType int32, peerID int64, username string) (*mtproto.Bool, error) {
	existing, err := c.svcCtx.Dao.UsernameDAO.SelectByUsername(c.ctx, username)
	if err != nil {
		c.Logger.Errorf("username.claimUsername - error: %v", err)
		return nil, err
	}
	stored := ""
	ours := false
	if existing != nil {
		stored = existing.Username
		ours = existing.PeerType == peerType && existing.PeerId == peerID && peerID != 0
	}
	// A reserved bot name cannot be inserted for a new peer. The peer that
	// already holds the row can keep it, including a case-only rewrite.
	// A missing row and a placeholder (peer_type 0) are not "ours".
	if user.GetBotIdByName(username) != 0 && !ours {
		return mtproto.BoolFalse, nil
	}
	plan := planUsernameClaim(existing != nil, ours, stored, username)
	if plan.Occupied {
		return mtproto.BoolFalse, nil
	}
	if plan.RewriteSpelling {
		if _, err = c.svcCtx.Dao.UsernameDAO.Update(c.ctx, map[string]interface{}{
			"username": username,
		}, stored); err != nil {
			if sqlx.IsDuplicate(err) {
				return mtproto.BoolFalse, nil
			}
			c.Logger.Errorf("username.claimUsername - error: %v", err)
			return nil, err
		}
	}
	if existing != nil && ours {
		return mtproto.BoolTrue, nil
	}
	rows, err := c.svcCtx.Dao.UsernameDAO.UpdateUsername(c.ctx, username, peerType, peerID)
	if err != nil {
		if sqlx.IsDuplicate(err) {
			return mtproto.BoolFalse, nil
		}
		c.Logger.Errorf("username.claimUsername - error: %v", err)
		return nil, err
	}
	if rows > 0 || !plan.InsertIfMissing {
		return mtproto.BoolTrue, nil
	}
	_, _, err = c.svcCtx.Dao.UsernameDAO.Insert(c.ctx, &dataobject.UsernameDO{
		Username: username,
		PeerType: peerType,
		PeerId:   peerID,
		Editable: true,
		Active:   true,
		Order2:   time.Now().Unix() << 32,
	})
	if err != nil {
		if sqlx.IsDuplicate(err) {
			return mtproto.BoolFalse, nil
		}
		c.Logger.Errorf("username.claimUsername - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
