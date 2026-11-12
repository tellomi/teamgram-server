package core

import "github.com/teamgram/proto/mtproto"

// statusViewer says whether the subject allows this viewer to see an exact
// timestamp, and whether the viewer allows the subject to see theirs.
// known is false when that viewer's rules were not loaded; they are skipped.
// A nil statusViewer means the whole privacy load failed. Recipients then
// stay empty: a failed load must not fan the exact status out.
type statusViewer func(id int64) (targetAllows, viewerAllows, known bool)

// shouldPushStatus is true only when online/offline changed. A heartbeat
// that refreshes an already-open window does not push.
func shouldPushStatus(wasOnline, nowOnline bool) bool {
	return wasOnline != nowOnline
}

// statusRecipients is the deduped audience, self excluded. blocked skips
// either direction. visible nil fails closed.
func statusRecipients(self int64, forward, reverse []int64, visible statusViewer, blocked map[int64]struct{}) []int64 {
	if visible == nil {
		return nil
	}
	seen := make(map[int64]struct{}, len(forward)+len(reverse))
	out := make([]int64, 0, len(forward)+len(reverse))
	consider := func(id int64) {
		if id == 0 || id == self {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		if blocked != nil {
			if _, ban := blocked[id]; ban {
				return
			}
		}
		out = append(out, id)
	}
	for _, id := range forward {
		consider(id)
	}
	for _, id := range reverse {
		consider(id)
	}
	return out
}

// statusDelivery is one updateUserStatus. The status is the exact online or
// offline object when both sides share timestamps, and the same bucket
// users.getUsers would return otherwise.
type statusDelivery struct {
	UserID int64
	Status *mtproto.UserStatus
}

func statusDeliveries(self int64, exact *mtproto.UserStatus, lastSeen int64, expires int32, now int64, forward, reverse []int64, visible statusViewer, blocked map[int64]struct{}) []statusDelivery {
	if visible == nil || exact == nil {
		return nil
	}
	seen := make(map[int64]struct{}, len(forward)+len(reverse))
	out := make([]statusDelivery, 0, len(forward)+len(reverse))
	consider := func(id int64) {
		if id == 0 || id == self {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		if blocked != nil {
			if _, ban := blocked[id]; ban {
				return
			}
		}
		targetAllows, viewerAllows, known := visible(id)
		if !known {
			return
		}
		st := mtproto.PushStatusForViewer(exact, lastSeen, expires, targetAllows, viewerAllows, now)
		if st == nil {
			return
		}
		out = append(out, statusDelivery{UserID: id, Status: st})
	}
	for _, id := range forward {
		consider(id)
	}
	for _, id := range reverse {
		consider(id)
	}
	return out
}

func userStatusPush(userID int64, status *mtproto.UserStatus, date int32) *mtproto.Updates {
	if status == nil {
		return nil
	}
	return mtproto.MakeTLUpdateShort(&mtproto.Updates{
		Update: mtproto.MakeTLUpdateUserStatus(&mtproto.Update{
			UserId:            userID,
			Status_USERSTATUS: status,
		}).To_Update(),
		Date: date,
	}).To_Updates()
}

// attachForwardContacts lets CheckPrivacy see "my contacts" on a user loaded
// without that edge list. ContactUserId is the person this user added.
func attachForwardContacts(me *mtproto.ImmutableUser, ids []int64) {
	if me == nil {
		return
	}
	rows := make([]*mtproto.ContactData, 0, len(ids))
	for _, id := range ids {
		if id == 0 || id == me.Id() {
			continue
		}
		rows = append(rows, mtproto.MakeTLContactData(&mtproto.ContactData{
			UserId:        me.Id(),
			ContactUserId: id,
		}).To_ContactData())
	}
	me.Contacts = rows
}
