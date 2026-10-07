# Native storage providers

Bloem SDK v0.24.0 adds the independent `bloem.plugin.v1.StorageProvider`
service. It preserves the existing `silo.plugin.v1` contract, handshake and
`CapabilityServers` layout. The SDK repository is public; "private storage"
means a separate host-approved extension, not a private module distribution.

## Create a provider with the published SDK

Start a separate Go module and pin the public release:

```sh
go mod init example.com/my-storage-provider
go get github.com/Bloem-Studios/bloem-plugin-sdk@v0.24.0
```

Implement `storagev1.StorageProviderServer` from
`github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/bloem/plugin/v1`.
The four methods are `Describe`, `List`, `Stat` and `Read`; their obligations
are [below](#wire-contract). Generated bindings ship in the module, so plugin
authors do not need to generate protobuf code or check out Bloem Server.

Embed a normal SDK manifest, including your own stable plugin ID, version,
`"checksum": "__CHECKSUM__"`, `"silo_api_version": "v1"` and the binary's
`supported_platforms`. A storage-only provider uses `"capabilities": []`.
Storage adds no `capability.KnownTypes` entry: do not invent
`storage_provider.v1`. Register the provider on the existing connection:

```go
import sdkruntime "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginsdk/runtime"

sdkruntime.ServeManifestWithOptions(manifestJSON, version,
    sdkruntime.CapabilityServers{},
    sdkruntime.WithStorageProvider(provider),
    sdkruntime.WithConfigure(provider.configure))
```

`manifestJSON` is the embedded manifest, `version` is your plugin version,
and `provider` implements all four storage RPCs and the callback below.
The SDK option can coexist with ordinary services on the same connection.
Bloem's current native installation path, however, accepts storage-only
artifacts with no public capabilities, HTTP routes or packaged assets.

### Receive backend configuration

Declare connection settings under `global_config_schema`. For an HTTP backend,
this is a manifest fragment with a typed URL and secret API key:

```json
{
  "global_config_schema": [
    {
      "key": "connection",
      "title": "Storage connection",
      "json_schema": "{\"type\":\"object\",\"properties\":{\"base_url\":{\"type\":\"string\"},\"api_key\":{\"type\":\"string\"}},\"additionalProperties\":false}",
      "admin_form": {
        "fields": [
          {"key": "base_url", "label": "Base URL", "control": "ADMIN_FORM_CONTROL_TEXT"},
          {"key": "api_key", "label": "API key", "control": "ADMIN_FORM_CONTROL_PASSWORD", "secret": true}
        ]
      }
    }
  ]
}
```

Adapt those fields to your backend; for example, an S3 adapter needs its bucket,
region/endpoint and chosen credential settings. `WithConfigure` accepts exactly
`func(context.Context, []*pluginv1.ConfigEntry) error`, where `pluginv1` is the
public `silo/plugin/v1` package. Each entry's `value` is a protobuf Struct.
Add configuration state and a callback to your provider implementation:

```go
import (
    "context"
    "encoding/json"
    "sync/atomic"

    pluginv1 "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

type connectionSettings struct {
    BaseURL string `json:"base_url"`
    APIKey  string `json:"api_key"`
}

type provider struct {
    settings atomic.Pointer[connectionSettings]
    // The four storage RPC implementations use this configuration snapshot.
}

func (p *provider) configure(_ context.Context, entries []*pluginv1.ConfigEntry) error {
    next := &connectionSettings{}
    for _, entry := range entries {
        if entry.GetKey() != "connection" {
            continue
        }
        data, err := json.Marshal(entry.GetValue().AsMap())
        if err != nil || json.Unmarshal(data, next) != nil {
            return status.Error(codes.InvalidArgument, "invalid connection settings")
        }
    }
    p.settings.Store(next)
    return nil
}
```

This excerpt supplies configuration handling, not the four RPC implementations.
Each RPC loads a settings snapshot and passes its context to backend requests.
Accept empty or incomplete settings in `Configure`; parse and store them without
network I/O because startup has a short control timeout. Report an unconfigured
backend from storage RPCs as `FailedPrecondition`, without exposing credentials
or pretending the source is absent. Storage has no `TestConnection` RPC.
Do not log settings: secret values arrive in plaintext inside the plugin even
though the host stores native configuration encrypted and omits it from responses.

The native host supplies authorized configuration before storage calls, a small
explicit process environment, and no general RuntimeHost broker. Do not depend
on inherited AWS environment credentials or `runtime.Host()` callbacks.
Configuration replacement uses the native route below and advances the host's
configuration generation; ordinary plugin configuration routes do not apply.

### Map a real backend

An S3 provider brings its own AWS client dependency and maps object identities,
listing and byte reads onto the four storage RPCs. Prove that the selected
backend can list every key, including a key that also prefixes descendants, and
serve bytes pinned to an immutable version. An ETag or a metadata check followed
by an unpinned download is not by itself that guarantee. Advertise
`revision_pinned_reads` only when the implementation can honor it; the current
host consumer refuses providers that report false. The SDK does not certify an
S3 endpoint or implement general media playback in the host.

[Bookwarehouse](https://github.com/RXWatcher/bookwarehouse) can be adapted through
its `GET /api/v1/books`, `GET /api/v1/books/{id}` and
`GET /api/v1/books/{id}/download` endpoints using the `X-API-Key` header.
Stable book IDs and metadata fields `file_hash`, `file_size` and `file_format`
can inform entry mapping. At the inspected Bookwarehouse revision
[`7deaffa`](https://github.com/RXWatcher/bookwarehouse/tree/7deaffa0601099b3de874dfb9400b7a532f94b3a),
[the download handler](https://github.com/RXWatcher/bookwarehouse/blob/7deaffa0601099b3de874dfb9400b7a532f94b3a/internal/api/handlers/books.go#L719)
streams the current storage object with status 200; it does not enforce Range,
If-Match or a requested revision. Its
[S3 read](https://github.com/RXWatcher/bookwarehouse/blob/7deaffa0601099b3de874dfb9400b7a532f94b3a/internal/storage/s3.go#L390)
selects bucket and key without a version or condition. A direct streaming wrapper
therefore cannot claim pinned reads merely because `file_hash` is present.
It needs an upstream immutable/conditional byte API or a separately designed,
bounded, verified immutable snapshot adapter before advertising that guarantee.
The SDK ships neither adapter. `ebook_backend.v1` is only a public constant;
there is no ebook-backend service definition to implement in this release.

## Package, approval and native installation

Build the binary for the host's exact OS/architecture. Keep the source manifest
embedded with its checksum placeholder; extract the final manifest from that
exact executable on a machine that can run it:

```sh
mkdir -p dist
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.version=0.1.0" -o dist/plugin .
# Run on a compatible Linux/amd64 machine:
./dist/plugin manifest > dist/manifest.json
```

The host maintainer must approve an immutable tuple containing the manifest,
its exact lowercase SHA-256 executable checksum, OS and architecture under an
opaque `artifact_key`. The checksum must match both the binary and manifest;
the manifest must support that platform, use API `v1`, avoid the reserved
`silo.builtin` identity, and declare no public capabilities, HTTP routes or
assets. The host checks the executable and its embedded manifest again at
runtime. Neither the manifest nor an installation request can approve itself;
there is no author-facing approval endpoint.

The native management upload contains the **raw executable**, not an ordinary
plugin ZIP. Ordinary catalog install, `/api/v1/admin/plugins/uploads`, ordinary
configuration and auto-update are separate paths and do not admit native storage.
The current host contract is documented in the server's
[native storage onboarding guide](https://github.com/Bloem-Studios/bloem-server/blob/ac00c6115a55c021c11f96ece01722f7c5745f7f/docs/architecture/bloem-native-storage-onboarding.md)
and [API reference](https://github.com/Bloem-Studios/bloem-server/blob/ac00c6115a55c021c11f96ece01722f7c5745f7f/docs/bloem-api-reference.md#native-storage-administration).

### Configure the host approval file

Bloem Server reads `BLOEM_NATIVE_STORAGE_APPROVALS` at startup. It names a JSON
file whose top-level keys are artifact keys and whose records contain the full
`manifest` object plus `checksum`, `os` and `arch`. There is no source-code change
or database edit required to supply this map. For the reviewed build above,
`jq` can produce the record without retyping the manifest or its checksum:

```sh
jq -n --slurpfile manifest dist/manifest.json \
  '{"approved-storage-release": {"manifest": $manifest[0], "checksum": $manifest[0].checksum, "os": "linux", "arch": "amd64"}}' \
  > dist/native-storage-approvals.json
```

The host operator reviews the artifact and verifies the executable's SHA-256
against this record before placing it on the server. This file is the authority
to approve executable code; generating it in a plugin build is not approval.
The startup loader requires:

- An absolute, clean path with no symlinks in any component.
- A regular file of at most 1 MiB, owned by root or the server's effective UID,
  readable by the server, with no write bits and no group/other permissions
  (normally mode `0400`).
- Parent directories owned by root or the server's effective UID and not
  group/other-writable, except a root-owned sticky directory.

For example, the operator can provision the protected file at
`/etc/bloem/native-storage-approvals.json`, set
`BLOEM_NATIVE_STORAGE_APPROVALS=/etc/bloem/native-storage-approvals.json` in the
server's service/container environment, then restart the server. The approval
map is immutable for that process; updates need a reviewed file and restart.
An unset variable produces an empty map. An explicitly invalid file or invalid
artifact record fails startup. In containers, these path and ownership checks
apply inside the container. See the server's
[startup loader](https://github.com/Bloem-Studios/bloem-server/blob/ac00c6115a55c021c11f96ece01722f7c5745f7f/internal/nativestorage/host.go).

### Inspect and install an approved artifact

Use the signed context returned by `POST /api/bloem/v1/admin/session` for one of
these prefixes:

- `/api/bloem/v1/admin/platform/native-storage`
- `/api/bloem/v1/admin/organization/native-storage`

The session must carry current scope and resource authority; an ordinary viewer
token does not substitute for it. Read `GET /capabilities` to inspect actual
host support, then `GET /artifacts` to select an approved `artifact_key` for the
binary you built. Both paths are relative to your scope prefix. An empty artifact list means there is no selectable approval.
An approval and mounted routes do not establish verified backend support.

`POST /installations` requires exactly two multipart parts: `request` containing
JSON and `binary` containing the executable. A new-source request has this shape;
replace the artifact/source/root IDs with the approved artifact and the stable
IDs your provider exposes through `Describe`:

```json
{
  "artifact_key": "approved-storage-release",
  "provider_source_id": "books",
  "root_entry_id": "root",
  "enabled": true,
  "config": {
    "connection": {
      "base_url": "https://books.example",
      "api_key": "example-only"
    }
  }
}
```

`artifact_key`, `provider_source_id`, `root_entry_id`, `enabled` and `config`
are required and non-null. `config` maps configuration keys to JSON objects;
`{}` is valid, but a null configuration or null entry is not. Put real credentials
only in the authorized request, never in the committed manifest or release files.
With that JSON saved as `install-request.json` and the signed administrative
context in `BLOEM_NATIVE_ADMIN_TOKEN`, an organization-scoped upload is:

```sh
curl --fail-with-body \
  "https://bloem.example/api/bloem/v1/admin/organization/native-storage/installations" \
  -H "Authorization: Bearer $BLOEM_NATIVE_ADMIN_TOKEN" \
  -F 'request=@install-request.json;type=application/json' \
  -F 'binary=@dist/plugin;type=application/octet-stream'
```

The `request` part is limited to 1 MiB, the nonempty binary to 256 MiB and the
complete multipart body to 258 MiB. Unknown or duplicate JSON fields, trailing
JSON, extra multipart parts and transfer-encoded parts are rejected. A new
source omits `source_key` and `expected_revision`. Supplying a retained
`source_key` requires its positive `expected_revision`, verified lineage and an
empty retained namespace; this is not general reattachment support. Platform
requests may supply `organization_id`; organization requests must omit that
field, including null. A successful upload returns `201 {source: ...}` and
creates neither a library nor a scan.

### Replace source configuration

Read `GET /sources/{source_key}` for the current `configuration_revision`, then
send `PUT /sources/{source_key}/configuration` with `Content-Type: application/json`
and a body such as:

```json
{
  "expected_revision": 1,
  "config": {
    "connection": {
      "base_url": "https://books.example",
      "api_key": "example-replacement"
    }
  }
}
```

Use the actual current revision in place of `1`. This replaces the whole config,
not selected fields, and is bounded to 1 MiB. Success returns
`{source_key, configuration_revision}`. Any binding, discovered entry or retained
reference blocks replacement with `409 configuration_namespace_unverified`.
A stale revision returns `409 revision_conflict`; reload before retrying.
A `503 mutation_outcome_unknown` requires reconciliation through source reads
before another mutation. Native mutations have no blanket automatic replay rule.

Library creation, initialization, binding and full scans are separate authorized
steps in the server's onboarding guide. Provider IDs grant no access by themselves;
tenant authority, source ownership, library/profile access, generations and
cancellation remain host responsibilities. Process/credential separation is not
an OS sandbox.

## Wire contract

The source is [`storage_provider.proto`](../proto/bloem/plugin/v1/storage_provider.proto).
The current server owns byte-identical protocol source and its own generated
bindings, so a standalone host build does not require this SDK checkout.

| RPC | Provider obligation |
|---|---|
| `Describe` | Return protocol revision `1`, stable source IDs and each source's stable root entry ID. Advertise `revision_pinned_reads` only when the backend can honor immutable version reads. |
| `List` | Enumerate one directory with bounded pages, stable entry identity and advancing opaque cursors. Honor `max_entries`; never exceed 512 entries or 1 MiB encoded entry data per page. A terminal page sets `complete` and has no next cursor. |
| `Stat` | Resolve the exact source/entry at `expected_revision`; distinguish authoritative absence from transport or permission failures. |
| `Read` | Deliver exactly the requested range from `expected_revision`, using consecutive absolute offsets and chunks of at most 128 KiB. Set `eof` on the final chunk and finish with OK only on complete success. |

`Entry.id` is stable identity; `revision` is an opaque content version. Treat
source IDs, entry IDs and cursors as opaque. `logical_path` is display metadata,
never a host filesystem path. Object keys that coexist with descendants must
remain individually discoverable; directory presentation must not hide a file
with the same prefix. Pagination completion alone does not establish an
immutable directory snapshot or authorize absence/deletion reconciliation.

A revision conflict, error after sending the last data chunk, truncated stream,
duplicate chunk or cancelled call is a failed read. Byte count alone does not
establish success: the final gRPC status must be OK. Propagate cancellation and
deadlines to backend work. Bound allocations and backend requests; the synthetic
fixture caps individual reads at 8 MiB, matching the host's bounded read adapter.

| gRPC status | Meaning |
|---|---|
| `NotFound` | Authoritative source/entry absence. |
| `Unavailable` | Provider/backend failure; never absence. |
| `PermissionDenied` | Backend denied access. |
| `FailedPrecondition` | The pinned revision changed or cannot be served. |
| `InvalidArgument` | Invalid identity, cursor or range. |
| `Canceled` / `DeadlineExceeded` | Caller cancellation or deadline. |

## Validation and production scope

[`hello-storage`](../examples/hello-storage/README.md) is an executable,
synthetic conformance fixture with an EPUB, bounded virtual discovery and
intentional fault entries. It accesses no real storage backend and is not a
production plugin. SDK tests cover service registration, compatibility and
fixture behavior without a database.

A released SDK establishes authoring/distribution availability, not backend
admission. Bloem Server still requires evidence of immutable version reads and
complete ordered listing for each production backend. Synthetic discovery is
not measured production throughput. Reader support currently targets authorized
EPUB/PDF reads; generic downloads, proxy delivery, Jellyfin attachments,
conversion and offline integrity are separate host integration contracts.
Cross-node cancellation, authoritative absence handling and recurring scan
retention remain server acceptance gates. See the server's
`docs/architecture/bloem-native-storage.md` for current host scope.

Bloem promotions and ambience workers are unrelated to this gRPC extension.
They use a bundled server-owned JSON process protocol and have no SDK capability.
