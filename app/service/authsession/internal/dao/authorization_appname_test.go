package dao

import "testing"

func TestAppNameForSession(t *testing.T) {
	cases := []struct {
		apiID    int32
		langPack string
		want     string
	}{
		{apiID: 100001, langPack: "android", want: "Tellomi"},
		{apiID: 100001, langPack: "", want: "Tellomi"},
		{apiID: 100001, langPack: "teamgram", want: "Tellomi"},
		{apiID: 6, langPack: "Teamgram", want: "Tellomi"},
		{apiID: 6, langPack: "android", want: "android"},
		{apiID: 0, langPack: "", want: ""},
		{apiID: 2040, langPack: "tdesktop", want: "tdesktop"},
	}
	for _, tc := range cases {
		if got := appNameForSession(tc.apiID, tc.langPack); got != tc.want {
			t.Errorf("appNameForSession(%d, %q) = %q, want %q", tc.apiID, tc.langPack, got, tc.want)
		}
	}
}
