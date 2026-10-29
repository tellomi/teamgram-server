package redact

import "testing"

func TestCredential(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"min", "***"},
		{"abcd", "***"},
		{"abcde", "***bcde"},
		{"r2-access-key-ABCDEFGH1234", "***1234"},
		{"r2-secret-value-WXYZ9876", "***9876"},
	}
	for _, tc := range cases {
		if got := Credential(tc.in); got != tc.want {
			t.Errorf("Credential(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if tc.in != "" && Credential(tc.in) == tc.in {
			t.Errorf("Credential(%q) returned the original value", tc.in)
		}
	}
}
