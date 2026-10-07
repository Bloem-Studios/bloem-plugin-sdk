# Native storage providers

Bloem SDK v0.24.0 adds the independent `bloem.plugin.v1.StorageProvider`
service. It preserves the existing `silo.plugin.v1` contract, handshake and
`CapabilityServers` layout. The SDK repository is public; "private storage"
means a separate host-approved extension, not a private module distribution.

## Authoring and admission

Import `pkg/pluginproto/bloem/plugin/v1` and register your implementation with
`runtime.WithStorageProvider`:

```go
runtime.ServeManifestWithOptions(manifestJSON, version,
    runtime.CapabilityServers{},
    runtime.WithStorageProvider(provider))
```

The normal runtime manifest and checksum remain required. Storage adds no
`capability.KnownTypes` entry: do not invent `storage_provider.v1` in the public
manifest. An empty capability list is valid for a storage-only provider.
Ordinary services can coexist on the same connection, and the option composes
with watch-device authorization and the newer auth extension services.

The host's native installation registry selects an immutable approved artifact
and verifies its manifest, exact checksum, supported platform and identity.
A package cannot approve itself. Ordinary catalog uploads, auto-update and
ordinary plugin configuration routes are not the native admission mechanism.
The host supplies authorized source configuration through `Runtime.Configure`;
a provider can use `WithConfigure` or its own Runtime implementation.

The native host process manager supplies a small explicit environment and no
general RuntimeHost broker. Do not depend on `runtime.Host()` callbacks. This
is process/credential separation, not an OS sandbox. Tenant authority, source
ownership, library/profile access, configuration generations and cancellation
remain host responsibilities; provider identifiers grant no access by themselves.

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
