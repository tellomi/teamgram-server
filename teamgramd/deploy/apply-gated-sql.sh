#!/usr/bin/env bash
# Apply teamgramd/deploy/gated/*.sql.
# migrate.sh and teamgramd/docker/entrypoint.sh do not call this script.
# A normal process start must not ALTER users or create these tables.
set -euo pipefail

if [ "${TEAMGRAM_SQL_BACKUP_CONFIRMED:-}" != "yes" ]; then
  echo "refusing gated SQL: dump MySQL with mysqldump --single-transaction, restore that dump on a spare instance, then set TEAMGRAM_SQL_BACKUP_CONFIRMED=yes" >&2
  exit 2
fi

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=migrate.sh
source "$here/migrate.sh"

echo "installing migration helpers"
mysql_cli < "$here/sql/0_migrate_helpers.sql"

shopt -s nullglob
files=("$here"/gated/*.sql)
if [ "${#files[@]}" -eq 0 ]; then
  echo "no files under $here/gated" >&2
  exit 1
fi
IFS=$'\n' files=($(printf '%s\n' "${files[@]}" | sort))
unset IFS

mysql_cli <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
  version VARCHAR(255) NOT NULL,
  applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
SQL

for f in "${files[@]}"; do
  ver="gated/$(basename "$f")"
  echo "apply $ver"
  mysql_cli < "$f"
  mysql_cli -e "INSERT IGNORE INTO schema_migrations (version) VALUES ('${ver}')"
done

echo "gated SQL applied; story advertisement writes stay failed until both story_advert_dirty and story_live_owner exist"
