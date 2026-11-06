package password

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/srp"
)

func TestGetPasswordDropsStaleChallenges(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	svc := New(Options{Now: func() time.Time { return now }})
	prev := Swap(svc)
	t.Cleanup(func() { Swap(prev) })
	SeedPhone(42)

	ctx := context.Background()
	pw, err := AccountPassword(ctx, 42, 7)
	if err != nil {
		t.Fatal(err)
	}
	salt1 := append(append([]byte{}, pw.GetNewAlgo().GetSalt1()...), bytes.Repeat([]byte{0x11}, 32)...)
	empty := mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP()
	if _, err = Update(ctx, 42, 7, empty, &mtproto.Account_PasswordInputSettings{
		NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
			Salt1: salt1,
			Salt2: append([]byte{}, pw.GetNewAlgo().GetSalt2()...),
			G:     pw.GetNewAlgo().GetG(),
			P:     append([]byte{}, pw.GetNewAlgo().GetP()...),
		}).To_PasswordKdfAlgo(),
		NewPasswordHash: srp.Verifier([]byte("secret"), salt1, pw.GetNewAlgo().GetSalt2()),
	}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		if _, err = AccountPassword(ctx, 42, 7); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = AccountPassword(ctx, 42, 8); err != nil {
		t.Fatal(err)
	}
	if n := challengeCount(svc); n != 6 {
		t.Fatalf("live challenges = %d, want 6", n)
	}

	now = now.Add(challengeTTL + time.Second)
	if _, err = AccountPassword(ctx, 42, 9); err != nil {
		t.Fatal(err)
	}
	if n := challengeCount(svc); n != 1 {
		t.Fatalf("expired rows left %d", n)
	}
}

func challengeCount(svc *Service) int {
	mem := svc.store.(*memoryStore)
	mem.mu.Lock()
	defer mem.mu.Unlock()
	return len(mem.challenges)
}
