package verify

import "testing"

func TestFormatAliyunPhone(t *testing.T) {
	got, err := FormatAliyunPhone("8613800138000", false)
	if err != nil || got != "13800138000" {
		t.Fatalf("china: %q %v", got, err)
	}
	if _, err := FormatAliyunPhone("14155552671", false); err != ErrInternationalDisabled {
		t.Fatalf("intl blocked: %v", err)
	}
	got, err = FormatAliyunPhone("14155552671", true)
	if err != nil || got != "14155552671" {
		t.Fatalf("intl allowed: %q %v", got, err)
	}
	if IsChina("13800138000") {
		t.Fatal("national form is not E.164")
	}
}
