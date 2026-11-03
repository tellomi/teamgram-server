#!/usr/bin/env bash
# Remove old tellomi teamgram/asr image tags to free root disk.
#   tellomi-image-cleanup.sh [--dry-run] [--keep <sha7>]...
#   tellomi-image-cleanup.sh --self-test
#
# Keeps: every 7-hex tag a container was created from (running or stopped),
# every tag named with --keep, and the TELLOMI_KEEP_RECENT (default 2) newest
# other teamgram tags. The same sha set is kept for asr. Newest is image
# creation time, then the local tag time (Metadata.LastTagTime, set on pull),
# then the git commit time in TELLOMI_DEPLOY_DIR, then the tag string. Equal
# creation times must not fall through to an unstable order: that deleted the
# rollback tag when two images shared a Created timestamp.
#
# Only 7-hex tags of */tellomi/teamgram and */tellomi/asr are touched.
# Never touches volumes, containers, /opt/tellomi/backups, or other images.
# Then runs "docker image prune -f" (dangling layers only). Logs to syslog.
set -euo pipefail

REPO_RE='/tellomi/(teamgram|asr)$'
KEEP_RECENT="${TELLOMI_KEEP_RECENT:-2}"
DRY=0
declare -A KEEP=()

fail() {
  echo "tellomi-image-cleanup: $*" >&2
  exit 1
}

log() {
  echo "$*"
  logger -t tellomi-image-cleanup -- "$*" 2>/dev/null || true
}

avail_gb() {
  df -BG --output=avail / | tail -1 | tr -dc '0-9'
}

# pull_epoch is the local tag time in unix seconds, or 0 when Docker has none.
pull_epoch() {
  local id="$1" raw epoch
  raw="$(docker image inspect -f '{{.Metadata.LastTagTime}}' "$id" 2>/dev/null || true)"
  case "$raw" in
    ""|*"no value"*|0001-01-01*)
      printf '0'
      return 0
      ;;
  esac
  epoch="$(date -d "$raw" +%s 2>/dev/null || true)"
  if [[ ! "$epoch" =~ ^[0-9]+$ ]]; then
    printf '0'
    return 0
  fi
  printf '%s' "$epoch"
}

# git_epoch is the commit time of the 7-hex tag, or 0 when the checkout has no such object.
git_epoch() {
  local tag="$1" dir epoch
  dir="${TELLOMI_DEPLOY_DIR:-/opt/tellomi/teamgram-server}"
  if [[ ! -e "$dir/.git" ]]; then
    printf '0'
    return 0
  fi
  epoch="$(git -C "$dir" log -1 --format=%ct "$tag" 2>/dev/null || true)"
  if [[ ! "$epoch" =~ ^[0-9]+$ ]]; then
    printf '0'
    return 0
  fi
  printf '%s' "$epoch"
}

# select_recent reads newest-first rows (created pull git ref id) and adds
# teamgram shas to KEEP until KEEP_RECENT new ones are chosen. Shas already
# in KEEP do not consume a slot.
select_recent() {
  local n=0 row ref sha
  while IFS= read -r row; do
    [[ -n "$row" ]] || continue
    IFS=' ' read -r _created _pull _git ref _id <<<"$row"
    [[ "$ref" == */teamgram:* ]] || continue
    sha="${ref##*:}"
    [[ -n "${KEEP[$sha]:-}" ]] && continue
    if (( n >= KEEP_RECENT )); then
      break
    fi
    KEEP["$sha"]=1
    n=$((n + 1))
  done
}

self_test() {
  local sorted first
  KEEP=()
  KEEP_RECENT=1
  # Same Created. ffffff1 sorts first by tag, but 0000001 was pulled later
  # and is the rollback image. The pull-time field must win.
  sorted="$(printf '%s\n' \
    '2020-01-01T00:00:00Z 0000000001 0000000001 reg.example/tellomi/teamgram:ffffff1 idF' \
    '2020-01-01T00:00:00Z 0000000009 0000000001 reg.example/tellomi/teamgram:0000001 id0' \
    | sort -r)"
  first="$(awk 'NR==1 { print $4 }' <<<"$sorted")"
  [[ "$first" == *":0000001" ]] || fail "creation-time tie kept ${first}"
  select_recent <<<"$sorted"
  [[ -n "${KEEP[0000001]:-}" ]] || fail "did not keep the later pull 0000001"
  [[ -z "${KEEP[ffffff1]:-}" ]] || fail "deleted the rollback image's slot for ffffff1"

  # Same Created and same pull time: later git commit is kept.
  KEEP=()
  KEEP_RECENT=1
  sorted="$(printf '%s\n' \
    '2020-01-01T00:00:00Z 0000000005 0000000001 reg.example/tellomi/teamgram:aaaaaaa idA' \
    '2020-01-01T00:00:00Z 0000000005 0000000008 reg.example/tellomi/teamgram:bbbbbbb idB' \
    | sort -r)"
  select_recent <<<"$sorted"
  [[ -n "${KEEP[bbbbbbb]:-}" ]] || fail "did not keep the later commit bbbbbbb"
  [[ -z "${KEEP[aaaaaaa]:-}" ]] || fail "kept the older commit aaaaaaa"

  # A newer Created still beats an older image with a later pull time.
  KEEP=()
  KEEP_RECENT=1
  sorted="$(printf '%s\n' \
    '2020-01-02T00:00:00Z 0000000001 0000000001 reg.example/tellomi/teamgram:1111111 id1' \
    '2020-01-01T00:00:00Z 0000000099 0000000099 reg.example/tellomi/teamgram:2222222 id2' \
    | sort -r)"
  select_recent <<<"$sorted"
  [[ -n "${KEEP[1111111]:-}" ]] || fail "newer Created lost to pull time"
  [[ -z "${KEEP[2222222]:-}" ]] || fail "older image consumed the only slot"

  # In-use sha does not consume a recent slot. asr rows are not a slot either.
  KEEP=()
  KEEP[1111111]=1
  KEEP_RECENT=1
  sorted="$(printf '%s\n' \
    '2020-01-03T00:00:00Z 0000000001 0000000001 reg.example/tellomi/teamgram:1111111 id1' \
    '2020-01-03T00:00:00Z 0000000001 0000000001 reg.example/tellomi/asr:1111111 id1a' \
    '2020-01-02T00:00:00Z 0000000001 0000000001 reg.example/tellomi/teamgram:3333333 id3' \
    '2020-01-01T00:00:00Z 0000000001 0000000001 reg.example/tellomi/teamgram:4444444 id4' \
    | sort -r)"
  select_recent <<<"$sorted"
  [[ -n "${KEEP[1111111]:-}" && -n "${KEEP[3333333]:-}" ]] || fail "in-use tag consumed the rollback slot"
  [[ -z "${KEEP[4444444]:-}" ]] || fail "kept a third teamgram tag"
  echo "tellomi-image-cleanup: self-test ok"
}

if [[ "${1:-}" == "--self-test" ]]; then
  self_test
  exit 0
fi

if [[ ! "$KEEP_RECENT" =~ ^[0-9]+$ ]]; then
  echo "TELLOMI_KEEP_RECENT must be an integer" >&2
  exit 2
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY=1 ;;
    --keep)
      if [[ $# -lt 2 ]]; then
        echo "usage: $0 [--dry-run] [--keep sha7]..." >&2
        exit 2
      fi
      shift
      if [[ "$1" =~ ^[0-9a-f]{7}$ ]]; then
        KEEP["$1"]=1
      fi
      ;;
    *)
      echo "usage: $0 [--dry-run] [--keep sha7]..." >&2
      exit 2
      ;;
  esac
  shift
done

exec 9>/run/tellomi-image-cleanup.lock
flock -w 600 9 || {
  log "another cleanup holds the lock; skipping"
  exit 0
}

declare -A USED=()
while IFS= read -r id; do
  [[ -n "$id" ]] || continue
  img="$(docker inspect -f '{{.Image}}' "$id")"
  USED["$img"]=1
done < <(docker ps -aq)

# Rows are "created pull_epoch git_epoch repo:tag id", newest first.
# pull_epoch is Metadata.LastTagTime. git_epoch breaks a remaining tie.
mapfile -t ROWS < <(
  docker images --no-trunc --format '{{.Repository}} {{.Tag}} {{.ID}}' \
    | awk -v re="$REPO_RE" '$1 ~ re && $2 ~ /^[0-9a-f]{7}$/ { print $1, $2, $3 }' \
    | while read -r repo tag id; do
        created="$(docker image inspect -f '{{.Created}}' "$id")"
        printf '%s %010d %010d %s:%s %s\n' \
          "$created" "$(pull_epoch "$id")" "$(git_epoch "$tag")" "$repo" "$tag" "$id"
      done \
    | sort -r
)

while IFS= read -r c; do
  [[ -n "$c" ]] || continue
  ref="$(docker inspect -f '{{.Config.Image}}' "$c")"
  # Untagged ref. The image-id guard below still refuses to delete that image.
  if [[ "$ref" =~ $REPO_RE ]]; then
    continue
  fi
  base="${ref%:*}"
  tag="${ref##*:}"
  if [[ "$base" =~ $REPO_RE && "$tag" =~ ^[0-9a-f]{7}$ ]]; then
    KEEP["$tag"]=1
  fi
done < <(docker ps -aq)

# Redirect, not a pipe: a pipe would run select_recent in a subshell and drop KEEP.
if [[ ${#ROWS[@]} -gt 0 ]]; then
  select_recent < <(printf '%s\n' "${ROWS[@]}")
fi

before="$(avail_gb)"
if [[ ${#KEEP[@]} -eq 0 ]]; then
  keep_list="-"
else
  keep_list="${!KEEP[*]}"
fi
log "start dry_run=${DRY} avail=${before}G keep=[${keep_list}]"
removed=0
if [[ ${#ROWS[@]} -gt 0 ]]; then
  for row in "${ROWS[@]}"; do
    IFS=' ' read -r _created _pull _git ref id <<<"$row"
    sha="${ref##*:}"
    [[ -n "${KEEP[$sha]:-}" ]] && continue
    if [[ -n "${USED[$id]:-}" ]]; then
      if [[ "$DRY" -eq 1 ]]; then
        log "would untag ${ref} (image in use)"
      elif docker rmi "$ref" >/dev/null 2>&1; then
        log "untagged ${ref} (image in use)"
      fi
      continue
    fi
    if [[ "$DRY" -eq 1 ]]; then
      log "would remove ${ref}"
    elif docker rmi "$ref" >/dev/null 2>&1; then
      log "removed ${ref}"
    else
      log "could not remove ${ref}"
    fi
    removed=$((removed + 1))
  done
fi
if [[ "$DRY" -eq 0 ]]; then
  docker image prune -f >/dev/null 2>&1 || true
fi
log "done dry_run=${DRY} tags=${removed} avail_before=${before}G avail_after=$(avail_gb)G"
exit 0
