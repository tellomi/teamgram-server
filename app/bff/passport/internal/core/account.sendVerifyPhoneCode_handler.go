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

package core

import (
	"os"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// AccountSendVerifyPhoneCode
// account.sendVerifyPhoneCode#a5a356f9 phone_number:string settings:CodeSettings = auth.SentCode;
// https://core.telegram.org/method/account.sendVerifyPhoneCode
//
// Issues an SMS-shaped auth.sentCode for Telegram Passport. Test-range
// numbers use the dev code. Other numbers are stored for account.verifyPhone;
// SMS transport is not connected. Set TELLOMI_PASSPORT_PHONE_CODE_LOG=1 to
// print the code while bringing a sender up.
func (c *PassportCore) AccountSendVerifyPhoneCode(in *mtproto.TLAccountSendVerifyPhoneCode) (*mtproto.Auth_SentCode, error) {
	user, err := c.requireUser()
	if err != nil {
		c.Logger.Errorf("account.sendVerifyPhoneCode - error: %v", err)
		return nil, err
	}
	hash, code, err := c.book().SendPhoneCode(c.ctx, user, in.GetPhoneNumber())
	if err != nil {
		c.Logger.Errorf("account.sendVerifyPhoneCode - error: %v", err)
		return nil, err
	}
	if os.Getenv("TELLOMI_PASSPORT_PHONE_CODE_LOG") == "1" {
		c.Logger.Infof("passport verify code user=%d phone=%s code=%s", user, in.GetPhoneNumber(), code)
	} else {
		c.Logger.Infof("passport verify code issued user=%d phone=%s", user, in.GetPhoneNumber())
	}
	_ = code
	return mtproto.MakeTLAuthSentCode(&mtproto.Auth_SentCode{
		Type: mtproto.MakeTLAuthSentCodeTypeSms(&mtproto.Auth_SentCodeType{
			Length: 5,
		}).To_Auth_SentCodeType(),
		PhoneCodeHash: hash,
		Timeout:       wrapperspb.Int32(180),
	}).To_Auth_SentCode(), nil
}
