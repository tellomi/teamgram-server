/*
 * WARNING! All changes made in this file will be lost!
 *   Created from by 'dalgen'
 *
 * Copyright (c) 2024-present,  Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package dataobject

type AuthUsersDO struct {
	Id          int64 `db:"id" json:"id"`
	AuthKeyId   int64 `db:"auth_key_id" json:"auth_key_id"`
	UserId      int64 `db:"user_id" json:"user_id"`
	Hash        int64 `db:"hash" json:"hash"`
	DateCreated int64 `db:"date_created" json:"date_created"`
	DateActive  int64 `db:"date_active" json:"date_active"`
	// ActivityTrusted is 1 after a real RPC or ping has refreshed date_active.
	// Rows written before that tracker existed stay 0. Their date_active is the
	// bind time, not inactivity, and must not expire the session.
	ActivityTrusted           bool  `db:"activity_trusted" json:"activity_trusted"`
	State                     int32 `db:"state" json:"state"`
	AndroidPushSessionId      int64 `db:"android_push_session_id" json:"android_push_session_id"`
	EncryptedRequestsDisabled bool  `db:"encrypted_requests_disabled" json:"encrypted_requests_disabled"`
	CallRequestsDisabled      bool  `db:"call_requests_disabled" json:"call_requests_disabled"`
	// Unconfirmed is authorization.unconfirmed. confirmed on
	// account.changeAuthorizationSettings clears it.
	Unconfirmed bool `db:"unconfirmed" json:"unconfirmed"`
	Deleted     bool `db:"deleted" json:"deleted"`
}
