# Private releases

Vondel Plugin SDK releases are GitHub releases inside the existing private
`Vondel-Media/vondel-plugin-sdk` repository. The release workflow never changes
repository visibility and does not publish the module to a public package
registry. It runs only when an operator explicitly pushes a `v*` tag, checks
out and tests that exact tag, and then creates the corresponding private GitHub
release.

## Create a private release

Run the complete local gate from a clean checkout, create an annotated tag, and
push only that reviewed tag:

```bash
git status --porcelain
GOWORK=off go test ./...
./scripts/verify-private-release.sh
git tag -a v0.13.2 -m "Vondel Plugin SDK v0.13.2"
git push origin v0.13.2
gh release view v0.13.2 --repo Vondel-Media/vondel-plugin-sdk
gh api repos/Vondel-Media/vondel-plugin-sdk --jq '.visibility'
```

The first command must print nothing. After the workflow finishes, the last
command must print `private`.

## Roll back a release

Delete the private GitHub release and its tag only if no downstream repository
has pinned that tag. Delete the remote tag and then the local tag after removing
the release. If any downstream repository has already pinned it, preserve the
published tag and issue a new patch tag with the correction instead.
