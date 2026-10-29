// Package terms builds help.termsOfService and decides when an already
// signed-in client must be shown a terms update.
//
// help.getTermsOfServiceUpdate returns help.termsOfServiceUpdateEmpty
// (expires = now + 86400) when Tos.UpdateId is empty, when this user has
// accepted that id, or when the user id is missing. The popup is returned
// only when UpdateId is set to an id this user has not accepted. The default
// config leaves UpdateId empty, so existing users are not prompted.
//
// Layer 232 has no help.getTermsOfService RPC. New accounts see the same
// document on auth.authorizationSignUpRequired.terms_of_service (the
// auth.signIn result that leads to auth.signUp). auth.signUp records
// acceptance of Config.Id. help.acceptTermsOfService records whatever id
// the client sends, one row per user and terms id.
//
// Config lives on the bff process under Tos: (see app/bff/bff/etc/bff.yaml).
// TELLOMI_TOS_ID, TELLOMI_TOS_UPDATE_ID, TELLOMI_TOS_TEXT,
// TELLOMI_TOS_TERMS_URL, and TELLOMI_TOS_PRIVACY_URL override it.
// Acceptance is stored in user_tos_acceptances. teamgramd/deploy/migrate.sh
// applies teamgramd/deploy/sql/migrate-20261029-tos.sql; the process also
// creates the table on startup. TEAMGRAM_MYSQL_DSN overrides the DSN.
package terms

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	// DefaultID is the help.termsOfService id shown at signup.
	DefaultID = "tellomi-tos-1"
	// EmptyTTL is how long a client should wait before asking again when
	// there is nothing to accept.
	EmptyTTL int32 = 86400
	// retryTTL is used when acceptance cannot be read. The client is not
	// shown a popup we may be unable to record, and it asks again soon.
	retryTTL int32 = 300

	defaultTermsLabel   = "服务条款"
	defaultPrivacyLabel = "隐私政策"
	defaultText         = "欢迎使用 Tellomi。使用本服务即表示你同意以下内容，并了解我们如何处理账号信息与消息数据。请阅读服务条款和隐私政策。"
)

// Config is the terms document and the optional update existing users must accept.
type Config struct {
	// Id is embedded in the signup help.termsOfService. Empty uses DefaultID.
	Id string `json:",optional"`
	// UpdateId, when set, is shown by help.getTermsOfServiceUpdate until this
	// user accepts it. Empty means no forced update.
	UpdateId string `json:",optional"`
	// Text replaces the Simplified Chinese summary. The terms and privacy
	// labels in the text become messageEntityTextUrl spans. A raw URL in the
	// text becomes messageEntityUrl. A missing label is appended.
	Text          string `json:",optional"`
	TermsURL      string `json:",optional"`
	PrivacyURL    string `json:",optional"`
	TermsLabel    string `json:",optional"`
	PrivacyLabel  string `json:",optional"`
	MinAgeConfirm int32  `json:",optional"`
}

// Service is the in-process terms document and acceptance store.
type Service struct {
	cfg   Config
	store Store
}

var (
	mu      sync.RWMutex
	current *Service
)

func init() {
	current = New(Config{}, nil)
}

// New builds a service. A nil store keeps acceptance in memory.
// Config is normalized and environment variables are not read, so tests stay stable.
func New(cfg Config, store Store) *Service {
	if store == nil {
		store = newMemoryStore()
	}
	return &Service{cfg: Normalize(cfg), store: store}
}

// Open stores cfg as the process-wide service. dsn empty uses memory,
// unless TEAMGRAM_MYSQL_DSN is set. A database that cannot be reached
// falls back to memory and logs the error.
func Open(dsn string, cfg Config) *Service {
	s := &Service{cfg: Normalize(applyEnv(cfg)), store: openStore(dsn)}
	mu.Lock()
	current = s
	mu.Unlock()
	return s
}

// Current is the process-wide service. Open replaces it.
func Current() *Service {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Normalize fills blanks. It does not read the environment.
func Normalize(c Config) Config {
	c.Id = strings.TrimSpace(c.Id)
	if c.Id == "" {
		c.Id = DefaultID
	}
	c.UpdateId = strings.TrimSpace(c.UpdateId)
	c.TermsURL = strings.TrimSpace(c.TermsURL)
	if c.TermsURL == "" {
		c.TermsURL = brand.TermsURL
	}
	c.PrivacyURL = strings.TrimSpace(c.PrivacyURL)
	if c.PrivacyURL == "" {
		c.PrivacyURL = brand.PrivacyURL
	}
	c.TermsLabel = strings.TrimSpace(c.TermsLabel)
	if c.TermsLabel == "" {
		c.TermsLabel = defaultTermsLabel
	}
	c.PrivacyLabel = strings.TrimSpace(c.PrivacyLabel)
	if c.PrivacyLabel == "" {
		c.PrivacyLabel = defaultPrivacyLabel
	}
	c.Text = strings.TrimSpace(c.Text)
	if c.Text == "" {
		c.Text = defaultText
	}
	if c.MinAgeConfirm < 0 {
		c.MinAgeConfirm = 0
	}
	return c
}

func applyEnv(c Config) Config {
	if v := strings.TrimSpace(os.Getenv("TELLOMI_TOS_ID")); v != "" {
		c.Id = v
	}
	if v := strings.TrimSpace(os.Getenv("TELLOMI_TOS_UPDATE_ID")); v != "" {
		c.UpdateId = v
	}
	if v := strings.TrimSpace(os.Getenv("TELLOMI_TOS_TEXT")); v != "" {
		c.Text = v
	}
	if v := strings.TrimSpace(os.Getenv("TELLOMI_TOS_TERMS_URL")); v != "" {
		c.TermsURL = v
	}
	if v := strings.TrimSpace(os.Getenv("TELLOMI_TOS_PRIVACY_URL")); v != "" {
		c.PrivacyURL = v
	}
	return c
}

// ID is the signup terms id.
func (s *Service) ID() string { return s.cfg.Id }

// UpdateID is the id that forces a popup. Empty means no update.
func (s *Service) UpdateID() string { return s.cfg.UpdateId }

// SignupTerms is the help.termsOfService attached to authorizationSignUpRequired.
func (s *Service) SignupTerms() *mtproto.Help_TermsOfService {
	return s.document(s.cfg.Id)
}

// TermsUpdate is the help.getTermsOfServiceUpdate result.
func (s *Service) TermsUpdate(ctx context.Context, userID int64, now int32) *mtproto.Help_TermsOfServiceUpdate {
	if s.cfg.UpdateId == "" || userID <= 0 {
		return emptyUpdate(now)
	}
	ok, err := s.store.Accepted(ctx, userID, s.cfg.UpdateId)
	if err != nil {
		logx.WithContext(ctx).Errorf("tos: read acceptance user=%d id=%s: %v", userID, s.cfg.UpdateId, err)
		return emptyUpdateAt(now + retryTTL)
	}
	if ok {
		return emptyUpdate(now)
	}
	return mtproto.MakeTLHelpTermsOfServiceUpdate(&mtproto.Help_TermsOfServiceUpdate{
		Expires:        now + EmptyTTL,
		TermsOfService: s.document(s.cfg.UpdateId),
	}).To_Help_TermsOfServiceUpdate()
}

// Accept records that userID accepted the id inside raw (a DataJSON payload
// or the bare id). A missing user or id is a no-op so the RPC can still
// return true. A store error is returned so the client retries.
func (s *Service) Accept(ctx context.Context, userID int64, raw string) error {
	id := parseID(raw)
	if userID <= 0 || id == "" {
		return nil
	}
	return s.store.Accept(ctx, userID, id)
}

// AcceptSignup records acceptance of the signup terms id.
func (s *Service) AcceptSignup(ctx context.Context, userID int64) {
	if err := s.Accept(ctx, userID, s.cfg.Id); err != nil {
		logx.WithContext(ctx).Errorf("tos: record signup acceptance user=%d id=%s: %v", userID, s.cfg.Id, err)
	}
}

func emptyUpdate(now int32) *mtproto.Help_TermsOfServiceUpdate {
	return emptyUpdateAt(now + EmptyTTL)
}

func emptyUpdateAt(expires int32) *mtproto.Help_TermsOfServiceUpdate {
	return mtproto.MakeTLHelpTermsOfServiceUpdateEmpty(&mtproto.Help_TermsOfServiceUpdate{
		Expires: expires,
	}).To_Help_TermsOfServiceUpdate()
}

func (s *Service) document(id string) *mtproto.Help_TermsOfService {
	text, entities := s.body()
	doc := &mtproto.Help_TermsOfService{
		Popup:    true,
		Id:       mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: idJSON(id)}).To_DataJSON(),
		Text:     text,
		Entities: entities,
	}
	if s.cfg.MinAgeConfirm > 0 {
		doc.MinAgeConfirm = wrapperspb.Int32(s.cfg.MinAgeConfirm)
	}
	return mtproto.MakeTLHelpTermsOfService(doc).To_Help_TermsOfService()
}

func (s *Service) body() (string, []*mtproto.MessageEntity) {
	text := s.cfg.Text
	var entities []*mtproto.MessageEntity
	text, entities = ensureLink(text, entities, s.cfg.TermsLabel, s.cfg.TermsURL)
	text, entities = ensureLink(text, entities, s.cfg.PrivacyLabel, s.cfg.PrivacyURL)
	return text, entities
}

func ensureLink(text string, entities []*mtproto.MessageEntity, label, url string) (string, []*mtproto.MessageEntity) {
	if e, ok := textURLEntity(text, label, url); ok {
		return text, append(entities, e)
	}
	if e, ok := rawURLEntity(text, url); ok {
		return text, append(entities, e)
	}
	if label == "" || url == "" {
		return text, entities
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	off := mtproto.UTF16Len(text)
	text += label
	entities = append(entities, mtproto.MakeTLMessageEntityTextUrl(&mtproto.MessageEntity{
		Offset: int32(off),
		Length: int32(mtproto.UTF16Len(label)),
		Url:    url,
	}).To_MessageEntity())
	return text, entities
}

func textURLEntity(text, label, url string) (*mtproto.MessageEntity, bool) {
	if label == "" || url == "" {
		return nil, false
	}
	i := strings.Index(text, label)
	if i < 0 {
		return nil, false
	}
	return mtproto.MakeTLMessageEntityTextUrl(&mtproto.MessageEntity{
		Offset: int32(mtproto.UTF16Len(text[:i])),
		Length: int32(mtproto.UTF16Len(label)),
		Url:    url,
	}).To_MessageEntity(), true
}

func rawURLEntity(text, url string) (*mtproto.MessageEntity, bool) {
	if url == "" {
		return nil, false
	}
	i := strings.Index(text, url)
	if i < 0 {
		return nil, false
	}
	return mtproto.MakeTLMessageEntityUrl(&mtproto.MessageEntity{
		Offset: int32(mtproto.UTF16Len(text[:i])),
		Length: int32(mtproto.UTF16Len(url)),
	}).To_MessageEntity(), true
}

func idJSON(id string) string {
	b, err := json.Marshal(struct {
		ID string `json:"id"`
	}{ID: id})
	if err != nil {
		return `{"id":"` + DefaultID + `"}`
	}
	return string(b)
}

func parseID(data string) string {
	data = strings.TrimSpace(data)
	if data == "" {
		return ""
	}
	if strings.HasPrefix(data, "{") {
		var obj struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(data), &obj); err == nil {
			if id := cleanID(obj.ID); id != "" {
				return id
			}
		}
	}
	if strings.HasPrefix(data, `"`) {
		var s string
		if err := json.Unmarshal([]byte(data), &s); err == nil {
			return cleanID(s)
		}
	}
	return cleanID(data)
}

func cleanID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 {
		return ""
	}
	return id
}
