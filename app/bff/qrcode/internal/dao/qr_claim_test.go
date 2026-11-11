package dao

import (
	"context"
	"sync"
	"testing"

	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/model"
)

func TestQRExpiredBoundary(t *testing.T) {
	if !model.QRExpired(100, 100) || !model.QRExpired(100, 101) || !model.QRExpired(0, 50) {
		t.Fatal("expired token would be accepted")
	}
	if model.QRExpired(100, 99) {
		t.Fatal("live token refused")
	}
}

func TestClaimQRLoginIsSingleUse(t *testing.T) {
	store := &nx{}
	var wg sync.WaitGroup
	okN := 0
	var mu sync.Mutex
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := claimOnce(context.Background(), store.SetnxExCtx, "qr_claim_9", "42", 30)
			if err != nil {
				errCh <- err
				return
			}
			if ok {
				mu.Lock()
				okN++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	if okN != 1 {
		t.Fatalf("claims %d", okN)
	}
	ok, err := claimOnce(context.Background(), store.SetnxExCtx, "qr_claim_9", "99", 30)
	if err != nil || ok {
		t.Fatalf("second dc claim ok=%v err=%v", ok, err)
	}
}

type nx struct {
	mu sync.Mutex
	m  map[string]string
}

func (n *nx) SetnxExCtx(_ context.Context, key, value string, _ int) (bool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.m == nil {
		n.m = map[string]string{}
	}
	if _, exists := n.m[key]; exists {
		return false, nil
	}
	n.m[key] = value
	return true, nil
}
