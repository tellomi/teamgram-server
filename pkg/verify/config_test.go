package verify

import "testing"

func TestLoadDefaultsUseLogEmail(t *testing.T) {
	t.Setenv("TELLOMI_SMS_PROVIDER", "")
	t.Setenv("TELLOMI_SMS_CODE_LENGTH", "")
	t.Setenv("TELLOMI_EMAIL_PROVIDER", "")
	t.Setenv("TELLOMI_EMAIL_FROM", "")
	t.Setenv("TELLOMI_EMAIL_FROM_NAME", "")
	t.Setenv("TELLOMI_EMAIL_CODE_LENGTH", "")
	t.Setenv("TELLOMI_EMAIL_RESEND_SECONDS", "")
	t.Setenv("TELLOMI_EMAIL_DAILY_LIMIT", "")
	t.Setenv("TELLOMI_EMAIL_IP_DAILY_LIMIT", "")
	t.Setenv("TELLOMI_EMAIL_MAX_ATTEMPTS", "")
	t.Setenv("TELLOMI_EMAIL_CODE_TTL_SECONDS", "")
	t.Setenv("TELLOMI_POSTAL_BASE_URL", "")
	t.Setenv("TELLOMI_POSTAL_API_KEY", "")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
	t.Setenv("TELLOMI_ALIYUN_SMS_SIGN_NAME", "")
	t.Setenv("TELLOMI_ALIYUN_SMS_TEMPLATE_CODE", "")
	t.Setenv("TELLOMI_ALIYUN_DM_ACCOUNT_NAME", "")
	t.Setenv("TELLOMI_SMS_ALLOW_INTERNATIONAL", "")
	t.Setenv("TELLOMI_EMAIL_SETUP_REQUIRED", "")

	cfg := Load()
	if cfg.SMS.Provider != ProviderFixed {
		t.Fatalf("sms provider %q", cfg.SMS.Provider)
	}
	if !cfg.Email.Enabled() || cfg.Email.Provider != ProviderLog {
		t.Fatalf("unset email provider must be log, got %+v", cfg.Email)
	}
	if cfg.Email.From != DefaultEmailFrom || cfg.Email.FromName != DefaultEmailFromName {
		t.Fatalf("from %+v", cfg.Email)
	}
	if cfg.Email.CodeLength != 6 {
		t.Fatalf("email code length %d", cfg.Email.CodeLength)
	}
	if cfg.Email.PostalBaseURL != "" || cfg.Email.PostalAPIKey != "" {
		t.Fatal("postal credentials must come from the environment")
	}
	if cfg.SMS.AllowInternational {
		t.Fatal("international SMS defaults off")
	}
	if cfg.SMS.CodeLength != 6 {
		t.Fatalf("code length %d", cfg.SMS.CodeLength)
	}
	if cfg.SMS.SignName != DefaultSMSSignName || cfg.SMS.TemplateCode != DefaultSMSTemplateCode {
		t.Fatalf("sms defaults: %+v", cfg.SMS)
	}
	if cfg.SMS.AccessKeyID != "" || cfg.SMS.AccessKeySecret != "" {
		t.Fatal("access keys must come from the environment")
	}
	if cfg.Email.SetupRequired {
		t.Fatal("email setup is not required by default")
	}
	if cfg.Limits.ResendSeconds != 60 || cfg.Limits.DailyPhone != 10 || cfg.Limits.MaxAttempts != 5 {
		t.Fatalf("limits %+v", cfg.Limits)
	}
	if cfg.EmailLimits.ResendSeconds != 60 || cfg.EmailLimits.DailyPhone != 10 || cfg.EmailLimits.DailyIP != 30 || cfg.EmailLimits.MaxAttempts != 5 || cfg.EmailLimits.TTLSeconds != 180 {
		t.Fatalf("email limits %+v", cfg.EmailLimits)
	}
}

func TestLoadLogAndAliyunProviders(t *testing.T) {
	t.Setenv("TELLOMI_SMS_PROVIDER", "aliyun")
	t.Setenv("TELLOMI_EMAIL_PROVIDER", "log")
	t.Setenv("TELLOMI_SMS_ALLOW_INTERNATIONAL", "true")
	t.Setenv("TELLOMI_SMS_CODE_LENGTH", "6")
	t.Setenv("TELLOMI_EMAIL_SETUP_REQUIRED", "1")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "id")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "secret")

	cfg := Load()
	if cfg.SMS.Provider != ProviderAliyun || !cfg.SMS.AllowInternational || cfg.SMS.CodeLength != 6 {
		t.Fatalf("sms %+v", cfg.SMS)
	}
	if !cfg.Email.Enabled() || cfg.Email.Provider != ProviderLog || !cfg.Email.SetupRequired {
		t.Fatalf("email %+v", cfg.Email)
	}
	if cfg.SMS.AccessKeyID != "id" || cfg.Email.AccessKeySecret != "secret" {
		t.Fatal("standard Aliyun env vars were not read")
	}
}

func TestLoadUnknownEmailProviderDisablesEmail(t *testing.T) {
	t.Setenv("TELLOMI_EMAIL_PROVIDER", "smtp")
	cfg := Load()
	if cfg.Email.Enabled() || cfg.Email.Provider != "" {
		t.Fatalf("unknown provider must disable email, got %+v", cfg.Email)
	}
}

func TestLoadPostalDoesNotRequireCredentialsAtStartup(t *testing.T) {
	t.Setenv("TELLOMI_EMAIL_PROVIDER", "postal")
	t.Setenv("TELLOMI_POSTAL_BASE_URL", "")
	t.Setenv("TELLOMI_POSTAL_API_KEY", "")
	t.Setenv("TELLOMI_EMAIL_FROM", "")
	t.Setenv("TELLOMI_EMAIL_FROM_NAME", "")
	cfg := Load()
	if !cfg.Email.Enabled() || cfg.Email.Provider != ProviderPostal {
		t.Fatalf("postal %+v", cfg.Email)
	}
	if cfg.Email.From != DefaultEmailFrom || cfg.Email.FromName != DefaultEmailFromName {
		t.Fatalf("from %+v", cfg.Email)
	}
	if cfg.Email.PostalBaseURL != "" || cfg.Email.PostalAPIKey != "" {
		t.Fatal("empty postal settings must not be invented")
	}
}

func TestLoadAliyunDMIsNotTheP0Sender(t *testing.T) {
	t.Setenv("TELLOMI_EMAIL_PROVIDER", "aliyun_dm")
	cfg := Load()
	if cfg.Email.Enabled() || cfg.Email.Provider != "" {
		t.Fatalf("aliyun_dm must not send, got %+v", cfg.Email)
	}
}

func TestLoadDisabledEmail(t *testing.T) {
	t.Setenv("TELLOMI_EMAIL_PROVIDER", "disabled")
	cfg := Load()
	if cfg.Email.Enabled() || cfg.Email.Provider != "" {
		t.Fatalf("%+v", cfg.Email)
	}
}
