package model

import "testing"

func TestToAuthSentCodeReportsRealLength(t *testing.T) {
	for _, code := range []string{"012345", "123456", "0123"} {
		tx := &PhoneCodeTransaction{
			PhoneCode:     code,
			PhoneCodeHash: "hashhashhashhash",
			SentCodeType:  SentCodeTypeSms,
			NextCodeType:  CodeTypeSms,
		}
		sent := tx.ToAuthSentCode()
		if sent.GetType().GetPredicateName() != "auth_sentCodeTypeSms" {
			t.Fatalf("type %s", sent.GetType().GetPredicateName())
		}
		if int(sent.GetType().GetLength()) != len(code) {
			t.Fatalf("reported %d for code %q", sent.GetType().GetLength(), code)
		}
	}
}

func TestResendKeepsSmsLength(t *testing.T) {
	tx := &PhoneCodeTransaction{
		PhoneCode:     "012345",
		PhoneCodeHash: "old-hash",
		SentCodeType:  SentCodeTypeApp,
		NextCodeType:  CodeTypeNone,
		State:         CodeStateSent,
	}
	// resendCode sets the SMS type before ToAuthSentCode, same as sendCode.
	tx.SentCodeType = SentCodeTypeSms
	tx.NextCodeType = CodeTypeSms
	tx.State = CodeStateSent
	sent := tx.ToAuthSentCode()
	if sent.GetType().GetPredicateName() != "auth_sentCodeTypeSms" || sent.GetType().GetLength() != 6 {
		t.Fatalf("resend sent code type=%s length=%d", sent.GetType().GetPredicateName(), sent.GetType().GetLength())
	}
}
