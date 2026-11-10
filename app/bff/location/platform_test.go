package location

import "testing"

func TestClassifyPlatform(t *testing.T) {
	cases := []struct {
		lang, model, system string
		api                 int32
		want                string
	}{
		{"ios", "", "", 0, platformIOS},
		{"IOS", "Pixel", "SDK 35", 4, platformIOS},
		{"android", "iPhone 16", "18.5", 8, platformAndroid},
		{"", "iPhone 16", "18.5", 0, platformIOS},
		{"", "iPad", "", 6, platformIOS},
		{"", "", "iPadOS 17", 0, platformIOS},
		{"", "Samsung SM-S928B", "SDK 35", 100001, platformAndroid},
		{"", "", "Android 14", 0, platformAndroid},
		{"", "", "", 8, platformIOS},
		{"", "", "", 4, platformAndroid},
		{"", "", "", 6, platformAndroid},
		{"tdesktop", "PC", "Windows 11", 2040, platformUnknown},
		{"", "", "", 100001, platformUnknown},
		{"", "", "", 10840, platformUnknown},
	}
	for _, tc := range cases {
		if got := Classify(tc.lang, tc.model, tc.system, tc.api); got != tc.want {
			t.Errorf("Classify(%q,%q,%q,%d)=%s want %s", tc.lang, tc.model, tc.system, tc.api, got, tc.want)
		}
	}
}

func TestRouteProviders(t *testing.T) {
	if got := route(platformIOS, true, true); len(got) != 1 || got[0] != providerApple {
		t.Fatalf("ios %+v", got)
	}
	if got := route(platformIOS, false, true); len(got) != 0 {
		t.Fatalf("ios without apple %+v", got)
	}
	if got := route(platformAndroid, true, true); len(got) != 1 || got[0] != providerAMap {
		t.Fatalf("android %+v", got)
	}
	if got := route(platformAndroid, true, false); len(got) != 0 {
		t.Fatalf("android without amap %+v", got)
	}
	got := route(platformUnknown, true, true)
	if len(got) != 2 || got[0] != providerApple || got[1] != providerAMap {
		t.Fatalf("unknown %+v", got)
	}
	if got = route(platformUnknown, false, false); len(got) != 0 {
		t.Fatalf("off %+v", got)
	}
}
