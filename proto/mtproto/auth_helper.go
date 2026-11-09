// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package mtproto

import "fmt"

const (
	AuthStateUnknown      = 0
	AuthStateNew          = 1
	AuthStatePermBound    = 2
	AuthStateWaitInit     = 3
	AuthStateUnauthorized = 4
	AuthStateNeedPassword = 5
	AuthStateNormal       = 6
	AuthStateLogout       = 7
	AuthStateDeleted      = 8
)

const (
	AuthKeyTypeUnknown   = -1
	AuthKeyTypePerm      = 0
	AuthKeyTypeTemp      = 1
	AuthKeyTypeMediaTemp = 2
)

func NewAuthKeyInfo(keyId int64, key []byte, keyType int) *AuthKeyInfo {
	keyData := &AuthKeyInfo{
		AuthKeyId:          keyId,
		AuthKey:            key,
		AuthKeyType:        int32(keyType),
		PermAuthKeyId:      0,
		TempAuthKeyId:      0,
		MediaTempAuthKeyId: 0,
	}

	switch keyType {
	case AuthKeyTypePerm:
		keyData.PermAuthKeyId = keyId
	case AuthKeyTypeTemp:
		keyData.TempAuthKeyId = keyId
	case AuthKeyTypeMediaTemp:
		keyData.MediaTempAuthKeyId = keyId
	}

	return keyData
}

// BoundPermAuthKeyId is the permanent key that owns this key's authorization.
// A permanent key is its own owner (including legacy rows whose perm id was
// left at 0). A temp or media-temp key uses the id written by
// auth.bindTempAuthKey. Zero means the temp key is not bound yet.
func (k *AuthKeyInfo) BoundPermAuthKeyId() int64 {
	if k == nil {
		return 0
	}
	if k.PermAuthKeyId != 0 {
		return k.PermAuthKeyId
	}
	if k.AuthKeyType == AuthKeyTypePerm && k.AuthKeyId != 0 {
		return k.AuthKeyId
	}
	return 0
}

// AuthKeyLogLabel is the only form of an auth key that logs may contain.
// The key bytes themselves are never included.
func AuthKeyLogLabel(k *AuthKeyInfo) string {
	if k == nil {
		return "auth_key_id=0"
	}
	return fmt.Sprintf("auth_key_id=%d auth_key_type=%d perm_auth_key_id=%d temp_auth_key_id=%d media_temp_auth_key_id=%d",
		k.AuthKeyId, k.AuthKeyType, k.PermAuthKeyId, k.TempAuthKeyId, k.MediaTempAuthKeyId)
}
