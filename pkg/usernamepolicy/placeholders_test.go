package usernamepolicy

import (
	"os"
	"strings"
	"testing"
)

func TestAppendixCPlaceholders(t *testing.T) {
	occupied, unresolved := AppendixCPlaceholders()
	if len(occupied) < 50 {
		t.Fatalf("occupied %d", len(occupied))
	}
	wantOccupied := []string{"wechat", "google", "openai", "telegram", "facebook", "twitter", "botfather", "line"}
	got := map[string]struct{}{}
	for _, name := range occupied {
		if _, assigned := officialOwner[name]; assigned {
			t.Fatalf("assigned name %s is a placeholder", name)
		}
		if !placeholderUsername(name) {
			t.Fatalf("not a public username: %s", name)
		}
		got[name] = struct{}{}
	}
	for _, name := range wantOccupied {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing %s", name)
		}
	}
	if len(unresolved) != 4 {
		t.Fatalf("unresolved = %v", unresolved)
	}
	for i, name := range []string{"jd", "qq", "x", "xai"} {
		if unresolved[i] != name {
			t.Fatalf("unresolved[%d] = %s", i, unresolved[i])
		}
		if placeholderUsername(name) {
			t.Fatalf("%s should not be a public username", name)
		}
	}
	if os.Getenv("DUMP_APPENDIX_C_SQL") == "1" {
		if err := os.WriteFile(os.Getenv("DUMP_APPENDIX_C_SQL_PATH"), []byte(renderAppendixCSQL(occupied)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func renderAppendixCSQL(occupied []string) string {
	quoted := make([]string, len(occupied))
	values := make([]string, len(occupied))
	doubled := make([]string, len(occupied))
	for i, name := range occupied {
		quoted[i] = "'" + name + "'"
		values[i] = "('" + name + "', 0, 0, 0, 0, 0, 0)"
		doubled[i] = "''" + name + "''"
	}
	var b strings.Builder
	b.WriteString(`-- D-56 / appendix D last row. Appendix C names that no official account owns.
--
-- account.checkUsername treats any username row as taken, including
-- peer_type 0. TELLOMI_USERNAME_POLICY defaults to off, so these rows are
-- what stops a normal user from registering the exact name. Case follows
-- utf8mb4_unicode_ci: WeChat and WECHAT are this row.
--
-- peer_type 0 and peer_id 0 are an empty placeholder. There is no user
-- and no channel. user.resolveUsername returns USERNAME_NOT_OCCUPIED for
-- that peer type, the same as the unassigned bot names in
-- migrate-20261105-official-accounts.sql. contacts.resolveUsername cannot
-- open a chat. account.checkUsername still returns USERNAME_OCCUPIED.
--
-- The list is pkg/usernamepolicy.AppendixCPlaceholders, the overlay in
-- spec.go whose source is requirements-v2/appendix-c, minus names appendix
-- D already assigned. qq, jd, x, and xai are shorter than the public
-- username minimum (4) and are not inserted. Underscore affixes
-- (wechat_01) and the 470-term lexicon are not rows; those still need
-- TELLOMI_USERNAME_POLICY=1.
--
-- username.id is AUTO_INCREMENT. A second run changes nothing. If any of
-- these names already belongs to another peer, the script stops and does
-- not take it (D-26). allowlist.toml is in tellomi/tellomi, not this repo.

SET @bad = (SELECT username FROM username WHERE username IN (
`)
	b.WriteString(strings.Join(quoted, ",\n"))
	b.WriteString(`
) AND NOT (peer_type = 0 AND peer_id = 0) LIMIT 1);
CALL teamgram_fail_if_set(@bad, 'D-56: reserved username taken: ');

INSERT IGNORE INTO username (username, peer_type, peer_id, editable, active, order2, deleted) VALUES
`)
	b.WriteString(strings.Join(values, ",\n"))
	b.WriteString(";\n\n")
	b.WriteString("SET @teamgram_stmt = 'UPDATE username SET peer_type = 0, peer_id = 0, editable = 0, active = 0, order2 = 0, deleted = 0 WHERE peer_type = 0 AND peer_id = 0 AND username IN (")
	b.WriteString(strings.Join(doubled, ", "))
	b.WriteString(")';\n")
	b.WriteString(`PREPARE teamgram_ps FROM @teamgram_stmt;
EXECUTE teamgram_ps;
DEALLOCATE PREPARE teamgram_ps;
`)
	return b.String()
}
