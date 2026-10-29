// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package mtproto

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
