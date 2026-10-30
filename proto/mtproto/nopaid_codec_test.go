package mtproto

import "testing"

func TestNoPaidMessagesTokenRoundTrip(t *testing.T) {
	if got := uint32(GetClazzID(Predicate_exportedNoPaidMessagesToken, Layer)); got != 0x695dd042 {
		t.Fatalf("result ctor 0x%x", got)
	}
	if got := uint32(GetClazzID(Predicate_contacts_exportNoPaidMessagesToken, Layer)); got != 0x83c586ae {
		t.Fatalf("export ctor 0x%x", got)
	}
	if got := uint32(GetClazzID(Predicate_contacts_importNoPaidMessagesToken, Layer)); got != 0x35a7c6ca {
		t.Fatalf("import ctor 0x%x", got)
	}

	exp := &TLContactsExportNoPaidMessagesToken{Revoke: true, Temporary: true}
	raw := encodeAt(t, exp, Layer)
	obj := NewDecodeBuf(raw).Object()
	if NewDecodeBuf(raw).GetError() != nil {
		t.Fatal(NewDecodeBuf(raw).GetError())
	}
	gotExp, ok := obj.(*TLContactsExportNoPaidMessagesToken)
	if !ok || !gotExp.GetRevoke() || !gotExp.GetTemporary() {
		t.Fatalf("export decode %#v", obj)
	}

	perm := MakeExportedNoPaidMessagesToken("tok", "https://t.me/ada?free=tok", 0)
	raw = encodeAt(t, perm, Layer)
	if ctorOf(raw) != 0x695dd042 {
		t.Fatalf("result ctor 0x%x", ctorOf(raw))
	}
	// flags, token, url, and no expires field.
	decoded := NewDecodeBuf(raw).Object()
	gotPerm, ok := decoded.(*ExportedNoPaidMessagesToken)
	if !ok || gotPerm.GetToken() != "tok" || gotPerm.GetExpires() != 0 || gotPerm.GetUrl() != "https://t.me/ada?free=tok" {
		t.Fatalf("permanent %#v", decoded)
	}

	temp := MakeExportedNoPaidMessagesToken("short", "https://t.me/contact/abc?free=short", 1_700_000_060)
	raw = encodeAt(t, temp, Layer)
	gotTemp, ok := NewDecodeBuf(raw).Object().(*ExportedNoPaidMessagesToken)
	if !ok || gotTemp.GetExpires() != 1_700_000_060 || gotTemp.GetToken() != "short" {
		t.Fatalf("temporary %#v", gotTemp)
	}

	imp := &TLContactsImportNoPaidMessagesToken{
		UserId: MakeTLInputUser(&InputUser{UserId: 42, AccessHash: 7}).To_InputUser(),
		Token:  "short",
	}
	raw = encodeAt(t, imp, Layer)
	if ctorOf(raw) != 0x35a7c6ca {
		t.Fatalf("import ctor 0x%x", ctorOf(raw))
	}
	gotImp, ok := NewDecodeBuf(raw).Object().(*TLContactsImportNoPaidMessagesToken)
	if !ok || gotImp.GetToken() != "short" || gotImp.GetUserId().GetUserId() != 42 || gotImp.GetUserId().GetAccessHash() != 7 {
		t.Fatalf("import %#v user %#v", gotImp, gotImp.GetUserId())
	}
}
