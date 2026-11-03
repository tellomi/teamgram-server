#!/usr/bin/env bash
# Fail unless every program the teamgram container launches is an executable
# file under <root>/bin. The names are read from docker/entrypoint.sh, from
# shell scripts it execs (runall-docker.sh), and from command/entrypoint
# values in the compose files passed in. There is no fixed list.
set -euo pipefail

fail() {
  echo "verify-teamgram-binaries: $*" >&2
  exit 1
}

declare -A want=()
declare -A parsed=()

reset_sets() {
  want=()
  parsed=()
}

add_name() {
  local name="$1"
  [[ -n "$name" ]] || return 0
  case "$name" in
    *[!A-Za-z0-9_.-]*|.*) return 0 ;;
    *.mmdb|*.yaml|*.yml|*.xdb|*.json|*.key|*.pem|*.sql) return 0 ;;
  esac
  want["$name"]=1
}

is_shell() {
  local f="$1" first
  [[ -f "$f" ]] || return 1
  IFS= read -r first <"$f" || return 1
  first="${first%$'\r'}"
  [[ "$first" == \#!* ]]
}

strip_comment() {
  local raw="$1"
  raw="${raw%$'\r'}"
  printf '%s' "${raw%%#*}"
}

clean_token() {
  local t="$1"
  t="${t//\"/}"
  t="${t//\'/}"
  t="${t#[}"
  t="${t%,}"
  t="${t%]}"
  t="${t%)}"
  printf '%s' "$t"
}

consider_token() {
  local token
  token="$(clean_token "$1")"
  case "$token" in
    /app/bin/*)
      add_name "${token#/app/bin/}"
      ;;
    ./*/*|../*|.|..)
      ;;
    ./*)
      add_name "${token#./}"
      ;;
  esac
}

parse_exec_file() {
  local file="$1"
  [[ -z "${parsed[$file]:-}" ]] || return 0
  parsed["$file"]=1
  local line token
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="$(strip_comment "$line")"
    local trimmed="${line#"${line%%[![:space:]]*}"}"
    [[ -z "$trimmed" ]] && continue
    set -f
    # shellcheck disable=SC2086
    for token in $trimmed; do
      consider_token "$token"
    done
    set +f
  done <"$file"
}

extract_bin_paths() {
  local s="$1" tok matches
  matches="$(grep -oE '/app/bin/[A-Za-z0-9_.-]+' <<<"$s" || true)"
  [[ -n "$matches" ]] || return 0
  while IFS= read -r tok; do
    add_name "${tok#/app/bin/}"
  done <<<"$matches"
}

parse_compose_file() {
  local file="$1"
  local in_exec=0 exec_indent=0 line code indent
  while IFS= read -r line || [[ -n "$line" ]]; do
    code="$(strip_comment "$line")"
    if [[ "$in_exec" -eq 1 ]]; then
      if [[ -z "${code//[[:space:]]/}" ]]; then
        continue
      fi
      indent=0
      if [[ "$code" =~ ^([[:space:]]*) ]]; then
        indent="${#BASH_REMATCH[1]}"
      fi
      if (( indent > exec_indent )); then
        extract_bin_paths "$code"
        continue
      fi
      in_exec=0
    fi
    if [[ "$code" =~ ^([[:space:]]*)(command|entrypoint):[[:space:]]*(.*)$ ]]; then
      in_exec=1
      exec_indent="${#BASH_REMATCH[1]}"
      extract_bin_paths "${BASH_REMATCH[3]}"
    fi
  done <"$file"
}

follow_scripts() {
  local bin="$1"
  local progress=1 name f
  while [[ "$progress" -eq 1 ]]; do
    progress=0
    local names=()
    for name in "${!want[@]}"; do
      names+=("$name")
    done
    for name in "${names[@]}"; do
      f="${bin}/${name}"
      if [[ -z "${parsed[$f]:-}" ]] && is_shell "$f"; then
        parse_exec_file "$f"
        progress=1
      fi
    done
  done
}

collect() {
  local root="$1"
  local compose="${2:-}"
  local bin="${root}/bin"
  local entry="${root}/docker/entrypoint.sh"
  local f
  reset_sets
  [[ -f "$entry" ]] || fail "missing ${entry}"
  [[ -d "$bin" ]] || fail "missing ${bin}"
  parse_exec_file "$entry"
  if [[ -n "$compose" ]]; then
    [[ -d "$compose" ]] || fail "missing compose dir ${compose}"
    local files=()
    shopt -s nullglob
    files=("$compose"/*.yaml "$compose"/*.yml)
    shopt -u nullglob
    [[ ${#files[@]} -gt 0 ]] || fail "no compose files in ${compose}"
    for f in "${files[@]}"; do
      parse_compose_file "$f"
    done
  fi
  follow_scripts "$bin"
  [[ ${#want[@]} -gt 0 ]] || fail "no executables derived from ${entry}"
}

check_files() {
  local bin="$1"
  local missing=0 name path
  while IFS= read -r name; do
    path="${bin}/${name}"
    if [[ -f "$path" && -x "$path" ]]; then
      echo "verify-teamgram-binaries: ok ${path}"
    else
      echo "verify-teamgram-binaries: missing or not executable: ${path}" >&2
      missing=1
    fi
  done < <(printf '%s\n' "${!want[@]}" | sort)
  [[ "$missing" -eq 0 ]] || fail "refusing to publish an image with missing binaries"
  echo "verify-teamgram-binaries: ${#want[@]} executables in ${bin}"
}

self_test() {
  local repo tmp names_file expect_file
  repo="$(cd "$(dirname "$0")/.." && pwd)"
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064
  trap "rm -rf '$tmp'" RETURN
  mkdir -p "$tmp/app/bin" "$tmp/app/docker" "$tmp/compose"
  cp "$repo/teamgramd/docker/entrypoint.sh" "$tmp/app/docker/entrypoint.sh"
  cp "$repo/teamgramd/bin/runall-docker.sh" "$tmp/app/bin/runall-docker.sh"
  chmod +x "$tmp/app/bin/runall-docker.sh"
  printf '\n# start httpserver ./httpserver -f=../etc2/httpserver.yaml\n' >>"$tmp/app/bin/runall-docker.sh"
  cp "$repo"/docker-compose*.yaml "$tmp/compose/"

  names_file="$tmp/names"
  expect_file="$tmp/expect"
  bash "$0" --list "$tmp/app" "$tmp/compose" | sort >"$names_file"
  awk '
    /^[[:space:]]*#/ { next }
    {
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^\.\/[A-Za-z0-9_.-]+$/) print substr($i, 3)
      }
    }
  ' "$tmp/app/docker/entrypoint.sh" "$tmp/app/bin/runall-docker.sh" | sort -u >"$expect_file"
  diff -u "$expect_file" "$names_file" || fail "derived list does not match entrypoint and runall-docker.sh"
  grep -qx bff "$names_file" || fail "bff was not derived from the entrypoint"
  if grep -qx httpserver "$names_file" || grep -qx 'GeoLite2-City.mmdb' "$names_file"; then
    fail "commented httpserver or mounted mmdb was required"
  fi

  local name
  while IFS= read -r name; do
    if [[ ! -e "$tmp/app/bin/$name" ]]; then
      printf '#!/bin/sh\nexit 0\n' >"$tmp/app/bin/$name"
      chmod +x "$tmp/app/bin/$name"
    fi
  done <"$names_file"
  bash "$0" "$tmp/app" "$tmp/compose" >/dev/null

  rm -f "$tmp/app/bin/bff"
  if bash "$0" "$tmp/app" "$tmp/compose" >/dev/null 2>&1; then
    fail "missing bff was accepted"
  fi
  printf '#!/bin/sh\nexit 0\n' >"$tmp/app/bin/bff"
  chmod a-x "$tmp/app/bin/bff"
  if bash "$0" "$tmp/app" "$tmp/compose" >/dev/null 2>&1; then
    fail "non-executable bff was accepted"
  fi
  chmod +x "$tmp/app/bin/bff"

  cat >"$tmp/compose/extra.yaml" <<'EOF'
services:
  tool:
    environment:
      MMDB: /app/bin/GeoLite2-City.mmdb
    command:
      - /app/bin/mediaretention
      - -f
      - /app/etc2/media.yaml
EOF
  if bash "$0" "$tmp/app" "$tmp/compose" >/dev/null 2>&1; then
    fail "compose command /app/bin/mediaretention was not required"
  fi
  bash "$0" --list "$tmp/app" "$tmp/compose" | grep -qx mediaretention || fail "mediaretention was not derived from compose"
  if bash "$0" --list "$tmp/app" "$tmp/compose" | grep -qx 'GeoLite2-City.mmdb'; then
    fail "compose environment path was treated as a binary"
  fi
  printf '#!/bin/sh\nexit 0\n' >"$tmp/app/bin/mediaretention"
  chmod +x "$tmp/app/bin/mediaretention"
  bash "$0" "$tmp/app" "$tmp/compose" >/dev/null
  echo "verify-teamgram-binaries: self-test ok"
}

if [[ "${1:-}" == "--self-test" ]]; then
  self_test
  exit 0
fi

mode="verify"
if [[ "${1:-}" == "--list" ]]; then
  mode="list"
  shift
fi

root="${1:-/app}"
compose="${2:-}"
collect "$root" "$compose"
if [[ "$mode" == "list" ]]; then
  printf '%s\n' "${!want[@]}" | sort
  exit 0
fi
check_files "${root}/bin"
