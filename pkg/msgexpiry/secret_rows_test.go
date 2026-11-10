package msgexpiry

import "testing"

func TestSecretRecordStaysBlockedAfterRestart(t *testing.T) {
	t.Setenv(EnvEnabled, "1")
	t.Setenv(EnvSweep, "1")
	sink := newMemSecret()
	SetSecretSink(sink)
	t.Cleanup(func() { SetSecretSink(nil) })

	now := int32(2_000)
	first := NewLedger()
	first.Register(Record{Kind: KindSecret, Scope: 4, ID: 3, Date: 100, TTL: 10, FileID: 13, Users: []int64{7, 8}})
	first.Register(Record{Kind: KindPrivate, Scope: 7, ID: 1, Date: 100, TTL: 10, FileID: 11, Users: []int64{7, 8}})
	first.Register(Record{Kind: KindViewOnce, Scope: 7, ID: 4, Date: now, TTL: 30, FileID: 14, Users: []int64{7, 8}})
	if got := sink.All(); len(got) != 1 || got[0].Kind != KindSecret || got[0].FileID != 13 {
		t.Fatalf("sink stored %+v", got)
	}

	restarted := NewLedger()
	for _, row := range sink.All() {
		restarted.Register(row)
	}
	if restarted.Readable(KindSecret, 4, 3, now) || !restarted.BlocksDownload(13, now) {
		t.Fatal("secret ciphertext revived after restart")
	}
	if !restarted.Readable(KindPrivate, 7, 1, now) {
		t.Fatal("private ledger row was restored from the secret sink")
	}
	swept := restarted.Sweep(now)
	if len(swept) != 1 || swept[0].Kind != KindSecret {
		t.Fatalf("sweep %+v", swept)
	}
	again := NewLedger()
	for _, row := range sink.All() {
		again.Register(row)
	}
	if n := again.Sweep(now); len(n) != 0 {
		t.Fatalf("second process swept %+v", n)
	}
	if again.Readable(KindSecret, 4, 3, now) || !again.BlocksDownload(13, now) {
		t.Fatal("swept secret ciphertext revived")
	}
}
