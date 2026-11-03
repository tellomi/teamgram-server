#!/bin/bash
# Exercises the MySQL DSN rewrite in entrypoint.sh. No Docker.
set -euo pipefail

root="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$root/entrypoint.sh"

fail() {
  echo "entrypoint_mysql_test: $*" >&2
  exit 1
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/clean"
printf '%s\n' 'DSN: teamgram:teamgram@tcp(mysql:3306)/teamgram?charset=utf8mb4' > "$tmp/clean/a.yaml"
printf '%s\n' 'ChannelMySQL: teamgram:previous@tcp(mysql:3306)/teamgram' > "$tmp/clean/b.yaml"
printf '%s\n' 'Name: interface.gateway' > "$tmp/clean/c.yaml"
rewrite_teamgram_mysql_dsns 's3cret#/&' "$tmp/clean" || fail "clean rewrite"
grep -Fq 'DSN: teamgram:s3cret#/&@tcp(mysql:3306)/teamgram?charset=utf8mb4' "$tmp/clean/a.yaml" || fail "a.yaml password"
grep -Fq 'ChannelMySQL: teamgram:s3cret#/&@tcp(mysql:3306)/teamgram' "$tmp/clean/b.yaml" || fail "previous password was not replaced"
grep -Fq 'Name: interface.gateway' "$tmp/clean/c.yaml" || fail "unrelated yaml changed"
if grep -q 'teamgram:teamgram@' "$tmp/clean"/*.yaml; then
  fail "default credential survived a non-default password"
fi

mkdir -p "$tmp/spaces"
printf '%s\n' 'DSN: teamgram:teamgram@tcp(mysql:3306)/teamgram' > "$tmp/spaces/a.yaml"
rewrite_teamgram_mysql_dsns 'p a s s' "$tmp/spaces" || fail "password with spaces"
grep -Fq 'teamgram:p a s s@tcp(mysql:3306)' "$tmp/spaces/a.yaml" || fail "spaces"

mkdir -p "$tmp/dev"
printf '%s\n' 'DSN: teamgram:teamgram@tcp(mysql:3306)/teamgram' > "$tmp/dev/a.yaml"
rewrite_teamgram_mysql_dsns 'teamgram' "$tmp/dev" || fail "dev default password must be allowed to remain"
grep -Fq 'teamgram:teamgram@tcp(mysql:3306)' "$tmp/dev/a.yaml" || fail "dev DSN"

mkdir -p "$tmp/missed"
printf '%s\n' 'DSN: teamgram:teamgram@tcp(mysql:3306)/teamgram' > "$tmp/missed/a.yaml"
printf '%s\n' 'DSN: teamgram:teamgram@tcp(127.0.0.1:3306)/teamgram' > "$tmp/missed/skipped.yaml"
if rewrite_teamgram_mysql_dsns 'rotated' "$tmp/missed"; then
  fail "a DSN the pattern does not match must keep the process from starting"
fi
grep -Fq 'teamgram:rotated@tcp(mysql:3306)' "$tmp/missed/a.yaml" || fail "matching DSN should still be rewritten before the check"
grep -Fq 'teamgram:teamgram@tcp(127.0.0.1:3306)' "$tmp/missed/skipped.yaml" || fail "non-matching DSN should be left as written"

echo "ok"
