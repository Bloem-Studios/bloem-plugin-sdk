#!/bin/sh
set -eu

fail() {
  printf 'private release guard failed: %s\n' "$1" >&2
  exit 1
}

scan_forbidden() {
  match_message=$1
  error_context=$2
  shift 2

  if "$@"; then
    fail "$match_message"
  else
    scan_status=$?
    test "$scan_status" -eq 1 ||
      fail "rg search failed while $error_context (exit $scan_status)"
  fi
}

command -v rg >/dev/null 2>&1 || fail "required command not found: rg"

test "$(sed -n '1s/^module //p' go.mod)" = \
  "github.com/Bloem-Studios/bloem-plugin-sdk" || fail "unexpected module"

if grep -Eq '^[[:space:]]*replace[[:space:]]|^replace[[:space:]]*\(' go.mod; then
  fail "go.mod contains a replace directive"
fi

scan_forbidden \
  "workflow contains a public publication path" \
  "checking workflows" \
  rg -n 'gh repo edit.*visibility|npm publish|docker push|pkg\.go\.dev' .github

scan_forbidden \
  "repository contains a local path or credential-bearing URL" \
  "checking local paths and credential-bearing URLs" \
  rg -n '/Users/|/home/[^/]+/|https://[^/@]+:[^/@]+@github\.com' \
    --glob '!scripts/verify-private-release.sh' .

printf '%s\n' 'private release guard passed'
