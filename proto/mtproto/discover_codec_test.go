package mtproto

import (
	"hash/crc32"
	"testing"
)

func TestDiscoverConstructorIDs(t *testing.T) {
	checks := []struct {
		schema string
		id     uint32
	}{
		{"discover.engineCredential token:string expires:int region:string = discover.EngineCredential", crcDiscoverEngineCredential},
		{"discover.getEngineCredential = discover.EngineCredential", crcDiscoverGetEngineCredential},
		{"discover.audienceToken token:string expires:int = discover.AudienceToken", crcDiscoverAudienceToken},
		{"discover.getAudienceToken kind:string user_ids:Vector long = discover.AudienceToken", crcDiscoverGetAudienceToken},
	}
	for _, c := range checks {
		if got := crc32.ChecksumIEEE([]byte(c.schema)); got != c.id {
			t.Fatalf("%s: got 0x%08x want 0x%08x", c.schema, got, c.id)
		}
	}
}

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

func TestDiscoverAudienceRoundTrip(t *testing.T) {
	req := &TLDiscoverGetAudienceToken{Kind: "exclude", UserIds: []int64{7, 9001}}
	raw := encodeAt(t, req, Layer)
	if ctorOf(raw) != crcDiscoverGetAudienceToken {
		t.Fatalf("request ctor 0x%x", ctorOf(raw))
	}
	gotReq, ok := NewDecodeBuf(raw).Object().(*TLDiscoverGetAudienceToken)
	if !ok || uint32(gotReq.GetConstructor()) != crcDiscoverGetAudienceToken {
		t.Fatalf("request %#v", gotReq)
	}
	if gotReq.GetKind() != "exclude" || len(gotReq.GetUserIds()) != 2 || gotReq.GetUserIds()[0] != 7 || gotReq.GetUserIds()[1] != 9001 {
		t.Fatalf("request fields %#v", gotReq)
	}

	tok := MakeDiscoverAudienceToken("header.payload.sig", 1_760_000_000)
	raw = encodeAt(t, tok, Layer)
	if ctorOf(raw) != crcDiscoverAudienceToken {
		t.Fatalf("result ctor 0x%x", ctorOf(raw))
	}
	got, ok := NewDecodeBuf(raw).Object().(*Discover_AudienceToken)
	if !ok || got.GetToken() != "header.payload.sig" || got.GetExpires() != 1_760_000_000 {
		t.Fatalf("result %#v", got)
	}
	if uint32(got.GetConstructor()) != crcDiscoverAudienceToken {
		t.Fatalf("result ctor field 0x%x", uint32(got.GetConstructor()))
	}
}
