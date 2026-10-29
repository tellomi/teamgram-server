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

package user

import (
	"strings"
)

const (
	MinUsernameLen = 3
	MaxUsernameLen = 32
)

const (
	UsernameNotExisted   = 0
	UsernameExisted      = 1
	UsernameExistedNotMe = 2
	UsernameExistedIsMe  = 3
)

var ()

/*
   if (name != null) {
       if (name.startsWith("_") || name.endsWith("_")) {
           checkTextView.setText(LocaleController.getString("LinkInvalid", R.string.LinkInvalid));
           checkTextView.setTag(Theme.key_windowBackgroundWhiteRedText4);
           checkTextView.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteRedText4));
           return false;
       }
       for (int a = 0; a < name.length(); a++) {
           char ch = name.charAt(a);
           if (a == 0 && ch >= '0' && ch <= '9') {
               checkTextView.setText(LocaleController.getString("LinkInvalidStartNumber", R.string.LinkInvalidStartNumber));
               checkTextView.setTag(Theme.key_windowBackgroundWhiteRedText4);
               checkTextView.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteRedText4));
               return false;
           }
           if (!(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_')) {
               checkTextView.setText(LocaleController.getString("LinkInvalid", R.string.LinkInvalid));
               checkTextView.setTag(Theme.key_windowBackgroundWhiteRedText4);
               checkTextView.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteRedText4));
               return false;
           }
       }
   }
   if (name == null || name.length() < 3) {
       checkTextView.setText(LocaleController.getString("LinkInvalidShort", R.string.LinkInvalidShort));
       checkTextView.setTag(Theme.key_windowBackgroundWhiteRedText4);
       checkTextView.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteRedText4));
       return false;
   }
   if (name.length() > 32) {
       checkTextView.setText(LocaleController.getString("LinkInvalidLong", R.string.LinkInvalidLong));
       checkTextView.setTag(Theme.key_windowBackgroundWhiteRedText4);
       checkTextView.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteRedText4));
       return false;
   }
*/

// CheckUsernameInvalid reports whether username matches the public username
// rule. The name is historical: it returns true when the username is valid.
// Valid usernames are 3–32 characters of [A-Za-z0-9_], start with a letter,
// and do not end with an underscore.
func CheckUsernameInvalid(username string) bool {
	if len(username) < MinUsernameLen || len(username) > MaxUsernameLen {
		return false
	}
	if strings.HasPrefix(username, "_") || strings.HasSuffix(username, "_") {
		return false
	}
	if !isASCIILetter(username[0]) {
		return false
	}
	for _, ch := range username {
		if !isUsernameRune(ch) {
			return false
		}
	}
	return true
}

func isASCIILetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isUsernameRune(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}
