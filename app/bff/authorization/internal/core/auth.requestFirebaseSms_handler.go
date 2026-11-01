// Copyright 2022 Teamgram Authors
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

package core

import (
	"github.com/teamgram/proto/mtproto"
)

// AuthRequestFirebaseSms
// auth.requestFirebaseSms#8e39261e flags:# phone_number:string phone_code_hash:string safety_net_token:flags.0?string play_integrity_token:flags.2?string ios_push_secret:flags.1?string = Bool;
// https://core.telegram.org/method/auth.requestFirebaseSms
//
// Firebase SMS is not used in mainland China. A well-formed request returns
// boolFalse so official clients call auth.resendCode and take next_type
// (normal SMS). boolTrue would make them wait for a Firebase message that
// never arrives.
func (c *AuthorizationCore) AuthRequestFirebaseSms(in *mtproto.TLAuthRequestFirebaseSms) (*mtproto.Bool, error) {
	phone, hash := "", ""
	if in != nil {
		phone = in.PhoneNumber
		hash = in.PhoneCodeHash
	}
	ok, err := c.codes().RequestFirebaseSms(c.ctx, c.authKey(), phone, hash)
	if err != nil {
		c.Logger.Errorf("auth.requestFirebaseSms - error: %v", err)
		return nil, err
	}
	c.Logger.Infof("auth.requestFirebaseSms boolFalse; client should auth.resendCode")
	return ok, nil
}
