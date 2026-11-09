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

package phonenumber

import (
	"strings"
	"sync"

	"github.com/nyaruka/phonenumbers"
)

// TestPhoneRange is a dev login range, shaped like Telegram's test DC numbers
// (99966XYYYY, where X is the datacenter digit).
//
// A number matches when its digits start with Prefix and, when Length is
// greater than zero, the digit string is exactly Length characters.
// Code, when set, is the login code. Otherwise CodeFromDigit repeats the
// first digit after Prefix five times (9996612345 → 11111).
type TestPhoneRange struct {
	Prefix        string `json:",optional"`
	Length        int    `json:",optional"`
	Code          string `json:",optional"`
	CodeFromDigit bool   `json:",optional"`
}

// DefaultTestPhoneRanges is Telegram's test-DC pattern, used when config
// omits TestPhones. Set TestPhones to an empty list to disable it.
func DefaultTestPhoneRanges() []TestPhoneRange {
	return []TestPhoneRange{{
		Prefix:        "99966",
		Length:        10,
		CodeFromDigit: true,
	}}
}

var (
	testPhoneMu     sync.RWMutex
	testPhoneRanges = DefaultTestPhoneRanges()
)

// SetTestPhoneRanges replaces the process-wide test ranges.
// A nil slice restores DefaultTestPhoneRanges. An empty non-nil slice disables them.
func SetTestPhoneRanges(ranges []TestPhoneRange) {
	testPhoneMu.Lock()
	defer testPhoneMu.Unlock()
	if ranges == nil {
		testPhoneRanges = DefaultTestPhoneRanges()
		return
	}
	testPhoneRanges = append([]TestPhoneRange(nil), ranges...)
}

func currentTestPhoneRanges() []TestPhoneRange {
	testPhoneMu.RLock()
	defer testPhoneMu.RUnlock()
	return append([]TestPhoneRange(nil), testPhoneRanges...)
}

// NormalizeDigits keeps decimal digits and drops every other rune.
func NormalizeDigits(phone string) string {
	var b strings.Builder
	b.Grow(len(phone))
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// LookupKeys returns the phone spellings that should be treated as the same
// number: the raw compact form, the digit form, a leading '+' variant, the
// international 00 prefix stripped, and the +86 form of a bare mainland
// mobile. Callers match on any shared key.
func LookupKeys(phone string) []string {
	var keys []string
	add := func(s string) {
		if s == "" || s == "+" {
			return
		}
		for _, k := range keys {
			if k == s {
				return
			}
		}
		keys = append(keys, s)
	}

	trimmed := strings.TrimSpace(phone)
	compact := stripPhoneSeparators(trimmed)
	digits := NormalizeDigits(compact)
	add(trimmed)
	add(compact)
	add(strings.TrimPrefix(compact, "+"))
	add(digits)
	if digits != "" {
		add("+" + digits)
	}
	if strings.HasPrefix(digits, "00") && len(digits) > 4 {
		rest := digits[2:]
		add(rest)
		add("+" + rest)
	}
	// Address books on iOS often store a mainland mobile with no country
	// code (13800138000). Signup stores the E.164 digits (8613800138000).
	// Tellomi's default region is CN, so a bare national number gains that
	// prefix. A number that already carries + or 00 is left as written.
	if !hasExplicitCallingCode(trimmed) {
		for _, extra := range chinaMobileAliases(digits) {
			add(extra)
		}
	}
	return keys
}

func hasExplicitCallingCode(phone string) bool {
	compact := stripPhoneSeparators(strings.TrimSpace(phone))
	if strings.HasPrefix(compact, "+") {
		return true
	}
	digits := NormalizeDigits(compact)
	return strings.HasPrefix(digits, "00") && len(digits) > 4
}

// chinaMobileNational is an 11-digit mainland mobile, 1[3-9] followed by 9 digits.
func chinaMobileNational(digits string) bool {
	if len(digits) != 11 || digits[0] != '1' {
		return false
	}
	return digits[1] >= '3' && digits[1] <= '9'
}

// chinaMobileAliases returns the E.164 spellings of a national mainland
// mobile, including a single trunk 0 (0138...). Empty when the digits are
// not a valid CN number, so a foreign number is not relabeled as +86.
func chinaMobileAliases(digits string) []string {
	national := digits
	switch {
	case chinaMobileNational(national):
	case len(national) == 12 && national[0] == '0' && chinaMobileNational(national[1:]):
		national = national[1:]
	default:
		return nil
	}
	parsed, err := phonenumbers.Parse(national, "CN")
	if err != nil || !phonenumbers.IsValidNumberForRegion(parsed, "CN") {
		return nil
	}
	e164 := phonenumbers.NormalizeDigitsOnly(phonenumbers.Format(parsed, phonenumbers.E164))
	if e164 == "" || e164 == national {
		return nil
	}
	return []string{e164, "+" + e164}
}

func stripPhoneSeparators(phone string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '(', ')', '.', '\t':
			return -1
		default:
			return r
		}
	}, phone)
}

func rangeMatches(r TestPhoneRange, digits string) bool {
	if r.Prefix == "" || !strings.HasPrefix(digits, r.Prefix) {
		return false
	}
	if r.Length > 0 && len(digits) != r.Length {
		return false
	}
	return true
}

func bestTestPhoneRange(digits string) (TestPhoneRange, bool) {
	var (
		found TestPhoneRange
		ok    bool
	)
	for _, r := range currentTestPhoneRanges() {
		if !rangeMatches(r, digits) {
			continue
		}
		if !ok || len(r.Prefix) > len(found.Prefix) {
			found = r
			ok = true
		}
	}
	return found, ok
}

// MatchTestPhone reports whether phone is in the configured test range and
// returns its canonical digits.
func MatchTestPhone(phone string) (string, bool) {
	digits := NormalizeDigits(phone)
	if _, ok := bestTestPhoneRange(digits); !ok {
		return "", false
	}
	return digits, true
}

func (r TestPhoneRange) codeFor(digits string) (string, bool) {
	if r.Code != "" {
		return r.Code, true
	}
	if !r.CodeFromDigit {
		return "", false
	}
	if len(digits) > len(r.Prefix) {
		d := digits[len(r.Prefix)]
		if d >= '0' && d <= '9' {
			return strings.Repeat(string(d), 5), true
		}
	}
	return "12345", true
}

// TestPhoneCode returns the dev login code for a test-range number.
func TestPhoneCode(phone string) (string, bool) {
	digits := NormalizeDigits(phone)
	r, ok := bestTestPhoneRange(digits)
	if !ok {
		return "", false
	}
	return r.codeFor(digits)
}

// AcceptsTestCode reports whether code is the dev login code for phone.
func AcceptsTestCode(phone, code string) bool {
	expect, ok := TestPhoneCode(phone)
	return ok && code != "" && code == expect
}
