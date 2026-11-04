package service

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/discoverrel"
	"github.com/teamgram/teamgram-server/pkg/discoverauth"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func withUser(userID int64, ip string) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID, ClientAddr: ip}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

type memAccounts struct {
	rows map[int64]Account
	err  error
}

func (m memAccounts) Account(_ context.Context, userID int64) (Account, error) {
	if m.err != nil {
		return Account{}, m.err
	}
	row, ok := m.rows[userID]
	if !ok {
		return Account{}, mtproto.ErrUserIdInvalid
	}
	return row, nil
}

func testKeys(t *testing.T) {
	t.Helper()
	t.Setenv(discoverauth.EnvEnabled, "1")
	t.Setenv(discoverauth.EnvSigningKey, base64.StdEncoding.EncodeToString(bytesSeq(1, 32)))
	t.Setenv(discoverauth.EnvPseudonymKey, base64.StdEncoding.EncodeToString(bytesSeq(2, 32)))
}

func bytesSeq(start, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(start + i)
	}
	return b
}

func TestDiscoverCredentialVerifiesAndStaysOffByDefault(t *testing.T) {
	t.Setenv(discoverauth.EnvEnabled, "")
	svc := New(memAccounts{rows: map[int64]Account{
		7: {CountryCode: "CN", Phone: "8613800138000"},
		8: {CountryCode: "US", Phone: "14155552671"},
		9: {Deleted: true},
	}})
	svc.mainland = func(string) bool { return false }
	fixed := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	svc.now = func() int64 { return fixed.Unix() }

	if _, err := svc.DiscoverGetEngineCredential(withUser(7, "1.2.3.4"), nil); status.Convert(err).Message() != "DISCOVER_DISABLED" {
		t.Fatalf("flag off: %v", err)
	}

	testKeys(t)
	got, err := svc.DiscoverGetEngineCredential(withUser(7, "8.8.8.8"), nil)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := discoverauth.LoadKeys()
	if err != nil {
		t.Fatal(err)
	}
	claims, err := discoverauth.Verify(keys.Signing, got.GetToken(), fixed)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Rgn != discoverauth.RegionCN || claims.Sub != discoverauth.EncodeSub(discoverauth.Pseudonym(keys.Pseudonym, 7)) {
		t.Fatalf("claims %+v", claims)
	}
	if got.GetRegion() != "cn" || int64(got.GetExpires()) != claims.Exp || uint32(got.GetConstructor()) != 0x0578ccaa {
		t.Fatalf("tl %+v", got)
	}

	over, err := svc.DiscoverGetEngineCredential(withUser(8, "8.8.8.8"), nil)
	if err != nil || over.GetRegion() != "global" {
		t.Fatalf("overseas %+v %v", over, err)
	}
	svc.mainland = func(string) bool { return true }
	forced, err := svc.DiscoverGetEngineCredential(withUser(8, "1.2.3.4"), nil)
	if err != nil || forced.GetRegion() != "cn" {
		t.Fatalf("mainland ip %+v %v", forced, err)
	}
	if _, err = svc.DiscoverGetEngineCredential(withUser(9, ""), nil); status.Convert(err).Message() != "USER_ID_INVALID" {
		t.Fatalf("deleted: %v", err)
	}
	if _, err = svc.DiscoverGetEngineCredential(withUser(0, ""), nil); status.Convert(err).Message() != "AUTH_KEY_UNREGISTERED" {
		t.Fatalf("anon: %v", err)
	}
}

func TestCredentialRecordsSubjectWhenRelationsOn(t *testing.T) {
	testKeys(t)
	svc := New(memAccounts{rows: map[int64]Account{
		7: {CountryCode: "US", Phone: "14155552671"},
	}})
	svc.mainland = func(string) bool { return false }
	fixed := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	svc.now = func() int64 { return fixed.Unix() }

	discoverrel.UseDirectory(nil)
	t.Cleanup(func() { discoverrel.UseDirectory(nil) })
	if _, err := svc.DiscoverGetEngineCredential(withUser(7, "8.8.8.8"), nil); err != nil {
		t.Fatal(err)
	}

	dir := discoverrel.NewMemory()
	discoverrel.UseDirectory(dir)
	got, err := svc.DiscoverGetEngineCredential(withUser(7, "8.8.8.8"), nil)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := discoverauth.LoadKeys()
	if err != nil {
		t.Fatal(err)
	}
	sub := discoverauth.Pseudonym(keys.Pseudonym, 7)
	users, err := dir.Users(context.Background(), [][]byte{sub})
	if err != nil || users[discoverauth.EncodeSub(sub)] != 7 {
		t.Fatalf("subject %v %v", users, err)
	}
	claims, err := discoverauth.Verify(keys.Signing, got.GetToken(), fixed)
	if err != nil || claims.Sub != discoverauth.EncodeSub(sub) {
		t.Fatalf("token sub %s err %v", claims.Sub, err)
	}

	svc = svc.WithMembers(&memMembers{views: map[int64]MemberView{8: {Live: true}}})
	if _, err = svc.DiscoverGetAudienceToken(withUser(7, ""), &mtproto.TLDiscoverGetAudienceToken{Kind: "include", UserIds: []int64{8}}); err != nil {
		t.Fatal(err)
	}
	users, err = dir.Users(context.Background(), [][]byte{sub})
	if err != nil || users[discoverauth.EncodeSub(sub)] != 7 {
		t.Fatalf("audience subject %v %v", users, err)
	}
}
