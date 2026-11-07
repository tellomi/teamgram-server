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
	if err := RequireCountry("8613800138000", false, []string{"86"}); err != nil {
		t.Fatal(err)
	}
	if err := RequireCountry("14155552671", false, nil); err != ErrInternationalDisabled {
		t.Fatalf("default allow list: %v", err)
	}
	if err := RequireCountry("14155552671", false, []string{"86", "1"}); err != nil {
		t.Fatal(err)
	}
	if err := RequireCountry("14155552671", true, []string{"86"}); err != nil {
		t.Fatal(err)
	}
}
