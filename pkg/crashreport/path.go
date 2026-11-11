package crashreport

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	// deviceAssignRE / appAssignRE prefer the caption fields the iOS client
	// sends: "app=<version> device=<device>". The looser patterns still run
	// when those labels are absent.
	deviceAssignRE = regexp.MustCompile(`(?i)\bdevice[=: ]+([^\s]+)`)
	appAssignRE    = regexp.MustCompile(`(?i)\bapp[=: ]+([0-9]+(?:\.[0-9]+){1,3})`)
	deviceRE       = regexp.MustCompile(`(?i)\b(iphone(?:\d+,?\d*)?|ipad(?:\d+,?\d*)?|ios[\d.]{0,8}|android[\d.]{0,8})\b`)
	appRE          = regexp.MustCompile(`(?i)(?:\bapp[=: ]+|\btellomi[-/ ])([0-9]+(?:\.[0-9]+){1,3})`)
)

// ObjectPath is the key inside the crash-reports class:
// YYYY/MM/DD/<sender>/<msg_id>_<filename>. The stored object key is
// crash-reports/ plus this path.
func ObjectPath(at time.Time, sender int64, msgID int32, filename string) string {
	if at.IsZero() {
		at = time.Now()
	}
	at = at.UTC()
	return fmt.Sprintf("%04d/%02d/%02d/%d/%d_%s", at.Year(), int(at.Month()), at.Day(), sender, msgID, safeName(filename))
}

// FullKey is the object key operators see in listings.
func FullKey(path string) string {
	path = strings.TrimPrefix(strings.TrimSpace(path), "/")
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, Class+"/") {
		return path
	}
	return Class + "/" + path
}

// PathOf strips the class prefix so presign and Head use the path inside the class.
func PathOf(storageKey string) string {
	key := strings.TrimPrefix(strings.TrimSpace(storageKey), "/")
	return strings.TrimPrefix(key, Class+"/")
}

func safeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		case r > 127:
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._")
	if out == "" {
		return "file.bin"
	}
	if utf8.RuneCountInString(out) > 80 {
		rs := []rune(out)
		out = string(rs[:80])
	}
	return out
}

// Hints pulls a short device and app token out of the filename, caption, and
// message. Explicit app= / device= caption fields win over a token found in
// the filename (Tellomi-iOS-<version>-crash.zip would otherwise report "iOS"
// and hide "iPhone15,2"). Empty strings mean nothing matched; the raw fields
// are still stored.
func Hints(filename, caption, message string) (device, app string) {
	blob := filename + "\n" + caption + "\n" + message
	if m := deviceAssignRE.FindStringSubmatch(blob); len(m) > 1 {
		device = trimHint(m[1])
	} else if m := deviceRE.FindStringSubmatch(blob); len(m) > 1 {
		device = trimHint(m[1])
	}
	if m := appAssignRE.FindStringSubmatch(blob); len(m) > 1 {
		app = trimHint(m[1])
	} else if m := appRE.FindStringSubmatch(blob); len(m) > 1 {
		app = trimHint(m[1])
	}
	return device, app
}

func trimHint(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 128 {
		s = s[:128]
	}
	return s
}
