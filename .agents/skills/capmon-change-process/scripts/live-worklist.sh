#!/usr/bin/env bash
# Print every source in a provider's format doc whose live fetch differs from
# what the drift check would accept: a changed hash, a fetch error, content the
# check rejects, or a redirect to another host. Run from the capmon repo root
# after `go build -o capmon ./cmd/capmon`.
#
#   bash .agents/skills/capmon-change-process/scripts/live-worklist.sh <slug>
#
# Output, one row per source that needs work:
#   CHANGED  <content_type> <uri> stored=<hash or -> live=<hash>
#   ERROR    <content_type> <uri> <first line of the hash-url error>
#   INVALID  <content_type> <uri> type=<content type> bytes=<n>
#   REDIRECT <content_type> <uri> -> <final url>
set -euo pipefail
slug=${1:?usage: live-worklist.sh <slug>}
awk '/^  [a-z_]+:$/{ct=$1; sub(":","",ct)} /- uri:/{u=$3; gsub(/"/,"",u)}
     /fetch_method:/{fm=$2}
     /content_hash:/{h=$2; gsub(/"/,"",h); print ct "\t" u "\t" (h==""?"-":h) "\t" (fm==""?"-":fm); fm=""}' \
  docs/provider-formats/$slug.yaml |
while IFS=$'\t' read -r ct u h fm; do
  [ "$fm" = local_file ] && continue
  out=$(./capmon hash-url "$u" 2>&1) || true
  n=$(printf '%s\n' "$out" | awk '/^content_hash:/{print $2}')
  if [ -z "$n" ]; then echo "ERROR    $ct $u $(printf '%s\n' "$out" | head -1)"; continue; fi
  t=$(printf '%s\n' "$out" | awk '/^content_type:/{sub(";","",$2); print $2}')
  b=$(printf '%s\n' "$out" | awk '/^bytes:/{print $2}')
  f=$(printf '%s\n' "$out" | awk '/^final_url:/{print $2}')
  if [ "$b" = 0 ] || { [ "$t" = text/html ] && [ "$b" -lt 512 ]; } ||
     printf '%s' "$t" | grep -Eq '^(image|video|audio)/|^application/(octet-stream|zip|pdf|gzip|x-tar)$'; then
    echo "INVALID  $ct $u type=$t bytes=$b"
  elif [ "$(printf '%s' "$f" | cut -d/ -f3)" != "$(printf '%s' "$u" | cut -d/ -f3)" ]; then
    echo "REDIRECT $ct $u -> $f"
  elif [ "$n" != "$h" ]; then
    echo "CHANGED  $ct $u stored=$h live=$n"
  fi
done
