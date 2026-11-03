#!/bin/bash
# Image entrypoint. Rewrites the MySQL password baked into /app/etc2, applies
# SQL migrations, then replaces this shell with runall so it is PID 1.

# rewrite_teamgram_mysql_line replaces the password in
# teamgram:<password>@tcp(mysql:3306). The password is inserted literally.
rewrite_teamgram_mysql_line() {
  local line="$1" pw="$2"
  local prefix="teamgram:" suffix="@tcp(mysql:3306)"
  local out="" rest="$line"
  while [[ "$rest" == *"$prefix"* ]]; do
    local head="${rest%%"$prefix"*}"
    local tail="${rest#*"$prefix"}"
    if [[ "$tail" != *"$suffix"* ]]; then
      out+="$rest"
      rest=""
      break
    fi
    local after="${tail#*"$suffix"}"
    out+="${head}${prefix}${pw}${suffix}"
    rest="$after"
  done
  printf '%s' "${out}${rest}"
}

# rewrite_teamgram_mysql_dsns rewrites every /app/etc2 yaml (or the directory
# passed in). When TEAMGRAM_MYSQL_PASSWORD is not the dev default "teamgram",
# teamgram:teamgram@ left behind means a DSN was missed and startup must stop.
# The dev default is itself "teamgram", so that string remaining is expected.
rewrite_teamgram_mysql_dsns() {
  local pw="$1"
  local dir="$2"
  local f tmp line mode
  local had_nullglob=0
  local rc=0
  if shopt -q nullglob; then
    had_nullglob=1
  fi
  shopt -s nullglob
  local files=("$dir"/*.yaml)
  if [[ ${#files[@]} -eq 0 ]]; then
    echo "no yaml files in ${dir}" >&2
    rc=1
  else
    for f in "${files[@]}"; do
      mode="$(stat -c %a "$f")"
      tmp="$(mktemp)" || { rc=1; break; }
      while IFS= read -r line || [[ -n "$line" ]]; do
        printf '%s\n' "$(rewrite_teamgram_mysql_line "$line" "$pw")"
      done < "$f" > "$tmp"
      mv "$tmp" "$f"
      chmod "$mode" "$f"
    done
  fi
  if [[ "$rc" -eq 0 ]] && grep -q 'teamgram:teamgram@' "$dir"/*.yaml; then
    if [[ "$pw" != "teamgram" ]]; then
      echo "refusing to start: teamgram:teamgram@ is still present in ${dir}" >&2
      rc=1
    fi
  fi
  if [[ "$had_nullglob" -eq 0 ]]; then
    shopt -u nullglob
  fi
  return "$rc"
}

if [[ "${BASH_SOURCE[0]}" != "$0" ]]; then
  return 0
fi

if [[ -n "${TEAMGRAM_MYSQL_PASSWORD:-}" ]]; then
  if ! rewrite_teamgram_mysql_dsns "$TEAMGRAM_MYSQL_PASSWORD" /app/etc2; then
    exit 1
  fi
fi

# Apply SQL migrations before the processes start. A new image can ship a
# migrate-*.sql that production never ran by hand (chat_participants.rank2).
if ! /app/deploy/migrate.sh; then
  echo "schema migration failed" >&2
  exit 1
fi

cd /app/bin
# Replace this shell so runall is PID 1 and receives SIGTERM.
exec ./runall-docker.sh
