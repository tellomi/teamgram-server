package catalog

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

var (
	currentMu sync.RWMutex
	current   *Service
)

// Current is the process-wide catalog. Nil before Open.
func Current() *Service {
	currentMu.RLock()
	defer currentMu.RUnlock()
	return current
}

// Must returns the process catalog, opening the built-in seed if Open has not run.
func Must() *Service {
	if s := Current(); s != nil {
		return s
	}
	return Open("")
}

func setCurrent(s *Service) {
	currentMu.Lock()
	current = s
	currentMu.Unlock()
}

// Open builds the built-in catalog, upserts it when MySQL is configured, and
// uploads document bytes to S3 in the background when those variables are set.
func Open(dsn string) *Service {
	s := open(dsn)
	go func() {
		if err := s.uploadSeed(); err != nil {
			logx.Errorf("stickers upload: %v", err)
		}
	}()
	return s
}

// Import upserts the catalog and uploads document bytes before returning.
func Import(dsn string) error {
	return open(dsn).uploadSeed()
}

func open(dsn string) *Service {
	if env := os.Getenv("TEAMGRAM_MYSQL_DSN"); env != "" {
		dsn = env
	}
	s := newService(builtinPacks())
	if dsn != "" {
		if err := s.attachMySQL(dsn); err != nil {
			logx.Errorf("stickers mysql: %v", err)
		}
	}
	setCurrent(s)
	return s
}

func (s *Service) uploadSeed() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return uploadAssets(ctx, s.packs)
}
