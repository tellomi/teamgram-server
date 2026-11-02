// Package retention removes old chat media from object storage.
//
// Policy (owner-approved, see docs/media-retention.md):
//   - Text is kept forever. Only chat media objects are removed.
//   - Chat media (photos, videos, voice, files) is kept Days (90) after its
//     original upload. Forwards reuse the same media id, so they do not reset
//     the clock.
//   - Media is kept forever while ANY user who still has it in a chat (a
//     message box owner, a basic-group member or a channel member) is an
//     active subscriber, or is inside GraceDays (30) after premium_expire_date.
//   - Avatars, stickers, custom emoji, GIFs and wallpapers are exempt, and so
//     is media that no message references any more.
//   - Small thumbnails (photo sizes s/i/j, document thumbs) are kept, so the
//     chat still shows a blurred preview.
package retention

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EnvEnabled   = "TELLOMI_MEDIA_RETENTION_ENABLED"
	EnvDays      = "TELLOMI_MEDIA_RETENTION_DAYS"
	EnvGraceDays = "TELLOMI_MEDIA_RETENTION_GRACE_DAYS"
	EnvDryRun    = "TELLOMI_MEDIA_RETENTION_DRY_RUN"
	EnvBatch     = "TELLOMI_MEDIA_RETENTION_BATCH"
	EnvMaxPerRun = "TELLOMI_MEDIA_RETENTION_MAX_PER_RUN"
	EnvRunHour   = "TELLOMI_MEDIA_RETENTION_RUN_HOUR"
)

// Config controls the sweeper. Zero values are replaced by defaults in
// Normalize.
type Config struct {
	Enabled   bool
	Days      int
	GraceDays int
	DryRun    bool
	// Batch is how many candidate rows are read per query.
	Batch int
	// MaxPerRun caps how many media items one run may expire (0 = no cap).
	MaxPerRun int
	// RunHour is the local hour (0-23) the daily run starts.
	RunHour int
}

// Defaults returns the production defaults: 90 days, 30-day grace.
func Defaults() Config {
	return Config{Days: 90, GraceDays: 30, Batch: 500, MaxPerRun: 5000, RunHour: 4}
}

func (c *Config) Normalize() {
	d := Defaults()
	if c.Days <= 0 {
		c.Days = d.Days
	}
	if c.GraceDays < 0 {
		c.GraceDays = 0
	}
	if c.Batch <= 0 {
		c.Batch = d.Batch
	}
	if c.MaxPerRun < 0 {
		c.MaxPerRun = 0
	}
	if c.RunHour < 0 || c.RunHour > 23 {
		c.RunHour = d.RunHour
	}
}

func (c Config) Retention() time.Duration { return time.Duration(c.Days) * 24 * time.Hour }
func (c Config) Grace() time.Duration     { return time.Duration(c.GraceDays) * 24 * time.Hour }

// FromEnv reads TELLOMI_MEDIA_RETENTION_*. Retention is off unless
// TELLOMI_MEDIA_RETENTION_ENABLED is true.
func FromEnv() (Config, error) {
	c := Defaults()
	var err error
	if c.Enabled, err = envBool(EnvEnabled, false); err != nil {
		return c, err
	}
	if c.DryRun, err = envBool(EnvDryRun, false); err != nil {
		return c, err
	}
	for _, f := range []struct {
		name string
		dst  *int
	}{{EnvDays, &c.Days}, {EnvGraceDays, &c.GraceDays}, {EnvBatch, &c.Batch}, {EnvMaxPerRun, &c.MaxPerRun}, {EnvRunHour, &c.RunHour}} {
		v := strings.TrimSpace(os.Getenv(f.name))
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return c, fmt.Errorf("%s=%q: %w", f.name, v, err)
		}
		*f.dst = n
	}
	if c.Days < 1 {
		return c, fmt.Errorf("%s must be at least 1", EnvDays)
	}
	c.Normalize()
	return c, nil
}

func envBool(name string, def bool) (bool, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def, fmt.Errorf("%s=%q: %w", name, v, err)
	}
	return b, nil
}
