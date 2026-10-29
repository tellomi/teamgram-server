// Package redact masks credentials before they are written to logs.
package redact

// Credential hides a secret in logs.
// An empty value stays empty so a missing credential is still visible.
// Values of four runes or fewer become "***". Longer values keep only the last four.
func Credential(v string) string {
	if v == "" {
		return ""
	}
	r := []rune(v)
	if len(r) <= 4 {
		return "***"
	}
	return "***" + string(r[len(r)-4:])
}
