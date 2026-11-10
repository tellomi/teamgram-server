package location

import "testing"

func TestVenueType(t *testing.T) {
	cases := map[string]string{
		"050301": "food/default",
		"050500": "food/cafe",
		"06":     "shops/default",
		"07":     "shops/default",
		"0801":   "arts_entertainment/default",
		"0901":   "building/medical",
		"1001":   "travel/hotel",
		"1102":   "parks_outdoors/default",
		"1200":   "building/default",
		"1301":   "building/government_capitolbuilding",
		"1401":   "education/default",
		"15":     "travel/default",
		"1505":   "travel/airport",
		"1502":   "travel/trainstation",
		"1503":   "travel/subway",
		"1601":   "shops/financial",
		"":       "building/default",
		"99":     "building/default",
	}
	for code, want := range cases {
		if got := VenueType(code); got != want {
			t.Errorf("%s → %s, want %s", code, got, want)
		}
	}
}

func TestTencentVenueType(t *testing.T) {
	if got := TencentVenueType("美食:中餐厅"); got != "food/default" {
		t.Fatal(got)
	}
	if got := TencentVenueType("酒店宾馆"); got != "building/default" && got != "travel/hotel" {
		// "酒店宾馆" does not match the exact head "酒店".
		if got != "building/default" {
			t.Fatal(got)
		}
	}
	if got := TencentVenueType("酒店"); got != "travel/hotel" {
		t.Fatal(got)
	}
}

func TestAppleVenueType(t *testing.T) {
	cases := map[string]string{
		"Cafe":       "food/cafe",
		"Restaurant": "food/default",
		"Hotel":      "travel/hotel",
		"Park":       "parks_outdoors/default",
		"Store":      "shops/default",
		"Nightlife":  "nightlife/default",
		"Hospital":   "building/medical",
		"Airport":    "travel/airport",
		"School":     "education/default",
		"Museum":     "arts_entertainment/default",
		"":           "building/default",
	}
	for in, want := range cases {
		if got := AppleVenueType(in); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
}
