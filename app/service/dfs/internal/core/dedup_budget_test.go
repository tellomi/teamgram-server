package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/teamgram-server/pkg/uploadledger"
)

func TestIndependentDedupChecksBudgetBeforeStore(t *testing.T) {
	led := uploadledger.New(uploadledger.Limits{
		Entity:  10,
		Network: 100,
		Hold:    100,
		Window:  time.Hour,
	})
	uploadledger.Use(led)
	t.Cleanup(func() { uploadledger.Use(nil) })
	if err := led.CommitNew(7, 10); err != nil {
		t.Fatal(err)
	}
	c := &DfsCore{}
	ctx := uploadledger.WithUser(context.Background(), 7)
	_, err := c.storeDocumentBytes(ctx, 99, []byte("0123456789abcdef"), false)
	if err == nil {
		t.Fatal("object store ran after the entity cap was full")
	}
}
