package mtproto

import "testing"

func TestDiscoverCredentialRoundTrip(t *testing.T) {
	req := &TLDiscoverGetEngineCredential{}
	raw := encodeAt(t, req, Layer)
	if ctorOf(raw) != crcDiscoverGetEngineCredential {
		t.Fatalf("request ctor 0x%x", ctorOf(raw))
	}
	gotReq, ok := NewDecodeBuf(raw).Object().(*TLDiscoverGetEngineCredential)
	if !ok || uint32(gotReq.GetConstructor()) != crcDiscoverGetEngineCredential {
		t.Fatalf("request %#v", gotReq)
	}

	cred := MakeDiscoverEngineCredential("header.payload.sig", "cn", 1_760_000_000)
	raw = encodeAt(t, cred, Layer)
	if ctorOf(raw) != crcDiscoverEngineCredential {
		t.Fatalf("result ctor 0x%x", ctorOf(raw))
	}
	got, ok := NewDecodeBuf(raw).Object().(*Discover_EngineCredential)
	if !ok || got.GetToken() != "header.payload.sig" || got.GetExpires() != 1_760_000_000 || got.GetRegion() != "cn" {
		t.Fatalf("result %#v", got)
	}
}
