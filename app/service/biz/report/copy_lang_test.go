package report

import (
	"strings"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/langpack"
)

func TestReportNoticesCoverEveryLanguage(t *testing.T) {
	en := Receipt("en", "T12", etaText(24, "en"))
	if en != "We received your report. Ticket T12. Expected time: 24 hours. You will get the result after a platform review." {
		t.Fatal(en)
	}
	if etaText(1, "en") != "1 hour" {
		t.Fatal(etaText(1, "en"))
	}
	ja := Receipt("ja-JP", "T12", etaText(3, "ja"))
	if strings.Contains(ja, "We received") || !strings.Contains(ja, "通報") || !strings.Contains(ja, "3時間") {
		t.Fatal(ja)
	}
	if !strings.Contains(Receipt("zh-hans", "T1", etaText(24, "zh-hans")), "经平台审核") {
		t.Fatal("simplified")
	}
	if !strings.Contains(Receipt("zh-Hant-HK", "T8", etaText(3, "zh-HK")), "經平台審核") {
		t.Fatal("traditional")
	}
	if !strings.Contains(DecisionForReporter("zh-rTW", StatusClear, "T1"), "經平台審核") {
		t.Fatal("zh-rTW clear")
	}
	for _, lang := range langpack.OfficialLanguages {
		code := lang.Code
		receipt := Receipt(code, "T12", etaText(24, code))
		viol := DecisionForReporter(code, StatusViolation, "T1、T2")
		clear := DecisionForReporter(code, StatusClear, "T1")
		subject := DecisionForSubject(code, StatusViolation)
		if receipt == "" || viol == "" || clear == "" || subject == "" {
			t.Fatalf("%s empty", code)
		}
		if DecisionForSubject(code, StatusClear) != "" {
			t.Fatalf("%s clear subject", code)
		}
		for _, text := range []string{receipt, viol, clear, subject, etaText(1, code)} {
			if strings.Contains(strings.ToLower(text), "ai") {
				t.Fatalf("%s names a model: %s", code, text)
			}
		}
		if code == "en" {
			continue
		}
		if receipt == en || viol == DecisionForReporter("en", StatusViolation, "T1、T2") || subject == DecisionForSubject("en", StatusViolation) {
			t.Fatalf("%s fell back to English", code)
		}
	}
}
