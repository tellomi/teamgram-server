package dao

import "testing"

func TestPhoneCodeLength(t *testing.T) {
	t.Setenv("TELLOMI_SMS_PROVIDER", "aliyun")
	t.Setenv("TELLOMI_SMS_CODE_LENGTH", "")
	if phoneCodeLength() != 6 {
		t.Fatalf("aliyun default %d", phoneCodeLength())
	}
	t.Setenv("TELLOMI_SMS_CODE_LENGTH", "5")
	if phoneCodeLength() != 5 {
		t.Fatalf("aliyun explicit %d", phoneCodeLength())
	}
	t.Setenv("TELLOMI_SMS_PROVIDER", "fixed")
	if phoneCodeLength() != 5 {
		t.Fatalf("fixed mode %d", phoneCodeLength())
	}
}
