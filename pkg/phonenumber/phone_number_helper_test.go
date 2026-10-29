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

import "testing"

func TestCheckPhoneNumberRealNumbers(t *testing.T) {
	cases := []struct {
		in     string
		region string
		digits string
	}{
		{"8613800138000", "CN", "8613800138000"},
		{"+8613800138000", "CN", "8613800138000"},
		{"+86 138 0013 8000", "CN", "8613800138000"},
		{"+86-138-0013-8000", "CN", "8613800138000"},
		{"008613800138000", "CN", "8613800138000"},
		{"+86 13800138000", "CN", "8613800138000"},
		{"79161234567", "RU", "79161234567"},
		{"+7 916 123 45 67", "RU", "79161234567"},
		{"+14155552671", "US", "14155552671"},
		{"+447911123456", "GG", "447911123456"},
		{"+6281234567890", "ID", "6281234567890"},
	}
	for _, tc := range cases {
		region, digits, err := CheckPhoneNumberInvalid(tc.in)
		if err != nil {
			t.Fatalf("CheckPhoneNumberInvalid(%q) error: %v", tc.in, err)
		}
		if region != tc.region || digits != tc.digits {
			t.Fatalf("CheckPhoneNumberInvalid(%q) = %q %q, want %q %q", tc.in, region, digits, tc.region, tc.digits)
		}
	}
}

func TestCheckPhoneNumberRejectsInvalid(t *testing.T) {
	for _, phone := range []string{"", "123", "8611111111111", "abc", "+999123"} {
		if _, _, err := CheckPhoneNumberInvalid(phone); err == nil {
			t.Fatalf("CheckPhoneNumberInvalid(%q) accepted an invalid number", phone)
		}
	}
}

func TestCheckPhoneNumberLegacyAndFragment(t *testing.T) {
	cases := map[string]string{
		"+42400":       "42400",
		"+424000":      "424000",
		"42777":        "42777",
		"+88808880080": "88808880080",
		"88808880080":  "88808880080",
	}
	for in, want := range cases {
		_, digits, err := CheckPhoneNumberInvalid(in)
		if err != nil || digits != want {
			t.Fatalf("CheckPhoneNumberInvalid(%q) = %q %v, want %q", in, digits, err, want)
		}
	}
	if _, _, err := CheckPhoneNumberInvalid("+8880888008"); err == nil {
		t.Fatal("short fragment number was accepted")
	}
}

func TestDefaultTestPhoneRange(t *testing.T) {
	SetTestPhoneRanges(nil)
	t.Cleanup(func() { SetTestPhoneRanges(nil) })

	region, digits, err := CheckPhoneNumberInvalid("+99966 1 2345")
	if err != nil {
		t.Fatal(err)
	}
	if region != "" || digits != "9996612345" {
		t.Fatalf("got region %q digits %q", region, digits)
	}
	code, ok := TestPhoneCode("9996612345")
	if !ok || code != "11111" {
		t.Fatalf("dc1 code = %q ok=%v", code, ok)
	}
	if code, _ := TestPhoneCode("9996620001"); code != "22222" {
		t.Fatalf("dc2 code = %q", code)
	}
	if !AcceptsTestCode("+9996639999", "33333") {
		t.Fatal("dc3 code was rejected")
	}
	if AcceptsTestCode("9996612345", "12345") {
		t.Fatal("unrelated code was accepted as the test code")
	}
	if _, _, err := CheckPhoneNumberInvalid("999661234"); err == nil {
		t.Fatal("short 99966 number was accepted")
	}
	if _, _, err := CheckPhoneNumberInvalid("9996712345"); err == nil {
		t.Fatal("99967 number was accepted by the 99966 range")
	}
}

func TestConfiguredTestPhoneRange(t *testing.T) {
	SetTestPhoneRanges([]TestPhoneRange{{
		Prefix: "1555",
		Length: 8,
		Code:   "42424",
	}})
	t.Cleanup(func() { SetTestPhoneRanges(nil) })

	_, digits, err := CheckPhoneNumberInvalid("15551234")
	if err != nil || digits != "15551234" {
		t.Fatalf("custom range: %q %v", digits, err)
	}
	if code, ok := TestPhoneCode("15551234"); !ok || code != "42424" {
		t.Fatalf("custom code = %q ok=%v", code, ok)
	}
	if _, _, err := CheckPhoneNumberInvalid("9996612345"); err == nil {
		t.Fatal("default range stayed active after a custom config")
	}

	SetTestPhoneRanges([]TestPhoneRange{})
	if _, _, err := CheckPhoneNumberInvalid("15551234"); err == nil {
		t.Fatal("empty config still accepted a test number")
	}
}

func TestLookupKeysShareCanonicalDigits(t *testing.T) {
	left := LookupKeys("+86 138-0013-8000")
	right := LookupKeys("8613800138000")
	if !sharesKey(left, right) {
		t.Fatalf("keys do not overlap:\n%v\n%v", left, right)
	}
	intl := LookupKeys("008613800138000")
	if !sharesKey(intl, right) {
		t.Fatalf("00 prefix keys do not overlap:\n%v\n%v", intl, right)
	}
}

func sharesKey(a, b []string) bool {
	set := map[string]struct{}{}
	for _, k := range a {
		set[k] = struct{}{}
	}
	for _, k := range b {
		if _, ok := set[k]; ok {
			return true
		}
	}
	return false
}
