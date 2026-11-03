package usernamepolicy

import (
	"encoding/json"
	"strings"
	"testing"
)

func fx(id, term string, matches, fields []string, outcome string) map[string]any {
	return fxRegions(id, term, matches, fields, []string{"global"}, outcome)
}

func fxRegions(id, term string, matches, fields, regions []string, outcome string) map[string]any {
	normalized := Normalize(term)
	skeleton := ""
	if containsStr(matches, MatchConfusable) {
		skeleton = Skeleton(normalized)
	}
	return map[string]any{
		"id": id, "term": term, "normalized": normalized, "skeleton": skeleton,
		"matches": matches, "fields": fields, "regions": regions, "outcome": outcome, "source": "engine_test",
	}
}

func TestFixtureMatrix(t *testing.T) {
	rawRules := []any{
		fx("reserved:admin", "admin", []string{MatchNormalizedExact, MatchPrefix, MatchSuffix}, []string{FieldUsername, FieldSlug, FieldDisplayName, FieldGroupName}, OutcomeReserved),
		fx("reserved:support", "support", []string{MatchNormalizedExact}, []string{FieldUsername, FieldSlug, FieldDisplayName}, OutcomeReserved),
		fx("reserved:staff", "staff", []string{MatchNormalizedExact}, []string{FieldUsername, FieldSlug}, OutcomeReserved),
		fx("impersonation:moderator", "moderator", []string{MatchNormalizedExact, MatchPrefix}, []string{FieldUsername, FieldSlug, FieldDisplayName}, OutcomeImpersonation),
		fx("brand:tellomi", "tellomi", []string{MatchNormalizedExact, MatchPrefix, MatchSuffix, MatchConfusable}, []string{FieldUsername, FieldSlug, FieldDisplayName, FieldGroupName}, OutcomeBrandProtected),
		fx("impersonation:tellomi_support", "tellomi_support", []string{MatchNormalizedExact, MatchConfusable}, []string{FieldUsername, FieldSlug, FieldDisplayName}, OutcomeImpersonation),
		fx("security:verify", "verify_account", []string{MatchNormalizedExact, MatchContains}, []string{FieldDisplayName, FieldGroupName}, OutcomeSecuritySensitive),
		fxRegions("cn:example", "敏感词示例", []string{MatchContains}, []string{FieldDisplayName, FieldGroupName}, []string{"cn"}, OutcomeRegionalRestricted),
	}
	body, err := json.Marshal(map[string]any{
		"name": "policy", "version": 2026092201, "schema": 1,
		"payload": map[string]any{
			"rules": rawRules,
			"allow": []any{map[string]any{"normalized": "admin", "fields": []string{FieldGroupName}, "regions": []string{"global"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	e, err := Load(body)
	if err != nil {
		t.Fatal(err)
	}
	global := []string{"global"}
	cn := []string{"global", "cn"}
	want := func(input, field string, regions []string, outcome string) {
		t.Helper()
		got := e.Check(input, field, regions)
		if got.Outcome != outcome {
			t.Fatalf("%s %q field %s: got %s rule %s, want %s", field, input, field, got.Outcome, got.RuleID, outcome)
		}
	}
	for _, s := range []string{"admin", "Admin", "ADMIN"} {
		want(s, FieldUsername, global, OutcomeReserved)
	}
	want("ＡＤＭＩＮ", FieldDisplayName, global, OutcomeReserved)
	want("ad\u200Bmin", FieldDisplayName, global, OutcomeInvalidUnicode)
	for _, s := range []string{"tel1omi", "te11omi", "tellorni", "t3llomi"} {
		want(s, FieldUsername, global, OutcomeBrandProtected)
	}
	want("admin_x", FieldUsername, global, OutcomeReserved)
	want("admin_team", FieldUsername, global, OutcomeReserved)
	want("administer", FieldUsername, global, OutcomeAllowed)
	want("administrator2", FieldUsername, global, OutcomeAllowed)
	want("badminton", FieldUsername, global, OutcomeAllowed)
	want("admin01", FieldUsername, global, OutcomeAllowed)
	want("team_admin", FieldUsername, global, OutcomeReserved)
	want("sysadmin", FieldUsername, global, OutcomeAllowed)
	want("please verify_account now", FieldDisplayName, global, OutcomeSecuritySensitive)
	want("the badminton club", FieldDisplayName, global, OutcomeAllowed)
	want("tellomi_support", FieldUsername, global, OutcomeImpersonation)
	want("support", FieldUsername, global, OutcomeReserved)
	want("staff", FieldDisplayName, global, OutcomeAllowed)
	want("Admin", FieldGroupName, global, OutcomeAllowed)
	want("Admin", FieldUsername, global, OutcomeReserved)
	phrase := "这是敏感词示例的一句话"
	want(phrase, FieldDisplayName, global, OutcomeAllowed)
	want(phrase, FieldDisplayName, cn, OutcomeRegionalRestricted)
	hit := e.Check("tellomi_support", FieldUsername, global)
	if hit.RuleID != "impersonation:tellomi_support" || hit.IsAllowed() {
		t.Fatalf("hit %+v", hit)
	}
	if !e.Check("xiaoming", FieldUsername, cn).IsAllowed() {
		t.Fatal("xiaoming refused")
	}
}

func TestLoadRejectsBadLexicon(t *testing.T) {
	short, _ := json.Marshal(map[string]any{
		"name": "policy", "version": 1, "schema": 1,
		"payload": map[string]any{"rules": []any{
			fx("bad:short", "adm", []string{MatchContains}, []string{FieldUsername}, OutcomeReserved),
		}, "allow": []any{}},
	})
	if _, err := Load(short); err == nil || !strings.Contains(err.Error(), "CONTAINS") {
		t.Fatalf("short contains: %v", err)
	}
	other, _ := json.Marshal(map[string]any{"name": "stickers", "version": 1, "schema": 1, "payload": map[string]any{"rules": []any{}, "allow": []any{}}})
	if _, err := Load(other); err == nil {
		t.Fatal("accepted stickers envelope")
	}
}
