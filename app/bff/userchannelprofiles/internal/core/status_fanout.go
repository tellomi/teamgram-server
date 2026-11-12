package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/statusbook"
)

// statusViewer says whether the subject allows this viewer to see an exact
// timestamp, and whether the viewer allows the subject to see theirs.
// known is false when that viewer's rules were not loaded; they are skipped.
// A nil statusViewer means the whole privacy load failed. Recipients then
// stay empty: a failed load must not fan the exact status out.
type statusViewer func(id int64) (targetAllows, viewerAllows, known bool)

// planStatusFanout decides a heartbeat. Crossing offline and online always
// reloads the audience. While the user stays online, a later expires is
// pushed from the cached recipient list and does not read contacts, privacy,
// or blocks. The same or an older expires does not push. Without a list the
// audience is loaded, including when the book read failed.
func planStatusFanout(wasOnline, nowOnline, haveList bool, lastPushed, newDeadline int64) (push, reload bool) {
	if wasOnline != nowOnline {
		return true, true
	}
	if !nowOnline {
		return false, false
	}
	if lastPushed > 0 && newDeadline <= lastPushed {
		return false, false
	}
	if !haveList {
		return true, true
	}
	return true, false
}

// heartbeatResult is one account.updateStatus fanout attempt.
type heartbeatResult struct {
	Next   statusbook.Record
	Save   bool
	Forget bool
	Loaded bool
}

// runStatusHeartbeat pushes when planStatusFanout says so. A failed load or
// any failed push leaves the book unchanged so the next heartbeat retries.
// loaded is true only when contacts, privacy, and blocks were read.
func runStatusHeartbeat(wasOnline, nowOnline bool, lastSeen int64, expires int32, now, newDeadline int64, exact *mtproto.UserStatus, aud statusbook.Record, load func() ([]statusDelivery, []statusbook.Viewer, error), push func(statusDelivery) error) (heartbeatResult, error) {
	doPush, reload := planStatusFanout(wasOnline, nowOnline, aud.HaveList, aud.Pushed, newDeadline)
	if !doPush {
		return heartbeatResult{}, nil
	}
	viewers := aud.Viewers
	var deliveries []statusDelivery
	loaded := false
	if reload || !aud.HaveList {
		var err error
		deliveries, viewers, err = load()
		if err != nil {
			return heartbeatResult{Loaded: true}, err
		}
		loaded = true
	} else {
		deliveries = deliveriesFromViewers(exact, lastSeen, expires, now, viewers)
	}
	for _, row := range deliveries {
		if err := push(row); err != nil {
			return heartbeatResult{Loaded: loaded}, err
		}
	}
	if !nowOnline {
		return heartbeatResult{Forget: true, Loaded: loaded}, nil
	}
	return heartbeatResult{
		Next: statusbook.Record{
			Pushed:   newDeadline,
			Viewers:  viewers,
			HaveList: true,
			Gen:      aud.Gen,
		},
		Save:   true,
		Loaded: loaded,
	}, nil
}

func deliveriesFromViewers(exact *mtproto.UserStatus, lastSeen int64, expires int32, now int64, viewers []statusbook.Viewer) []statusDelivery {
	if exact == nil {
		return nil
	}
	out := make([]statusDelivery, 0, len(viewers))
	for _, v := range viewers {
		if v.ID == 0 {
			continue
		}
		targetAllows, viewerAllows := v.Exact, true
		if !v.Exact {
			targetAllows = false
			viewerAllows = !v.ByMe
		}
		st := mtproto.PushStatusForViewer(exact, lastSeen, expires, targetAllows, viewerAllows, now)
		if st == nil {
			continue
		}
		out = append(out, statusDelivery{UserID: v.ID, Status: st})
	}
	return out
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
	deliveries, _ := statusAudience(self, exact, lastSeen, expires, now, forward, reverse, visible, blocked)
	return deliveries
}

func statusAudience(self int64, exact *mtproto.UserStatus, lastSeen int64, expires int32, now int64, forward, reverse []int64, visible statusViewer, blocked map[int64]struct{}) ([]statusDelivery, []statusbook.Viewer) {
	if visible == nil || exact == nil {
		return nil, nil
	}
	seen := make(map[int64]struct{}, len(forward)+len(reverse))
	out := make([]statusDelivery, 0, len(forward)+len(reverse))
	viewers := make([]statusbook.Viewer, 0, len(forward)+len(reverse))
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
		viewers = append(viewers, statusbook.Viewer{
			ID:    id,
			Exact: targetAllows && viewerAllows,
			ByMe:  !viewerAllows,
		})
	}
	for _, id := range forward {
		consider(id)
	}
	for _, id := range reverse {
		consider(id)
	}
	return out, viewers
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
