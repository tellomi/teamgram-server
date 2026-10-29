#!/usr/bin/env bash
# Apply teamgramd/deploy/sql/migrate-*.sql in order, once each.
# Statements are idempotent (see sql/0_migrate_helpers.sql). schema_migrations
# records filenames so a later deploy does not repeat them.
#
# Connection, first match wins:
#   MYSQL_HOST / MYSQL_PORT / MYSQL_USER / MYSQL_PASSWORD / MYSQL_DATABASE
#   MYSQL_DSN  user:pass@tcp(host:port)/db
#   defaults   teamgram:teamgram@tcp(mysql:3306)/teamgram
set -euo pipefail

SQL_DIR="${TEAMGRAM_SQL_DIR:-}"
if [ -z "$SQL_DIR" ]; then
  SQL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/sql" && pwd)"
fi

parse_mysql_dsn() {
  local dsn="$1"
  local rest userinfo hostport
  userinfo="${dsn%%@*}"
  rest="${dsn#*@}"
  MYSQL_USER="${userinfo%%:*}"
  MYSQL_PASSWORD="${userinfo#*:}"
  rest="${rest#tcp(}"
  hostport="${rest%%)*}"
  rest="${rest#*)}"
  rest="${rest#/}"
  MYSQL_HOST="${hostport%%:*}"
  MYSQL_PORT="${hostport##*:}"
  MYSQL_DATABASE="${rest%%\?*}"
}

if [ -z "${MYSQL_HOST:-}" ] && [ -n "${MYSQL_DSN:-}" ]; then
  parse_mysql_dsn "$MYSQL_DSN"
fi

MYSQL_HOST="${MYSQL_HOST:-mysql}"
MYSQL_PORT="${MYSQL_PORT:-3306}"
MYSQL_USER="${MYSQL_USER:-teamgram}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:-teamgram}"
MYSQL_DATABASE="${MYSQL_DATABASE:-teamgram}"

mysql_cli() {
  MYSQL_PWD="$MYSQL_PASSWORD" mysql \
    --protocol=TCP \
    --connect-timeout=5 \
    -h "$MYSQL_HOST" \
    -P "$MYSQL_PORT" \
    -u "$MYSQL_USER" \
    --default-character-set=utf8mb4 \
    "$MYSQL_DATABASE" \
    "$@"
}

apply_migrations() {
  local i
  echo "waiting for mysql at ${MYSQL_HOST}:${MYSQL_PORT}/${MYSQL_DATABASE}"
  for i in $(seq 1 60); do
    if mysql_cli -e "SELECT 1" >/dev/null 2>&1; then
      break
    fi
    if [ "$i" -eq 60 ]; then
      echo "mysql not reachable at ${MYSQL_HOST}:${MYSQL_PORT}" >&2
      return 1
    fi
    sleep 2
  done

  echo "installing migration helpers"
  mysql_cli < "$SQL_DIR/0_migrate_helpers.sql"

  mysql_cli <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
  version VARCHAR(255) NOT NULL,
  applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
SQL

  local f ver count
  shopt -s nullglob
  local files=("$SQL_DIR"/migrate-*.sql)
  if [ "${#files[@]}" -eq 0 ]; then
    echo "no migrate-*.sql under $SQL_DIR" >&2
    return 1
  fi
  IFS=$'\n' files=($(printf '%s\n' "${files[@]}" | sort))
  unset IFS

  for f in "${files[@]}"; do
    ver="$(basename "$f")"
    count="$(mysql_cli -N -e "SELECT COUNT(*) FROM schema_migrations WHERE version='${ver}'")"
    count="${count//[[:space:]]/}"
    if [ "$count" = "1" ]; then
      echo "skip $ver"
      continue
    fi
    echo "apply $ver"
    mysql_cli < "$f"
    # IGNORE: the app entrypoint and the mysql-migrate service can apply the
    # same file at the same time. The statements are idempotent.
    mysql_cli -e "INSERT IGNORE INTO schema_migrations (version) VALUES ('${ver}')"
  done

  if [ -s "$SQL_DIR/z_init.sql" ]; then
    ver="z_init.sql"
    count="$(mysql_cli -N -e "SELECT COUNT(*) FROM schema_migrations WHERE version='${ver}'")"
    count="${count//[[:space:]]/}"
    if [ "$count" != "1" ]; then
      echo "apply $ver"
      mysql_cli < "$SQL_DIR/z_init.sql"
      mysql_cli -e "INSERT IGNORE INTO schema_migrations (version) VALUES ('${ver}')"
    else
      echo "skip $ver"
    fi
  fi

  echo "schema migrations complete"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  apply_migrations
fi
