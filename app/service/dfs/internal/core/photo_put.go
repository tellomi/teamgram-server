package core

import (
	"context"
	"sync"
)

// photoPutConcurrency is how many photo objects a single upload writes to
// object storage at once. Each R2 put is a round trip; writing the size
// ladder one object at a time made sendMedia wait 2.9–3.6s. The cap stays
// small so one photo cannot open dozens of connections.
const photoPutConcurrency = 4

// runBounded runs fns with at most limit in flight and waits for every one.
// The first error cancels the rest. A successful return means every fn
// returned nil, so every object the caller will advertise is durable.
func runBounded(ctx context.Context, limit int, fns []func(context.Context) error) error {
	if len(fns) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if limit < 1 {
		limit = 1
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	for _, fn := range fns {
		fn := fn
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			if err := fn(ctx); err != nil {
				select {
				case errCh <- err:
					cancel()
				default:
				}
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}
