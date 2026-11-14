package dao

import (
	"context"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// contactPremiumCache is the raw new_noncontact_peers_require_premium bit
// copied onto user.contact_require_premium. account.setGlobalPrivacySettings
// drops the row so the next render reads the database.
type contactPremiumCache struct {
	mu   sync.Mutex
	rows map[int64]contactPremiumEntry
}

type contactPremiumEntry struct {
	value bool
	at    time.Time
}

const contactPremiumTTL = 30 * time.Second

var contactPremium = &contactPremiumCache{rows: map[int64]contactPremiumEntry{}}

func (d *Dao) contactRequirePremium(userID int64) bool {
	if userID == 0 {
		return false
	}
	if value, ok := contactPremium.load(userID); ok {
		return value
	}
	if d == nil || d.UserClient == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	settings, err := d.UserClient.UserGetGlobalPrivacySettings(ctx, &userpb.TLUserGetGlobalPrivacySettings{UserId: userID})
	if err != nil || settings == nil {
		return false
	}
	value := settings.GetNewNoncontactPeersRequirePremium()
	contactPremium.store(userID, value)
	return value
}

func (c *contactPremiumCache) load(userID int64) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	row, ok := c.rows[userID]
	if !ok || time.Since(row.at) > contactPremiumTTL {
		return false, false
	}
	return row.value, true
}

func (c *contactPremiumCache) store(userID int64, value bool) {
	c.mu.Lock()
	c.rows[userID] = contactPremiumEntry{value: value, at: time.Now()}
	c.mu.Unlock()
}

func invalidateContactRequirePremium(userID int64) {
	if userID == 0 {
		return
	}
	contactPremium.mu.Lock()
	delete(contactPremium.rows, userID)
	contactPremium.mu.Unlock()
}

func init() {
	mtproto.SetInvalidateContactRequirePremiumFunc(invalidateContactRequirePremium)
}
