package core

import "github.com/teamgram/proto/mtproto"

// mergePhonePrivacyException adds peer to the phone-number allow list and
// drops peer from the disallow list. Disallow is checked before allow, so
// leaving the id on both would keep the number hidden.
func mergePhonePrivacyException(rules []*mtproto.PrivacyRule, peer int64) []*mtproto.PrivacyRule {
	if peer == 0 {
		return rules
	}
	var allow []int64
	out := make([]*mtproto.PrivacyRule, 0, len(rules)+1)
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		switch rule.GetPredicateName() {
		case mtproto.Predicate_privacyValueAllowUsers:
			allow = append(allow, rule.GetUsers()...)
		case mtproto.Predicate_privacyValueDisallowUsers:
			kept := withoutUser(rule.GetUsers(), peer)
			if len(kept) == 0 {
				continue
			}
			out = append(out, mtproto.MakeTLPrivacyValueDisallowUsers(&mtproto.PrivacyRule{
				Users: kept,
			}).To_PrivacyRule())
		default:
			out = append(out, rule)
		}
	}
	if !containsUser(allow, peer) {
		allow = append(allow, peer)
	}
	out = append(out, mtproto.MakeTLPrivacyValueAllowUsers(&mtproto.PrivacyRule{
		Users: allow,
	}).To_PrivacyRule())
	return out
}

func withoutUser(ids []int64, peer int64) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != peer {
			out = append(out, id)
		}
	}
	return out
}

func containsUser(ids []int64, peer int64) bool {
	for _, id := range ids {
		if id == peer {
			return true
		}
	}
	return false
}
