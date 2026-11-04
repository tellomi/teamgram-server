package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/discoverauth"

	"google.golang.org/grpc/status"
)

type memMembers struct {
	views map[int64]MemberView
	err   error
	calls int
}

func (m *memMembers) Lookup(_ context.Context, _ int64, ids []int64) (map[int64]MemberView, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	out := make(map[int64]MemberView, len(ids))
	for _, id := range ids {
		out[id] = m.views[id]
	}
	return out, nil
}

func TestAudienceTokenVerifiesAndStaysOffByDefault(t *testing.T) {
	t.Setenv(discoverauth.EnvEnabled, "")
	dir := &memMembers{views: map[int64]MemberView{
		7: {Live: true},
		8: {Live: true, Blocked: true},
		9: {Live: false},
	}}
	svc := New(memAccounts{rows: map[int64]Account{
		4: {CountryCode: "CN"},
		5: {Deleted: true},
	}}).WithMembers(dir)
	fixed := time.Date(2026, 11, 4, 3, 0, 0, 0, time.UTC)
	svc.now = func() int64 { return fixed.Unix() }

	req := &mtproto.TLDiscoverGetAudienceToken{Kind: "include", UserIds: []int64{7}}
	if _, err := svc.DiscoverGetAudienceToken(withUser(4, ""), req); status.Convert(err).Message() != "DISCOVER_DISABLED" {
		t.Fatalf("flag off: %v", err)
	}
	if dir.calls != 0 {
		t.Fatal("flag off looked up members")
	}

	testKeys(t)
	if _, err := svc.DiscoverGetAudienceToken(withUser(0, ""), req); status.Convert(err).Message() != "AUTH_KEY_UNREGISTERED" {
		t.Fatalf("anon: %v", err)
	}
	bad := &mtproto.TLDiscoverGetAudienceToken{Kind: "friends", UserIds: []int64{7}}
	if _, err := svc.DiscoverGetAudienceToken(withUser(4, ""), bad); status.Convert(err).Message() != "AUDIENCE_INVALID" {
		t.Fatalf("kind: %v", err)
	}
	self := &mtproto.TLDiscoverGetAudienceToken{Kind: "exclude", UserIds: []int64{4}}
	if _, err := svc.DiscoverGetAudienceToken(withUser(4, ""), self); status.Convert(err).Message() != "AUDIENCE_INVALID" {
		t.Fatalf("self: %v", err)
	}
	tooMany := make([]int64, discoverauth.AudienceMaxMembers+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	if _, err := svc.DiscoverGetAudienceToken(withUser(4, ""), &mtproto.TLDiscoverGetAudienceToken{Kind: "include", UserIds: tooMany}); status.Convert(err).Message() != "AUDIENCE_INVALID" || dir.calls != 0 {
		t.Fatalf("cap calls %d err %v", dir.calls, err)
	}
	if _, err := svc.DiscoverGetAudienceToken(withUser(5, ""), req); status.Convert(err).Message() != "USER_ID_INVALID" {
		t.Fatalf("deleted caller: %v", err)
	}

	got, err := svc.DiscoverGetAudienceToken(withUser(4, ""), &mtproto.TLDiscoverGetAudienceToken{Kind: "exclude", UserIds: []int64{7}})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := discoverauth.LoadKeys()
	if err != nil {
		t.Fatal(err)
	}
	claims, err := discoverauth.VerifyAudience(keys.Signing, got.GetToken(), fixed)
	if err != nil {
		t.Fatal(err)
	}
	wantExp := fixed.Add(discoverauth.AudienceTTL).Unix()
	if claims.Kind != "exclude" || claims.Owner != discoverauth.EncodeSub(discoverauth.Pseudonym(keys.Pseudonym, 4)) {
		t.Fatalf("%+v", claims)
	}
	if len(claims.Members) != 1 || claims.Members[0] != discoverauth.EncodeSub(discoverauth.Pseudonym(keys.Pseudonym, 7)) || claims.Exp != wantExp {
		t.Fatalf("%+v", claims)
	}
	if int64(got.GetExpires()) != wantExp || uint32(got.GetConstructor()) != 0xda177181 {
		t.Fatalf("tl %+v", got)
	}

	if _, err = svc.DiscoverGetAudienceToken(withUser(4, ""), &mtproto.TLDiscoverGetAudienceToken{Kind: "include", UserIds: []int64{8}}); status.Convert(err).Message() != "USER_IS_BLOCKED" {
		t.Fatalf("blocked: %v", err)
	}
	if _, err = svc.DiscoverGetAudienceToken(withUser(4, ""), &mtproto.TLDiscoverGetAudienceToken{Kind: "include", UserIds: []int64{9}}); status.Convert(err).Message() != "USER_ID_INVALID" {
		t.Fatalf("missing: %v", err)
	}
	dir.err = errors.New("db")
	if _, err = svc.DiscoverGetAudienceToken(withUser(4, ""), req); status.Convert(err).Message() != "INTERNAL" {
		t.Fatalf("lookup: %v", err)
	}

	bare := New(memAccounts{rows: map[int64]Account{4: {}}}).WithMembers(nil)
	bare.now = svc.now
	if _, err = bare.DiscoverGetAudienceToken(withUser(4, ""), req); status.Convert(err).Message() != "INTERNAL" {
		t.Fatalf("no directory: %v", err)
	}
}

func TestAudienceRPCIsRegistered(t *testing.T) {
	tup := mtproto.FindRPCContextTuple(&mtproto.TLDiscoverGetAudienceToken{})
	if tup == nil || tup.Method != "/mtproto.RPCDiscover/discover_getAudienceToken" {
		t.Fatalf("%v", tup)
	}
	if _, ok := tup.NewReplyFunc().(*mtproto.Discover_AudienceToken); !ok {
		t.Fatalf("%T", tup.NewReplyFunc())
	}
	var found bool
	for _, m := range mtproto.RPCDiscover_ServiceDesc.Methods {
		if m.MethodName == "discover_getAudienceToken" {
			found = true
		}
	}
	if !found {
		t.Fatal("grpc service is missing discover_getAudienceToken")
	}
}
