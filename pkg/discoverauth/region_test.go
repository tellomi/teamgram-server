package discoverauth

import "testing"

func TestContentRegion(t *testing.T) {
	cases := []struct {
		country, phone string
		ip             bool
		want           string
	}{
		{"CN", "", false, RegionCN},
		{"cn", "14155552671", false, RegionCN},
		{"", "8613800138000", false, RegionCN},
		{"", "+8613800138000", false, RegionCN},
		{"US", "14155552671", false, RegionGlobal},
		{"HK", "85251234567", false, RegionGlobal},
		{"", "", true, RegionCN},
		{"US", "14155552671", true, RegionCN},
		{"", "", false, RegionGlobal},
		{"", "user@tellomi.app", false, RegionGlobal},
	}
	for _, tc := range cases {
		if got := ContentRegion(tc.country, tc.phone, tc.ip); got != tc.want {
			t.Errorf("ContentRegion(%q, %q, %v)=%s want %s", tc.country, tc.phone, tc.ip, got, tc.want)
		}
	}
}

func TestMainlandPlace(t *testing.T) {
	if !MainlandPlace("中国", "四川 成都") {
		t.Fatal("chengdu")
	}
	if MainlandPlace("中国", "香港") || MainlandPlace("中国", "台湾 台北") || MainlandPlace("中国", "澳门") {
		t.Fatal("hmt")
	}
	if MainlandPlace("", "") || MainlandPlace("United States", "California") {
		t.Fatal("empty or overseas")
	}
}
