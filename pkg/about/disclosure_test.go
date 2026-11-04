package about

import "testing"

func TestDisclosureDefaultsOff(t *testing.T) {
	t.Setenv(EnvEnabled, "")
	t.Setenv(EnvAlgorithmFiling, "网信算备示例")
	t.Setenv(EnvAIModel, "Qwen")
	t.Setenv(EnvAIRegistration, "示例登记")
	if PersonalizedRecommendDefault() || AlgorithmFilingNumber() != "" || AIModelName() != "" || AIRegistrationNumber() != "" {
		t.Fatal("disclosure published while the flag is unset")
	}
	if HashFragment() != "about=0 filing= model= aireg=" {
		t.Fatalf("hash fragment %q", HashFragment())
	}
}

func TestDisclosureRequiresFlagValueOne(t *testing.T) {
	t.Setenv(EnvAlgorithmFiling, "网信算备示例")
	for _, v := range []string{"", "0", "true", "yes", "on"} {
		t.Setenv(EnvEnabled, v)
		if PersonalizedRecommendDefault() {
			t.Fatalf("value %q advertised the switch", v)
		}
	}
}

func TestFilingNumberAdvertisesTheDefault(t *testing.T) {
	t.Setenv(EnvEnabled, "1")
	t.Setenv(EnvAlgorithmFiling, "")
	t.Setenv(EnvAIModel, "")
	t.Setenv(EnvAIRegistration, "")
	off := HashFragment()
	if PersonalizedRecommendDefault() || AlgorithmFilingNumber() != "" {
		t.Fatal("empty filing number advertised the switch")
	}
	t.Setenv(EnvAlgorithmFiling, "  网信算备110108000000000000001号  ")
	if !PersonalizedRecommendDefault() || AlgorithmFilingNumber() != "网信算备110108000000000000001号" {
		t.Fatalf("filing %q default %v", AlgorithmFilingNumber(), PersonalizedRecommendDefault())
	}
	if HashFragment() == off {
		t.Fatal("filing number did not change the hash fragment")
	}
}

func TestRejectsUnsafeDisclosureText(t *testing.T) {
	t.Setenv(EnvEnabled, "1")
	for _, bad := range []string{
		"https://telegram.org/faq",
		"https://t.me/tellomi",
		"line\nbreak",
		"has\ttab",
		stringsRepeat("号", maxRunes+1),
	} {
		t.Setenv(EnvAlgorithmFiling, bad)
		if AlgorithmFilingNumber() != "" || PersonalizedRecommendDefault() {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestAIStringsIndependentOfFilingNumber(t *testing.T) {
	t.Setenv(EnvEnabled, "1")
	t.Setenv(EnvAlgorithmFiling, "")
	t.Setenv(EnvAIModel, "通义千问")
	t.Setenv(EnvAIRegistration, "Beijing-GenAI-0001")
	if PersonalizedRecommendDefault() {
		t.Fatal("model name turned the recommendation switch on")
	}
	if AIModelName() != "通义千问" || AIRegistrationNumber() != "Beijing-GenAI-0001" {
		t.Fatalf("model %q reg %q", AIModelName(), AIRegistrationNumber())
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
