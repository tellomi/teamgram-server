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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/verify"
)

// AuthReportMissingCode
// auth.reportMissingCode#cb9deff6 phone_number:string phone_code_hash:string mnc:string = Bool;
// https://core.telegram.org/method/auth.reportMissingCode
func (c *AuthorizationCore) AuthReportMissingCode(in *mtproto.TLAuthReportMissingCode) (*mtproto.Bool, error) {
	phone, hash, mnc := "", "", ""
	if in != nil {
		phone = in.PhoneNumber
		hash = in.PhoneCodeHash
		mnc = in.Mnc
	}
	ok, err := c.codes().ReportMissingCode(phone, hash, mnc, func(phone, hash, mnc string) {
		shown := phone
		if verify.IsEmail(phone) {
			shown = verify.MaskEmail(phone)
		} else {
			shown = maskPhone(phone)
		}
		c.Logger.Infof("auth.reportMissingCode phone=%s hash=%s mnc=%s", shown, hashPrefix(hash), mnc)
	})
	if err != nil {
		c.Logger.Errorf("auth.reportMissingCode - error: %v", err)
		return nil, err
	}
	return ok, nil
}
