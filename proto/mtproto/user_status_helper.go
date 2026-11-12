// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package mtproto

import (
	"time"
)

// UserOnlinePeriod is how long account.updateStatus keeps a user online
// after the last foreground heartbeat. The stored user_presences.expires
// column uses the same window. Readers must not substitute a shorter one.
const UserOnlinePeriod int32 = 300

var (
	userStatusEmpty = MakeTLUserStatusEmpty(nil).To_UserStatus()
)

func MakeUserStatusOnline() *UserStatus {
	now := time.Now().Unix()
	status := MakeTLUserStatusOnline(&UserStatus{
		Expires: int32(now) + UserOnlinePeriod,
	}).To_UserStatus()
	return status
}

func MakeUserStatusOffline(lastSeenAt int64) *UserStatus {
	return MakeTLUserStatusOffline(&UserStatus{
		WasOnline: int32(lastSeenAt),
	}).To_UserStatus()
}

// MakeUserStatus
/**********************************************************************************
    // Android client source code
	//
    public static String formatUserStatus(int currentAccount, TLRPC.User user) {
        if (user != null && user.status != null && user.status.expires == 0) {
            if (user.status instanceof TLRPC.TL_userStatusRecently) {
                user.status.expires = -100;
            } else if (user.status instanceof TLRPC.TL_userStatusLastWeek) {
                user.status.expires = -101;
            } else if (user.status instanceof TLRPC.TL_userStatusLastMonth) {
                user.status.expires = -102;
            }
        }
        if (user != null && user.status != null && user.status.expires <= 0) {
            if (MessagesController.getInstance(currentAccount).onlinePrivacy.containsKey(user.id)) {
                return getString("Online", R.string.Online);
            }
        }
        if (user == null || user.status == null || user.status.expires == 0 || UserObject.isDeleted(user) || user instanceof TLRPC.TL_userEmpty) {
            return getString("ALongTimeAgo", R.string.ALongTimeAgo);
        } else {
            int currentTime = ConnectionsManager.getInstance(currentAccount).getCurrentTime();
            if (user.status.expires > currentTime) {
                return getString("Online", R.string.Online);
            } else {
                if (user.status.expires == -1) {
                    return getString("Invisible", R.string.Invisible);
                } else if (user.status.expires == -100) {
                    return getString("Lately", R.string.Lately);
                } else if (user.status.expires == -101) {
                    return getString("WithinAWeek", R.string.WithinAWeek);
                } else if (user.status.expires == -102) {
                    return getString("WithinAMonth", R.string.WithinAMonth);
                }  else {
                    return formatDateOnline(user.status.expires);
                }
            }
        }
    }

	// 60*60*24*7   week
	// 60*60*24*30  month
*/
// PresenceCursor maps a stored heartbeat and its expires window onto the
// timestamp MakeUserStatus treats as one UserOnlinePeriod before the
// displayed boundary. expires > 0 means the user is online until
// lastSeenAt+expires. expires == 0 means they went offline at lastSeenAt.
// A missing row (lastSeenAt <= 0) stays missing. Existing rows are not
// rewritten; this is only the value copied onto ImmutableUser.
func PresenceCursor(lastSeenAt int64, expires int32) int64 {
	if lastSeenAt <= 0 {
		return 0
	}
	if expires == UserOnlinePeriod {
		return lastSeenAt
	}
	if expires <= 0 {
		return lastSeenAt - int64(UserOnlinePeriod)
	}
	return lastSeenAt + int64(expires) - int64(UserOnlinePeriod)
}

// PresenceStatus is the status account.updateStatus stored.
// expires > 0 keeps the user online until lastSeenAt+expires.
// expires == 0 is an explicit offline at lastSeenAt.
// lastSeenAt <= 0 is a user with no presence row.
func PresenceStatus(lastSeenAt int64, expires int32, allowTimestamp bool, now int64) *UserStatus {
	if lastSeenAt <= 0 {
		if !allowTimestamp {
			return nil
		}
		return userStatusEmpty
	}
	until := lastSeenAt
	online := false
	if expires > 0 {
		until = lastSeenAt + int64(expires)
		online = now < until
	}
	if !allowTimestamp {
		seen := until
		if online {
			seen = now
		}
		return approximateStatus(now, seen, false)
	}
	if online {
		return MakeTLUserStatusOnline(&UserStatus{
			Expires: int32(until),
		}).To_UserStatus()
	}
	return MakeTLUserStatusOffline(&UserStatus{
		WasOnline: int32(until),
	}).To_UserStatus()
}

// MakeUserStatus reads an ImmutableUser last_seen_at. That value is a
// PresenceCursor: a normal heartbeat is stored as-is and stays online for
// UserOnlinePeriod, and an explicit offline is stored one period earlier so
// the displayed was_online is the real offline time.
func MakeUserStatus(lastSeenAt int64, allowTimestamp bool) *UserStatus {
	return PresenceStatus(lastSeenAt, UserOnlinePeriod, allowTimestamp, time.Now().Unix())
}

func approximateStatus(now, seen int64, byMe bool) *UserStatus {
	var st *UserStatus
	if seen <= 0 || now-seen >= 60*60*24*30 {
		return nil
	} else if now-seen >= 60*60*24*7 {
		st = MakeTLUserStatusLastMonth(&UserStatus{ByMe: byMe}).To_UserStatus()
	} else if now-seen >= 60*60*24*3 {
		st = MakeTLUserStatusLastWeek(&UserStatus{ByMe: byMe}).To_UserStatus()
	} else {
		st = MakeTLUserStatusRecently(&UserStatus{ByMe: byMe}).To_UserStatus()
	}
	return st
}

// PresenceOnline reports whether a stored heartbeat is still inside its window.
func PresenceOnline(now, lastSeenAt int64, expires int32) bool {
	return lastSeenAt > 0 && expires > 0 && now < lastSeenAt+int64(expires)
}

// StatusForViewer is the status a viewer sees for targetLastSeen, which is a
// PresenceCursor. Exact online or offline requires the target to allow the
// viewer and the viewer to allow the target. Otherwise the same buckets
// users.getUsers returns are used. by_me is set when the bucket exists
// because the viewer hid their own last seen.
func StatusForViewer(targetLastSeen int64, targetAllowsViewer, viewerAllowsTarget bool, now int64) *UserStatus {
	if targetAllowsViewer && viewerAllowsTarget {
		return PresenceStatus(targetLastSeen, UserOnlinePeriod, true, now)
	}
	st := PresenceStatus(targetLastSeen, UserOnlinePeriod, false, now)
	if st != nil && !viewerAllowsTarget {
		st.ByMe = true
	}
	return st
}

// PushStatusForViewer is StatusForViewer for a status update that was just
// written. exact is the online or offline object stored for viewers who may
// see the timestamp. Everyone else gets the bucket for that same instant.
func PushStatusForViewer(exact *UserStatus, lastSeenAt int64, expires int32, targetAllowsViewer, viewerAllowsTarget bool, now int64) *UserStatus {
	if exact == nil {
		return nil
	}
	if targetAllowsViewer && viewerAllowsTarget {
		return exact
	}
	st := PresenceStatus(lastSeenAt, expires, false, now)
	if st != nil && !viewerAllowsTarget {
		st.ByMe = true
	}
	return st
}
