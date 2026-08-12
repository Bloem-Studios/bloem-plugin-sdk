# Private releases

Vondel Plugin SDK releases are GitHub releases inside the existing private
`Vondel-Media/vondel-plugin-sdk` repository. The release workflow never changes
repository visibility and does not publish the module to a public package
registry. It runs only when an operator explicitly pushes a `v*` tag, checks
out and tests that exact tag, and then creates the corresponding private GitHub
release.

## Create a private release

`v0.13.2` is an immutable failed historical candidate and must never be reused
or moved. `v0.13.3` is the first verified Vondel Plugin SDK release. For each
future release, set `RELEASE_TAG` to a new, unused semantic version, run the
complete local gate from a clean checkout, create an annotated tag, and push
only that reviewed tag:

```bash
set -euo pipefail
RELEASE_TAG=vX.Y.Z
if [[ ! "$RELEASE_TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "replace vX.Y.Z with a concrete, unused vMAJOR.MINOR.PATCH tag" >&2
  exit 1
fi

test -z "$(git status --porcelain)"
test -z "$(git tag --list "$RELEASE_TAG")"
remote_tags="$(git ls-remote --tags origin "refs/tags/$RELEASE_TAG" "refs/tags/$RELEASE_TAG^{}")"
test -z "$remote_tags"
test "$(gh api repos/Vondel-Media/vondel-plugin-sdk --jq '.visibility')" = private
if release_lookup="$(gh api --include "repos/Vondel-Media/vondel-plugin-sdk/releases/tags/$RELEASE_TAG" 2>&1)"; then
  echo "release $RELEASE_TAG already exists" >&2
  exit 1
elif [[ "$release_lookup" != *"HTTP 404"* ]]; then
  printf 'unable to prove release tag is unused:\n%s\n' "$release_lookup" >&2
  exit 1
fi
GOWORK=off go test ./...
./scripts/verify-private-release.sh
git tag -a "$RELEASE_TAG" -m "Vondel Plugin SDK $RELEASE_TAG"
git push origin "$RELEASE_TAG"
gh release view "$RELEASE_TAG" --repo Vondel-Media/vondel-plugin-sdk
gh api repos/Vondel-Media/vondel-plugin-sdk --jq '.visibility'
```

The clean-worktree check and all three uniqueness checks must succeed before
tagging. After the workflow finishes, the final command must print `private`.

## Roll back a release

Delete the private GitHub release and its tag only if no downstream repository
has pinned that tag. Delete the remote tag and then the local tag after removing
the release. If any downstream repository has already pinned it, preserve the
published tag and issue a new patch tag with the correction instead.
