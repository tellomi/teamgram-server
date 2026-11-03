package redact

import (
	"reflect"
	"strings"
)

// mask is the replacement for a secret. Empty secrets stay empty so a
// missing credential is still visible in the startup log.
const mask = "***"

// Value returns a deep copy of v that is safe to pass to logx.Infov or
// logx.Info. v is not modified.
//
// String and byte fields whose names contain Secret, Password, Pass, or Key
// (case-insensitive, including json and yaml names) are replaced with ***.
// That covers Redis Pass, MinIO and S3 secret keys, private keys, and other
// credentials that share those names. Non-secret fields with the same
// substrings, such as an etcd service Key, are masked too.
//
// Every other string is scanned for DSN and URL userinfo. A password there
// is replaced with ***, and the user, host, and database stay so the log
// still shows which database the process opened.
func Value(v any) any {
	if v == nil {
		return nil
	}
	w := &walker{seen: map[uintptr]reflect.Value{}}
	out := w.redact(reflect.ValueOf(v), false)
	if !out.IsValid() || !out.CanInterface() {
		return v
	}
	return out.Interface()
}

// DSN returns s with userinfo passwords replaced by ***.
// MySQL forms such as user:password@tcp(host:port)/db and user:password@unix(/path)/db
// are recognized, as is scheme://user:password@host. An empty password is left
// unchanged. Strings that are not DSNs are returned as they are.
func DSN(s string) string {
	if s == "" || !strings.Contains(s, "@") {
		return s
	}
	next := redactProtoDSN(s)
	next = redactURLUserinfo(next)
	return next
}

type walker struct {
	seen map[uintptr]reflect.Value
}

func (w *walker) redact(v reflect.Value, sensitive bool) reflect.Value {
	if !v.IsValid() {
		return v
	}
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		inner := w.redact(v.Elem(), sensitive)
		out := reflect.New(v.Type()).Elem()
		if inner.IsValid() && inner.Type().AssignableTo(out.Type()) {
			out.Set(inner)
			return out
		}
		return v
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		ptr := v.Pointer()
		if prev, ok := w.seen[ptr]; ok {
			return prev
		}
		out := reflect.New(v.Elem().Type())
		w.seen[ptr] = out
		out.Elem().Set(w.redact(v.Elem(), sensitive))
		return out
	case reflect.Struct:
		return w.redactStruct(v, sensitive)
	case reflect.Slice:
		return w.redactSlice(v, sensitive)
	case reflect.Array:
		return w.redactArray(v, sensitive)
	case reflect.Map:
		return w.redactMap(v, sensitive)
	case reflect.String:
		s := v.String()
		if sensitive {
			if s == "" {
				return v
			}
			return setString(v, mask)
		}
		if red := DSN(s); red != s {
			return setString(v, red)
		}
		return v
	default:
		return v
	}
}

func (w *walker) redactStruct(v reflect.Value, sensitive bool) reflect.Value {
	out := reflect.New(v.Type()).Elem()
	out.Set(v)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		field := v.Field(i)
		childSensitive := sensitive || fieldSensitive(sf)
		redacted := w.redact(field, childSensitive)
		if redacted.IsValid() && out.Field(i).CanSet() && redacted.Type().AssignableTo(out.Field(i).Type()) {
			out.Field(i).Set(redacted)
		}
	}
	return out
}

func (w *walker) redactSlice(v reflect.Value, sensitive bool) reflect.Value {
	if v.IsNil() {
		return v
	}
	if v.Type().Elem().Kind() == reflect.Uint8 {
		return redactBytes(v, sensitive)
	}
	out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
	for i := 0; i < v.Len(); i++ {
		out.Index(i).Set(w.redact(v.Index(i), sensitive))
	}
	return out
}

func (w *walker) redactArray(v reflect.Value, sensitive bool) reflect.Value {
	if v.Type().Elem().Kind() == reflect.Uint8 && sensitive && v.Len() > 0 {
		return reflect.New(v.Type()).Elem()
	}
	out := reflect.New(v.Type()).Elem()
	for i := 0; i < v.Len(); i++ {
		out.Index(i).Set(w.redact(v.Index(i), sensitive))
	}
	return out
}

func (w *walker) redactMap(v reflect.Value, sensitive bool) reflect.Value {
	if v.IsNil() {
		return v
	}
	out := reflect.MakeMapWithSize(v.Type(), v.Len())
	for _, key := range v.MapKeys() {
		// Map keys are data (route names, cache keys), not struct fields.
		// A key such as "/mtproto.RPCPasskey" must not hide its target.
		val := w.redact(v.MapIndex(key), sensitive)
		out.SetMapIndex(key, val)
	}
	return out
}

func redactBytes(v reflect.Value, sensitive bool) reflect.Value {
	if sensitive {
		if v.Len() == 0 {
			return v
		}
		masked := []byte(mask)
		out := reflect.MakeSlice(v.Type(), len(masked), len(masked))
		reflect.Copy(out, reflect.ValueOf(masked))
		return out
	}
	out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
	reflect.Copy(out, v)
	return out
}

func setString(v reflect.Value, s string) reflect.Value {
	out := reflect.New(v.Type()).Elem()
	out.SetString(s)
	return out
}

func fieldSensitive(sf reflect.StructField) bool {
	if sensitiveName(sf.Name) {
		return true
	}
	for _, key := range []string{"json", "yaml"} {
		tag := sf.Tag.Get(key)
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if sensitiveName(name) {
			return true
		}
	}
	return false
}

func sensitiveName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" || n == "-" {
		return false
	}
	for _, frag := range []string{"secret", "password", "passwd", "pass", "key"} {
		if strings.Contains(n, frag) {
			return true
		}
	}
	return false
}

// redactProtoDSN masks passwords in driver DSNs:
//
//	user:password@tcp(host:port)/db
//	user:password@unix(/path)/db
//	user:password@/db
func redactProtoDSN(s string) string {
	ats := protoAtSigns(s)
	if len(ats) == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	last := 0
	changed := false
	for _, at := range ats {
		if at < last {
			continue
		}
		start := userinfoStart(s, at)
		if start < last {
			start = last
		}
		user, pass, ok := splitUserinfo(s[start:at])
		b.WriteString(s[last:start])
		if !ok || pass == "" {
			b.WriteString(s[start:at])
		} else {
			b.WriteString(user)
			b.WriteString(":")
			b.WriteString(mask)
			changed = true
		}
		last = at
	}
	b.WriteString(s[last:])
	if !changed {
		return s
	}
	return b.String()
}

func protoAtSigns(s string) []int {
	var ats []int
	for i := 0; i < len(s); i++ {
		if s[i] != '@' {
			continue
		}
		if isProtoMarker(s, i) || isSlashDSN(s, i) {
			ats = append(ats, i)
		}
	}
	return ats
}

func isProtoMarker(s string, at int) bool {
	i := at + 1
	if i >= len(s) || !isAlpha(s[i]) {
		return false
	}
	i++
	for i < len(s) && isAlphaNum(s[i]) {
		i++
	}
	return i < len(s) && s[i] == '('
}

func isSlashDSN(s string, at int) bool {
	return at+1 < len(s) && s[at+1] == '/'
}

func userinfoStart(s string, at int) int {
	i := at - 1
	for i >= 0 {
		switch s[i] {
		case ' ', '\t', '\n', '\r', '\'', '"', '=', ',':
			return i + 1
		}
		i--
	}
	return 0
}

func splitUserinfo(userinfo string) (user, pass string, ok bool) {
	if userinfo == "" || strings.ContainsAny(userinfo, " \t\r\n/\\") {
		return "", "", false
	}
	colon := strings.Index(userinfo, ":")
	if colon <= 0 {
		return "", "", false
	}
	return userinfo[:colon], userinfo[colon+1:], true
}

func redactURLUserinfo(s string) string {
	var b strings.Builder
	rest := s
	changed := false
	for {
		i := strings.Index(rest, "://")
		if i < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:i+3])
		rest = rest[i+3:]
		at := strings.Index(rest, "@")
		end := strings.IndexAny(rest, "/?#")
		if at < 0 || (end >= 0 && end < at) {
			continue
		}
		user, pass, ok := splitUserinfo(rest[:at])
		if !ok || pass == "" {
			continue
		}
		b.WriteString(user)
		b.WriteString(":")
		b.WriteString(mask)
		rest = rest[at:]
		changed = true
	}
	if !changed {
		return s
	}
	return b.String()
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isAlphaNum(c byte) bool {
	return isAlpha(c) || (c >= '0' && c <= '9')
}
