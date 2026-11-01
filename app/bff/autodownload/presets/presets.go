// Package presets stores the three auto-download presets a client saves
// with account.saveAutoDownloadSettings and reads back with
// account.getAutoDownloadSettings.
//
// https://core.telegram.org/method/account.saveAutoDownloadSettings
// https://core.telegram.org/method/account.getAutoDownloadSettings
//
// low, high, or neither (medium) selects which preset is replaced. Presets
// the user has not saved stay at the defaults clients receive on a fresh
// account. Redis is used when KV is configured. An in-process map is used
// otherwise, which is what the unit tests and a process without Redis get.
package presets

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"

	"github.com/teamgram/marmota/pkg/stores/kv"
	"github.com/teamgram/proto/mtproto"
	gokv "github.com/zeromicro/go-zero/core/stores/kv"
)

type store interface {
	GetCtx(ctx context.Context, key string) (string, error)
	SetCtx(ctx context.Context, key, value string) error
}

// Book is the per-user auto-download store.
type Book struct {
	kv store
}

// New uses Redis when conf is set and an in-process map otherwise.
func New(conf gokv.KvConf) *Book {
	var s store
	if len(conf) == 0 {
		s = newMem()
	} else {
		s = kv.NewStore(conf)
	}
	return &Book{kv: s}
}

var (
	sharedMu sync.Mutex
	shared   = New(nil)
)

// Shared is the book the auto-download handlers use.
func Shared() *Book {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	return shared
}

// Install replaces the process-wide book. Startup calls it after opening
// Redis. Tests call it so cases do not leak into each other.
func Install(b *Book) {
	if b == nil {
		b = New(nil)
	}
	sharedMu.Lock()
	shared = b
	sharedMu.Unlock()
}

type row struct {
	Disabled                      bool  `json:"disabled"`
	VideoPreloadLarge             bool  `json:"video_preload_large"`
	AudioPreloadNext              bool  `json:"audio_preload_next"`
	PhonecallsLessData            bool  `json:"phonecalls_less_data"`
	StoriesPreload                bool  `json:"stories_preload"`
	PhotoSizeMax                  int32 `json:"photo_size_max"`
	VideoSizeMax                  int64 `json:"video_size_max"`
	FileSizeMax                   int64 `json:"file_size_max"`
	VideoUploadMaxbitrate         int32 `json:"video_upload_maxbitrate"`
	SmallQueueActiveOperationsMax int32 `json:"small_queue_active_operations_max"`
	LargeQueueActiveOperationsMax int32 `json:"large_queue_active_operations_max"`
}

type saved struct {
	Low    *row `json:"low,omitempty"`
	Medium *row `json:"medium,omitempty"`
	High   *row `json:"high,omitempty"`
}

// Get returns the three presets. Unsaved slots are the fresh-account defaults.
func (b *Book) Get(ctx context.Context, userID int64) (*mtproto.Account_AutoDownloadSettings, error) {
	cur := defaults()
	if b == nil || userID == 0 {
		return cur.toMT(), nil
	}
	raw, err := b.kv.GetCtx(ctx, key(userID))
	if err != nil || raw == "" {
		return cur.toMT(), err
	}
	var s saved
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return cur.toMT(), nil
	}
	if s.Low != nil {
		cur.Low = *s.Low
	}
	if s.Medium != nil {
		cur.Medium = *s.Medium
	}
	if s.High != nil {
		cur.High = *s.High
	}
	return cur.toMT(), nil
}

// Save replaces one preset. low selects the low preset, high selects the
// high preset, and neither selects medium. Both flags set is treated as low,
// which is the flag clients send on their own.
func (b *Book) Save(ctx context.Context, userID int64, low, high bool, settings *mtproto.AutoDownloadSettings) error {
	if b == nil || userID == 0 {
		return mtproto.ErrInputRequestInvalid
	}
	if settings == nil {
		return mtproto.ErrInputRequestInvalid
	}
	var s saved
	if raw, err := b.kv.GetCtx(ctx, key(userID)); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &s)
	}
	slot := fromMT(settings)
	switch {
	case low:
		s.Low = &slot
	case high:
		s.High = &slot
	default:
		s.Medium = &slot
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return b.kv.SetCtx(ctx, key(userID), string(raw))
}

func key(userID int64) string {
	return "autodownload_" + strconv.FormatInt(userID, 10)
}

type triple struct {
	Low, Medium, High row
}

func defaults() triple {
	return triple{
		Low: row{
			VideoPreloadLarge:  true,
			AudioPreloadNext:   true,
			PhonecallsLessData: true,
			PhotoSizeMax:       1048576,
			VideoSizeMax:       512000,
			FileSizeMax:        512000,
		},
		Medium: row{
			VideoPreloadLarge: true,
			AudioPreloadNext:  true,
			PhotoSizeMax:      1048576,
			VideoSizeMax:      10485760,
			FileSizeMax:       1048576,
		},
		High: row{
			VideoPreloadLarge: true,
			AudioPreloadNext:  true,
			PhotoSizeMax:      1048576,
			VideoSizeMax:      15728640,
			FileSizeMax:       3145728,
		},
	}
}

func (t triple) toMT() *mtproto.Account_AutoDownloadSettings {
	return mtproto.MakeTLAccountAutoDownloadSettings(&mtproto.Account_AutoDownloadSettings{
		Low:    t.Low.toMT(),
		Medium: t.Medium.toMT(),
		High:   t.High.toMT(),
	}).To_Account_AutoDownloadSettings()
}

func (r row) toMT() *mtproto.AutoDownloadSettings {
	video32 := int32(r.VideoSizeMax)
	file32 := int32(r.FileSizeMax)
	if r.VideoSizeMax > int64(^uint32(0)>>1) {
		video32 = int32(^uint32(0) >> 1)
	}
	if r.FileSizeMax > int64(^uint32(0)>>1) {
		file32 = int32(^uint32(0) >> 1)
	}
	return mtproto.MakeTLAutoDownloadSettings(&mtproto.AutoDownloadSettings{
		Disabled:                      r.Disabled,
		VideoPreloadLarge:             r.VideoPreloadLarge,
		AudioPreloadNext:              r.AudioPreloadNext,
		PhonecallsLessData:            r.PhonecallsLessData,
		StoriesPreload:                r.StoriesPreload,
		PhotoSizeMax:                  r.PhotoSizeMax,
		VideoSizeMax_INT32:            video32,
		VideoSizeMax_INT64:            r.VideoSizeMax,
		FileSizeMax_INT32:             file32,
		FileSizeMax_INT64:             r.FileSizeMax,
		VideoUploadMaxbitrate:         r.VideoUploadMaxbitrate,
		SmallQueueActiveOperationsMax: r.SmallQueueActiveOperationsMax,
		LargeQueueActiveOperationsMax: r.LargeQueueActiveOperationsMax,
	}).To_AutoDownloadSettings()
}

func fromMT(s *mtproto.AutoDownloadSettings) row {
	video := s.GetVideoSizeMax_INT64()
	if video == 0 {
		video = int64(s.GetVideoSizeMax_INT32())
	}
	file := s.GetFileSizeMax_INT64()
	if file == 0 {
		file = int64(s.GetFileSizeMax_INT32())
	}
	return row{
		Disabled:                      s.GetDisabled(),
		VideoPreloadLarge:             s.GetVideoPreloadLarge(),
		AudioPreloadNext:              s.GetAudioPreloadNext(),
		PhonecallsLessData:            s.GetPhonecallsLessData(),
		StoriesPreload:                s.GetStoriesPreload(),
		PhotoSizeMax:                  s.GetPhotoSizeMax(),
		VideoSizeMax:                  video,
		FileSizeMax:                   file,
		VideoUploadMaxbitrate:         s.GetVideoUploadMaxbitrate(),
		SmallQueueActiveOperationsMax: s.GetSmallQueueActiveOperationsMax(),
		LargeQueueActiveOperationsMax: s.GetLargeQueueActiveOperationsMax(),
	}
}

type mem struct {
	mu sync.Mutex
	m  map[string]string
}

func newMem() *mem { return &mem{m: map[string]string{}} }

func (m *mem) GetCtx(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.m[key], nil
}

func (m *mem) SetCtx(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = value
	return nil
}
