package verify

import (
	"os"
	"strconv"
	"strings"
)

const (
	ProviderFixed    = "fixed"
	ProviderAliyun   = "aliyun"
	ProviderLog      = "log"
	ProviderAliyunDM = "aliyun_dm"

	DefaultRegion          = "cn-hangzhou"
	DefaultSMSSignName     = "重庆半格智能"
	DefaultSMSTemplateCode = "SMS_338570180"
	DefaultDMAccountName   = "noreply@mail.tellomi.cn"
	DefaultDMFromAlias     = "Tellomi"
	DefaultSMSEndpoint     = "https://dysmsapi.aliyuncs.com/"
	DefaultDMEndpoint      = "https://dm.aliyuncs.com/"
	DefaultDMRegion        = "cn-hangzhou"

	SMSTemplateText = "您的验证码为：${code}，请勿泄露于他人！"
)

// Config is process configuration for verification delivery.
// Secrets are read from the environment at startup and are never given defaults.
type Config struct {
	SMS    SMSConfig
	Email  EmailConfig
	Limits Limits
}

type SMSConfig struct {
	// Provider is "fixed" (dev code 12345) or "aliyun".
	Provider           string
	AllowInternational bool
	CodeLength         int
	Region             string
	SignName           string
	TemplateCode       string
	Endpoint           string
	AccessKeyID        string
	AccessKeySecret    string
}

type EmailConfig struct {
	// Provider is "" (disabled), "log", or "aliyun_dm".
	Provider        string
	SetupRequired   bool
	AccountName     string
	FromAlias       string
	Endpoint        string
	Region          string
	AccessKeyID     string
	AccessKeySecret string
}

func (e EmailConfig) Enabled() bool {
	switch e.Provider {
	case ProviderLog, ProviderAliyunDM:
		return true
	default:
		return false
	}
}

type Limits struct {
	ResendSeconds int
	DailyPhone    int
	DailyIP       int
	MaxAttempts   int
	TTLSeconds    int
}

// Load reads TELLOMI_* and the standard Aliyun credential variables.
// Missing email configuration does not fail; an unset TELLOMI_EMAIL_PROVIDER
// leaves email sign-in disabled.
func Load() Config {
	smsProvider := strings.ToLower(strings.TrimSpace(os.Getenv("TELLOMI_SMS_PROVIDER")))
	switch smsProvider {
	case "", ProviderFixed, "none":
		smsProvider = ProviderFixed
	case ProviderAliyun:
	default:
		smsProvider = ProviderFixed
	}

	emailProvider := strings.ToLower(strings.TrimSpace(os.Getenv("TELLOMI_EMAIL_PROVIDER")))
	switch emailProvider {
	case ProviderLog, ProviderAliyunDM:
	default:
		emailProvider = ""
	}

	codeLen := envInt("TELLOMI_SMS_CODE_LENGTH", 5)
	if codeLen != 5 && codeLen != 6 {
		codeLen = 5
	}

	return Config{
		SMS: SMSConfig{
			Provider:           smsProvider,
			AllowInternational: envBool("TELLOMI_SMS_ALLOW_INTERNATIONAL", false),
			CodeLength:         codeLen,
			Region:             envString("TELLOMI_ALIYUN_REGION", DefaultRegion),
			SignName:           envString("TELLOMI_ALIYUN_SMS_SIGN_NAME", DefaultSMSSignName),
			TemplateCode:       envString("TELLOMI_ALIYUN_SMS_TEMPLATE_CODE", DefaultSMSTemplateCode),
			Endpoint:           envString("TELLOMI_ALIYUN_SMS_ENDPOINT", DefaultSMSEndpoint),
			AccessKeyID:        strings.TrimSpace(os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")),
			AccessKeySecret:    strings.TrimSpace(os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")),
		},
		Email: EmailConfig{
			Provider:        emailProvider,
			SetupRequired:   envBool("TELLOMI_EMAIL_SETUP_REQUIRED", false),
			AccountName:     envString("TELLOMI_ALIYUN_DM_ACCOUNT_NAME", DefaultDMAccountName),
			FromAlias:       envString("TELLOMI_ALIYUN_DM_FROM_ALIAS", DefaultDMFromAlias),
			Endpoint:        envString("TELLOMI_ALIYUN_DM_ENDPOINT", DefaultDMEndpoint),
			Region:          DefaultDMRegion,
			AccessKeyID:     strings.TrimSpace(os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")),
			AccessKeySecret: strings.TrimSpace(os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")),
		},
		Limits: Limits{
			ResendSeconds: envInt("TELLOMI_SMS_RESEND_SECONDS", 60),
			DailyPhone:    envInt("TELLOMI_SMS_DAILY_LIMIT", 10),
			DailyIP:       envInt("TELLOMI_SMS_IP_DAILY_LIMIT", 30),
			MaxAttempts:   envInt("TELLOMI_SMS_MAX_ATTEMPTS", 5),
			TTLSeconds:    envInt("TELLOMI_SMS_CODE_TTL_SECONDS", 180),
		},
	}
}

func envString(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "":
		return def
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}
