package tellomiai

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/app/bff/discoverrel"
	"github.com/zeromicro/go-zero/core/logx"
)

// Start turns the service on when TELLOMI_AI=1 and TELLOMI_AI_API_KEY is set.
// A missing key is logged and does not stop bff. Login is not involved.
// addr "off" still answers private chats and does not listen for the engine.
// users resolves TELLOMI_AI_ALLOW_USERNAMES. A nil directory with a username
// list fails closed (the busy sentence) until the lookup is available.
func Start(dsn string, sender Sender, subjects Subjects, users UsernameDirectory) {
	if !Advertised() {
		return
	}
	ledger := NewLedger()
	ledger.MonthBudget = envFloat("TELLOMI_AI_MONTHLY_BUDGET")
	if perK := envFloat("TELLOMI_AI_PRICE_PER_1K"); perK > 0 {
		ledger.PricePerToken = perK / 1000
	}
	svc := &Service{
		enabled: true,
		Client: NewClient(
			getenv(EnvBaseURL),
			getenv(EnvAPIKey),
			getenv(EnvModelPrimary),
			getenv(EnvModelSmall),
			getenv(EnvModelFallback),
			envDurationSeconds(EnvTimeoutSeconds, defaultTimeout),
		),
		Ledger: ledger,
		Sender: sender,
		Allow:  AllowFromEnv(users),
	}
	if g, ok := sender.(GroupSender); ok {
		svc.Group = g
	}
	if strings.TrimSpace(dsn) != "" {
		db, err := OpenMySQL(dsn)
		if err != nil {
			logx.Error("tellomi ai: reply log is process memory only")
		} else {
			svc.Recorder = db
		}
		if subjects == nil {
			dir, err := discoverrel.OpenMySQL(dsn)
			if err != nil {
				logx.Error("tellomi ai: subject lookup is unavailable")
			} else {
				subjects = dir
			}
		}
	}
	Use(svc)
	addr := getenv(EnvAddr)
	if addr == "" {
		addr = defaultAddr
	}
	if addr == "off" {
		return
	}
	key, err := decodeKey(getenv(EnvInternalKey))
	if err != nil {
		logx.Error("tellomi ai: internal key is missing or invalid; engine endpoint is off")
		return
	}
	if subjects == nil {
		logx.Error("tellomi ai: no subject directory; engine endpoint is off")
		return
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler{Service: svc, Subjects: subjects, Key: key},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		logx.Infof("tellomi ai listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Errorf("tellomi ai http failed")
		}
	}()
}

// StartClosed installs a service that does not call the model.
// Private messages and group mentions still receive DisabledMessage.
func StartClosed(sender Sender) {
	svc := &Service{enabled: false, Sender: sender}
	if g, ok := sender.(GroupSender); ok {
		svc.Group = g
	}
	Use(svc)
}

func decodeKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errEmptyDSN
	}
	buf, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		buf, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil || len(buf) < 32 {
		return nil, errEmptyDSN
	}
	return buf, nil
}
