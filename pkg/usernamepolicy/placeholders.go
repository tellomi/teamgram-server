package usernamepolicy

import "sort"

// AppendixCPlaceholders splits requirements appendix C / D-56 names that
// appendix D does not assign to an account.
//
// occupied are public usernames: 4–32 characters, a letter first, then
// [a-z0-9_], and not ending in an underscore. That is the same shape
// account.checkUsername accepts (MinUsernameLen in the user service is 4).
// The migration stores each one as an empty placeholder, peer_type 0 and
// peer_id 0. utf8mb4_unicode_ci makes WeChat and WECHAT the same row.
//
// unresolved are shorter than that minimum. checkUsername returns
// USERNAME_INVALID before it reads the table, so a row would not change
// registration or resolution. qq, jd, and x are slug-only in the overlay.
// xai is three characters.
func AppendixCPlaceholders() (occupied, unresolved []string) {
	seen := map[string]struct{}{}
	for _, spec := range overlaySpecs() {
		for _, word := range spec.words {
			name := Normalize(word)
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			if _, assigned := officialOwner[name]; assigned {
				continue
			}
			if !placeholderUsername(name) {
				unresolved = append(unresolved, name)
				continue
			}
			occupied = append(occupied, name)
		}
	}
	sort.Strings(occupied)
	sort.Strings(unresolved)
	return occupied, unresolved
}

func placeholderUsername(name string) bool {
	if len(name) < 4 || len(name) > 32 {
		return false
	}
	if name[0] < 'a' || name[0] > 'z' {
		return false
	}
	if name[len(name)-1] == '_' {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
		default:
			return false
		}
	}
	return true
}
