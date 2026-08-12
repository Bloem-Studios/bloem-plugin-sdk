#!/bin/sh
set -eu

fail() {
  printf 'private release guard failed: %s\n' "$1" >&2
  exit 1
}

test "$(sed -n '1s/^module //p' go.mod)" = \
  "github.com/Vondel-Media/vondel-plugin-sdk" || fail "unexpected module"

if grep -Eq '^[[:space:]]*replace[[:space:]]|^replace[[:space:]]*\(' go.mod; then
  fail "go.mod contains a replace directive"
fi

if rg -n 'gh repo edit.*visibility|npm publish|docker push|pkg\.go\.dev' .github; then
  fail "workflow contains a public publication path"
fi

if rg -n '/Users/|/home/[^/]+/|https://[^/@]+:[^/@]+@github\.com' \
  --glob '!scripts/verify-private-release.sh' .; then
  fail "repository contains a local path or credential-bearing URL"
fi

printf '%s\n' 'private release guard passed'
