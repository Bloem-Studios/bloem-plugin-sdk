#!/bin/sh
set -eu

guard=./scripts/verify-private-release.sh
workflow=.github/workflows/release.yml

fail() {
  printf 'private release test failed: %s\n' "$1" >&2
  exit 1
}

expect_guard_failure() {
  expected=$1
  test_path=$2

  if output=$(PATH="$test_path" "$guard" 2>&1); then
    fail "guard unexpectedly passed: $expected"
  fi

  case "$output" in
    *"$expected"*) ;;
    *) fail "guard did not report: $expected" ;;
  esac
}

make_scan_path() {
  destination=$1
  mkdir -p "$destination"
  ln -s "$(command -v sed)" "$destination/sed"
  ln -s "$(command -v grep)" "$destination/grep"
}

test_workflow_sha_pin() {
  grep -Fq 'ref: ${{ github.sha }}' "$workflow" ||
    fail "release checkout is not pinned to the event SHA"
  grep -Fq 'git ls-remote origin' "$workflow" ||
    fail "release does not resolve the remote tag"
  grep -Fq 'test "$remote_sha" = "$EXPECTED_SHA"' "$workflow" ||
    fail "release does not compare the remote tag with the event SHA"
}

test_missing_rg() {
  missing_path=$test_root/missing-rg
  make_scan_path "$missing_path"
  expect_guard_failure "required command not found: rg" "$missing_path"
}

test_rg_error() {
  error_path=$test_root/rg-error
  make_scan_path "$error_path"
  printf '%s\n' '#!/bin/sh' 'exit 2' > "$error_path/rg"
  chmod +x "$error_path/rg"
  expect_guard_failure "rg search failed while checking workflows" "$error_path"
}

test_clean_guard() {
  "$guard" >/dev/null
}

test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

case ${1:-all} in
  workflow) test_workflow_sha_pin ;;
  missing-rg) test_missing_rg ;;
  rg-error) test_rg_error ;;
  all)
    test_workflow_sha_pin
    test_missing_rg
    test_rg_error
    test_clean_guard
    ;;
  *) fail "unknown test case: $1" ;;
esac

printf '%s\n' 'private release tests passed'
