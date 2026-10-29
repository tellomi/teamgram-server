package verify

import "testing"

func TestLoadDefaultsLeaveEmailDisabled(t *testing.T) {
	t.Setenv("TELLOMI_SMS_PROVIDER", "")
	t.Setenv("TELLOMI_SMS_CODE_LENGTH", "")
	t.Setenv("TELLOMI_EMAIL_PROVIDER", "")
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
	if cfg.Email.Enabled() {
		t.Fatal("email must be disabled when TELLOMI_EMAIL_PROVIDER is unset")
	}
	if cfg.Email.Provider != "" {
		t.Fatalf("email provider %q", cfg.Email.Provider)
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

func TestLoadAliyunDMDoesNotRequireCredentials(t *testing.T) {
	t.Setenv("TELLOMI_EMAIL_PROVIDER", "aliyun_dm")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
	cfg := Load()
	if !cfg.Email.Enabled() {
		t.Fatal("aliyun_dm is an explicit provider")
	}
	if cfg.Email.Endpoint != DefaultDMEndpoint || cfg.Email.Region != DefaultDMRegion {
		t.Fatalf("dm endpoint/region %+v", cfg.Email)
	}
	if cfg.Email.AccountName != DefaultDMAccountName || cfg.Email.FromAlias != DefaultDMFromAlias {
		t.Fatalf("dm identity %+v", cfg.Email)
	}
}
