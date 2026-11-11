package core

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"sync"
	"testing"
	"time"
)

func TestReviewDamagedJPEGIsNotAThumb(t *testing.T) {
	src := []byte{0xff, 0xd8, 0xff, 0x00, 0x01, 0x02, 0x03}
	jpg, ok := thumbFromDocumentBytes(src)
	if ok || jpg != nil {
		t.Fatalf("damaged jpeg accepted (%d bytes)", len(jpg))
	}
	if _, _, err := image.Decode(bytes.NewReader(src)); err == nil {
		t.Fatal("fixture unexpectedly decoded")
	}
	_, err := jpeg.Decode(bytes.NewReader(src))
	if err == nil {
		t.Fatal("fixture unexpectedly decoded as jpeg")
	}
}

func TestReviewThumbSlotCancelsAndBoundsDisk(t *testing.T) {
	if !reserveThumbDisk(maxThumbDisk) {
		t.Fatal("budget did not accept a full reservation")
	}
	if reserveThumbDisk(1) {
		releaseThumbDisk(1)
		releaseThumbDisk(maxThumbDisk)
		t.Fatal("budget accepted a byte past the cap")
	}
	releaseThumbDisk(maxThumbDisk)

	thumbSlots <- struct{}{}
	thumbSlots <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := acquireThumbSlot(ctx, 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("third rebuild %v", err)
	}
	<-thumbSlots
	<-thumbSlots
	release, err := acquireThumbSlot(context.Background(), maxThumbStream)
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestReviewConcurrentFramesDoNotShareFFmpeg(t *testing.T) {
	prev := runThumbFFmpeg
	t.Cleanup(func() { runThumbFFmpeg = prev })
	var mu sync.Mutex
	seen := map[string]int{}
	runThumbFFmpeg = func(ctx context.Context, path string) ([]byte, error) {
		mu.Lock()
		seen[path]++
		mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
		img := image.NewGray(image.Rect(0, 0, 8, 8))
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, nil); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for _, name := range []string{"/tmp/a", "/tmp/b"} {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			_, err := finishVideoFrame(context.Background(), path)
			errCh <- err
		}(name)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if seen["/tmp/a"] != 1 || seen["/tmp/b"] != 1 {
		t.Fatalf("paths %v", seen)
	}
}

func TestReviewCanceledFrameIsNotFileIDInvalid(t *testing.T) {
	prev := runThumbFFmpeg
	t.Cleanup(func() { runThumbFFmpeg = prev })
	runThumbFFmpeg = func(ctx context.Context, _ string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := finishVideoFrame(ctx, "/tmp/canceled")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
	_, err = resolveThumbDownload(0, 1, nil, nil, func() ([]byte, error) {
		return nil, context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel became %v", err)
	}
}
