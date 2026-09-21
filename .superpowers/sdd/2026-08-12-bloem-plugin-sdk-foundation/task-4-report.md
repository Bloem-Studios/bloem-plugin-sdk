# Task 4 Report: Rebrand and Verify the Author Examples

## Status

Rebranded the author examples to Bloem while retaining the existing v1 runtime
and wire compatibility contract.

## RED Evidence

Added `examples/examples_test.go` with `TestExampleManifestIdentity`, which
loads both embedded-manifest source files through `manifest.Load` and requires
their Bloem plugin IDs plus `silo_api_version: v1`.

Command:

```text
go test ./examples -count=1
```

Result: exit code 1, with the intended legacy-identity failures:

```text
--- FAIL: TestExampleManifestIdentity (0.00s)
    --- FAIL: TestExampleManifestIdentity/hello-scheduled-task (0.00s)
        examples_test.go:30: plugin_id = "example.hello-task", want "bloem.example.hello-task"
    --- FAIL: TestExampleManifestIdentity/hello-runtime-host (0.00s)
        examples_test.go:30: plugin_id = "example.hello-runtime-host", want "bloem.example.runtime-host"
FAIL
FAIL    github.com/Bloem-Studios/bloem-plugin-sdk/examples    0.305s
FAIL
```

## GREEN Evidence

Ran the required no-workspace verification:

```text
GOWORK=off go test ./examples -count=1
ok      github.com/Bloem-Studios/bloem-plugin-sdk/examples    0.343s

GOWORK=off go build ./examples/hello-scheduled-task
# exit 0

GOWORK=off go build ./examples/hello-runtime-host
# exit 0
```

## Files Changed

- Added `examples/examples_test.go` to test both manifest identities through
  the public manifest loader and retain `silo_api_version: v1`.
- Updated `examples/hello-scheduled-task/manifest.json` with
  `bloem.example.hello-task` and Bloem-facing display text.
- Updated `examples/hello-runtime-host/manifest.json` with
  `bloem.example.runtime-host` and Bloem-facing display text.
- Updated `examples/hello-scheduled-task/README.md` for Bloem author-facing
  copy.
- Updated `examples/hello-runtime-host/main.go` so its logger identifies the
  Bloem example.

The Go module imports in both example binaries were already updated to
`github.com/Bloem-Studios/bloem-plugin-sdk` by the earlier identity task; this
task verified those imports remain in place.

## Self-Review

- `git diff --check` passed with no whitespace errors.
- Scanned the examples for stale `github.com/Silo-Server/silo-plugin-sdk`,
  legacy `example.*` plugin IDs, and `Install into Silo`; none remain.
- Confirmed both manifests still contain `silo_api_version: "v1"` and
  `scheduled_task.v1`, and both examples still import protobuf code from
  `pkg/pluginproto/silo/plugin/v1`.
- The two no-workspace builds produced root-level binaries; both generated
  files were sent to Trash after verification and are recoverable.

## Concerns

None.
