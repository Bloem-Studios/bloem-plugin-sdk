# Bloem Plugin SDK

The private Go library for writing Bloem server plugins. **Not a runtime plugin** — this is
a library that plugin authors depend on via `go.mod`. It gives a plugin author the generated
protobuf and gRPC code for every capability a Bloem server understands, helpers to load and
validate a `manifest.json`, a `Serve` function that turns a Go program into a process the server
can launch and talk to, and a typed client for calling back into the server. Plugins built with
this SDK target Bloem servers and compatible upstream Silo servers through the same v1 wire
contract. It is for plugin authors, and for the maintainer who keeps the contract stable and
cuts releases.

This repository is Bloem's source of truth for the plugin authoring contract. Bloem hosts and
plugins pin tagged semver releases. Local multi-repo workspaces may use `go.work` or a temporary
`replace`, but CI and release builds resolve the SDK from a published module tag.

A few identifiers keep spellings from the project's origins because servers and plugins already
speak them: the Go module path `github.com/Vondel-Media/vondel-plugin-sdk`, the protobuf package
`silo.plugin.v1`, the manifest field `silo_api_version` and the handshake cookie
`SILO_PLUGIN=silo-rpc-plugin-v1`. They are pinned by guard tests and must be typed exactly as shown.

## Features

**Authoring**

- `ServeManifest` (short path): embed `manifest.json`, pass a version and your capability servers, and the SDK loads, validates, stamps the checksum and serves; `Serve` (long path) for full control.
- A `manifest` subcommand on every plugin binary that prints the manifest with the real checksum and exits, so a server can introspect a plugin without launching it.
- Self-describing binaries: the SHA-256 of the running executable is computed at start-up and written into the manifest, so a package is installable without external repository state.
- `runtimedefault`: an embeddable `Runtime` server with `BindHostBroker` already wired.
- New gRPC services are added through `ServeManifestOption` values (for example `WithWatchSyncDeviceAuthorization`), never by changing the `CapabilityServers` shape, so plugins written for v0.12 keep compiling.
- Two complete example plugins (`hello-scheduled-task`, `hello-runtime-host`) and a compat probe that proves a server can launch an SDK-built binary.

**Manifest and settings**

- Manifest loading, validation and checksum stamping (`manifest` package); unknown JSON keys are ignored on load.
- Presentation block rules for catalog-ready plugins and `ValidateCatalogPresentation` for catalog tooling.
- HTTP route and asset registration for plugins that serve pages.
- Settings declared as JSON Schema (draft 2020-12) in `config_schema`, validated by the `config` package before the server accepts a value; `secret` fields are stored encrypted by the server.
- `convert` helpers between capability descriptors and plain Go maps for hosts that store capabilities in a database.

**Capabilities (thirteen known types)**

- Metadata: `metadata_provider.v1` (search, details, seasons, episodes, images, people, image URL resolution) and `image_resolver.v1`.
- Playback markers: `marker_provider.v1` (external intro/credits/recap/preview segments) and `media_analyzer.v1` (local file analysis).
- Host integration: `scheduled_task.v1`, `event_consumer.v1`, `http_routes.v1` (with per-route `access` levels enforced by the server), `auth_provider.v1` (password and OAuth/OIDC login).
- Media pipeline: `request_router.v1`, `scan_source.v1` (Autoscan change sources), `watch_sync_provider.v1` (external watch-history sync with device-code authorization).
- `audiobook_backend.v1` and `ebook_backend.v1` as constants only; no service definition ships in this SDK. There is no subtitle capability.

**Calling back into the server (`runtimehost`)**

- Events: `PublishEvent`, `PublishEventTo`, `PublishEventToInstallation`; the server prefixes every name with `plugin.<plugin_id>.` so a plugin cannot forge a core event.
- Host and catalog reads: `GetHostInfo`, `ListLibraries`, `CheckMediaPresence`, `ListLibraryMedia`, `GetCatalogStats`, `ResolveCatalogImageURLs` — public-safe rows only.
- Peer discovery and plugin-to-plugin HTTP: `ListInstalledPlugins`, `CallPluginHTTP`, with `ListInstalledPluginsByCapability` and `CallPluginJSON` helpers.
- `MintScopedStream` for short-lived, narrowly scoped stream grants, and `SetGlobalConfigEntry` for plugin-owned configuration.
- `runtime.Host()` caches one client on the single broker stream the server listens on.

**Contract stability and release engineering**

- Guard tests lock the wire contract, proto sources, module path, `NOTICE` attribution, example identities and the `CapabilityServers` field order; a failing guard means revert, not "fix the test".
- `buf` lint and breaking-change rules on the proto module; generated code is committed so authors never run the generator.
- Semver policy: additive API, proto field, capability family or `RuntimeHost` RPC is a minor; compatible fix or docs is a patch; any breaking change to `v1` is a major.
- Private-release guard (`scripts/verify-private-release.sh`) runs in CI and at release: module path, no `replace`, no public publication path in workflows, no absolute home-directory paths or credential-bearing URLs anywhere in the repository, documentation included.
- Release workflow on `v*` tags re-tests the exact SHA, refuses a moved tag, and creates a private GitHub Release; repository visibility never changes and nothing goes to a public registry.
- `httpclient`: a small outbound JSON client for plugins that talk to a third-party API with an `X-Api-Key` header.

## Quick start

Your first plugin in five steps (Go 1.26, Git, and a Bloem server you may install plugins on;
the full walkthrough is in the [User Guide](docs/user-guide.md#2-your-first-plugin)).

1. **Create the module and add the SDK.** The repository is private, so your Git credentials must
   be able to read it; set `GOPRIVATE=github.com/Vondel-Media` if Go tries the public proxy.

   ```sh
   mkdir hello-plugin && cd hello-plugin
   go mod init example.com/hello-plugin
   go get github.com/Vondel-Media/vondel-plugin-sdk@v0.13.3
   ```

2. **Write `manifest.json`** with `plugin_id`, `version`, `"checksum": "__CHECKSUM__"`,
   `"silo_api_version": "v1"`, `supported_platforms` (required by the server) and one capability,
   for example `scheduled_task.v1`.

3. **Write `main.go`**: embed the manifest, implement the capability server, and call
   `sdkruntime.ServeManifest(manifestJSON, version, sdkruntime.CapabilityServers{ScheduledTask: helloTask{}})`.
   `ServeManifest` loads and validates the manifest, sets the version, stamps the checksum, serves
   a default `Runtime` service alongside your servers, and never returns.

4. **Build and inspect.**

   ```sh
   GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
     -ldflags "-s -w -X main.version=0.1.0" -o plugin .
   ./plugin manifest        # prints the manifest with the real checksum
   ```

5. **Package and install.** The package is a zip with `manifest.json` and an executable named
   exactly `plugin` at the root; upload it to the server's admin API.

   ```sh
   ./plugin manifest > manifest.json
   zip plugin.zip manifest.json plugin
   curl -X POST "https://bloem.example/api/v1/admin/plugins/uploads" \
     -H "Authorization: Bearer $BLOEM_ADMIN_TOKEN" \
     -F archive=@plugin.zip
   ```

   The server verifies the checksum, unpacks the package, starts the process and calls
   `Configure`; the plugin appears in **Admin → Plugins**.

## Packages

- `github.com/Vondel-Media/vondel-plugin-sdk/pkg/pluginproto/silo/plugin/v1` — generated protobuf code.
- `github.com/Vondel-Media/vondel-plugin-sdk/pkg/pluginsdk/capability` — stable capability type constants for manifests and peer discovery.
- `github.com/Vondel-Media/vondel-plugin-sdk/pkg/pluginsdk/config` — config-schema helpers.
- `github.com/Vondel-Media/vondel-plugin-sdk/pkg/pluginsdk/convert` — type conversions.
- `github.com/Vondel-Media/vondel-plugin-sdk/pkg/pluginsdk/manifest` — manifest loading/rendering.
- `github.com/Vondel-Media/vondel-plugin-sdk/pkg/pluginsdk/runtime` — `manifest` subcommand + `Runtime` server scaffolding.
- `github.com/Vondel-Media/vondel-plugin-sdk/pkg/pluginsdk/runtimedefault` — default `Runtime` implementation with `BindHostBroker` already wired; embed it to skip boilerplate.
- `github.com/Vondel-Media/vondel-plugin-sdk/pkg/pluginsdk/runtimehost` — typed client for the host's `RuntimeHost` service, including event publishing, host info, catalog browsing, installed-plugin discovery, scoped streams, plugin-to-plugin HTTP calls, and plugin-owned config writes.
- `github.com/Vondel-Media/vondel-plugin-sdk/pkg/pluginsdk/httpclient` — small outbound JSON HTTP client for plugins that talk to a third-party API with an `X-Api-Key` header.

## Capability families

The SDK ships protobuf contracts for every capability the host understands:

- `metadata_provider.v1`
- `image_resolver.v1`
- `marker_provider.v1`
- `media_analyzer.v1`
- `scheduled_task.v1`
- `event_consumer.v1`
- `auth_provider.v1`
- `http_routes.v1`
- `request_router.v1`
- `scan_source.v1`
- `watch_sync_provider.v1`
- `audiobook_backend.v1` (constant only; no service definition ships in this SDK)
- `ebook_backend.v1` (constant only; no service definition ships in this SDK)

Plugins implement one or more, advertise them in `manifest.json`, and serve them over gRPC. A manifest that names any other type fails validation with `unknown type`.

## Author workflow

A typical plugin:

1. Defines a `manifest.json` using the protobuf-derived schema.
2. Exposes a `Runtime` gRPC server plus one or more capability servers.
3. Supports the `manifest` subcommand via `pkg/pluginsdk/runtime` so the host can introspect manifests without launching the plugin.
4. Is installed either from a catalog or by uploading a trusted package to a Bloem server.

For a minimal self-describing plugin, see [`examples/hello-scheduled-task`](examples/hello-scheduled-task). For a plugin that calls back into the host via `RuntimeHost` (publishing events, listing libraries), see [`examples/hello-runtime-host`](examples/hello-runtime-host).

## Operator-facing presentation

`PluginManifest.presentation` gives the Bloem admin UI typed, plugin-level copy
and canonical links. It is optional for backward compatibility, but cataloged
plugins should provide a complete block:

```json
{
  "presentation": {
    "display_name": "Example Plugin",
    "summary": "A one-sentence explanation for a homelab administrator.",
    "description_markdown": "A longer description of what the plugin does and when to use it.",
    "setup_markdown": "1. Install the plugin.\n2. Add the required connection.\n3. Enable it for the relevant library.",
    "homepage_url": "https://example.com/plugin",
    "source_url": "https://github.com/example-org/example-plugin",
    "support_url": "https://github.com/example-org/example-plugin/issues",
    "changelog_url": "https://github.com/example-org/example-plugin/releases",
    "publisher_name": "Example Org",
    "publisher_url": "https://github.com/example-org",
    "license_spdx": "AGPL-3.0-or-later"
  }
}
```

- `display_name` is limited to 120 characters and `summary` to 240 characters;
  both are concise card copy without leading or trailing whitespace.
- `description_markdown` and `setup_markdown` use CommonMark-style Markdown;
  raw HTML is not part of the contract, and each field is limited to 32 KiB.
- All URLs must be absolute `http` or `https` links and must not contain
  embedded credentials; each URL is limited to 2048 bytes.
- Publisher and source fields are self-declared identity information. Catalog
  provenance and approval are assigned by the host/catalog, never by this block.
  `publisher_name` is limited to 120 characters.
- Use an SPDX license expression of at most 120 characters. Use `NOASSERTION`
  when the repository has not declared a license rather than guessing one.

Curated catalog tooling should call
`manifest.ValidateCatalogPresentation(manifest, canonicalRepositoryURL)` to
require the complete block and prevent a published `source_url` from drifting
away from the repository that produced the release.

## Calling back into the host

Plugins talk to the host through the `RuntimeHost` service, accessed via `pkg/pluginsdk/runtimehost.Client`. The host invokes `Runtime.BindHostBroker` on startup so plugins can dial back over the shared broker; `runtimedefault` handles that step for you. Available RPCs:

- `PublishEvent` / `PublishEventTo` / `PublishEventToInstallation` — fire events into the host's bus, broadcast, addressed to a stable `plugin_id`, or addressed to one specific installation.
- `GetHostInfo` — read host URL metadata for callback URLs and external-facing plugin links.
- `ListLibraries` — enumerate libraries the operator has configured.
- `CheckMediaPresence` — ask whether a given external id is already in the catalog.
- `ListInstalledPlugins` — discover sibling plugins (e.g. routers a request plugin can target).
- `ListLibraryMedia` / `GetCatalogStats` — read public-safe catalog rows and aggregate counts.
- `ResolveCatalogImageURLs` — resolve stored poster/backdrop image paths into host-generated browser URLs.
- `MintScopedStream` — create short-lived, narrowly scoped stream grants for guest/public workflows.
- `CallPluginHTTP` — invoke another installed plugin's `http_routes.v1` handler through the host control plane.
- `SetGlobalConfigEntry` — persist plugin-owned config that admins didn't set via the manifest form.

For plugin-to-plugin JSON calls, prefer the helper layer:

```go
plugins, err := host.ListInstalledPluginsByCapability(ctx, capability.RequestRouter)
if err != nil || len(plugins) == 0 {
    return err
}

var out struct {
    Accepted bool `json:"accepted"`
}
err = host.CallPluginJSON(ctx, runtimehost.CallPluginJSONRequest{
    InstallationID: int(plugins[0].GetInstallationId()),
    Path:           "/api/request",
    Request:        map[string]any{"title": "The Matrix"},
    Response:       &out,
})
```

The `auth_provider.v1` capability also exposes OAuth-flow RPCs (`InitAuthorize`, `ExchangeCode`, `RefreshSession`) for plugins that wrap external identity providers.

## Watch sync providers

`watch_sync_provider.v1` lets external plugins participate in the server's host-owned
watch-provider pipeline. The host owns encrypted per-profile credentials,
authorization-code and device-code flow state, durable desired-state events,
retries, ordering, and reconciliation. Plugins are stateless protocol adapters:
they receive secrets only for the duration of an RPC, map rich movie/episode
identity to an upstream service, and return typed apply or retry outcomes.

Watch-sync plugins must not persist or log credentials, authorization codes,
provider flow state, or secret configuration. `ApplyEvents` is an at-least-once
contract; plugins must treat `event_id` as stable across retries and implement
convergent desired-state updates rather than increments. That rule also applies
to scrobble stops: replaying the same event ID must not create another play.
For playback events, `completed` is the host's authoritative watched decision;
plugins must not infer completion from `watch_history_id` or percentage alone.

Authenticated RPCs receive the same host-owned capability, configuration, and
credential data through `WatchSyncAuthenticatedContext`. The context exists
only for one invocation and is never plugin configuration or plugin state.
Credentials returned by any RPC are complete authoritative replacements, not
patches. The host validates and persists them before consuming results, pages,
or faults—even when the response contains a fault. If credential persistence
fails, the host commits no other response data.

Device-code plugins register both `WatchSyncProvider` and the separate
`WatchSyncDeviceAuthorizationService`. Keeping device authorization in a
second service preserves source compatibility for v0.12 Go providers that
implemented `WatchSyncProviderServer` directly. Register it without changing
the released `CapabilityServers` shape:

```go
runtime.ServeManifestWithOptions(manifestJSON, version, servers,
    runtime.WithWatchSyncDeviceAuthorization(deviceAuthServer))
```

A pending poll may replace its opaque provider state, polling interval, and
expiry; the host encrypts and persists those values before the next poll.
Those updates remain part of the same user challenge, so the original user code
and verification URL must stay valid until expiry. An explicitly empty
`provider_state` clears the prior state; omitting it retains the prior state.

`WatchSyncProviderConfig` is keyed by manifest config key and field, for example
`provider.client_id`. Scalar values are sent as strings and structured values
as JSON. Fields marked secret in the manifest are sent through `secret_values`;
undeclared fields are treated as secret. Plugins must accept configuration from
the RPC context rather than relying on process-global state.

Descriptors and events use the shared `WatchSyncMediaType` enum so advertised
support and delivered media cannot drift between string conventions. Apply
results pair their delivery status with a typed fault: successful results omit
the fault, temporary retries use `TEMPORARY`, rate limits use `RATE_LIMITED`
with an optional delay, and rejected events use a non-retryable fault code.
Connection-wide faults such as invalid credentials belong on the RPC response.

`ListRemoteState` returns provider-neutral typed subrecords. `watched` carries a
play count and last-watched time; `progress` carries a fractional percentage and
paused time; `favorite` and `watchlist` carry list membership. An item may
contain multiple state families. The host requests only the state families a
sync phase needs, keeps that phase's `cursor` fixed while following ephemeral
page tokens, commits each successful page, and only then persists the final
`next_cursor`. `complete_snapshot=true` means the traversal is authoritative;
when false, missing items are not deletions. An incremental favorite or
watchlist removal is an item whose corresponding list state has `removed=true`;
it may omit `media` when `provider_item_key` identifies a record previously
returned to the host. When
`provides_watchlist_order=true`, watchlist traversals must be complete snapshots
and the order of returned watchlist states is the remote list order. Event
`list_position` is presence-aware: an explicit zero means the first position,
while omission means no requested ordering.

## Scan sources

The `scan_source.v1` capability is for Autoscan providers. The host owns the
poll timer, marker persistence, path rewrites, validation, dedupe, and scan
enqueueing. The plugin only polls its upstream provider and returns changed
absolute paths in that provider's source namespace. The host applies autoscan
source rewrite rules before enqueueing scans.

The host resolves the configured upstream connection and passes it to
`PollChanges` for each poll. Plugins should treat request values such as API
keys as transient secrets and avoid logging them without redaction.

## Self-describing binaries

Direct binary upload works best when the plugin embeds a manifest template and computes its own executable checksum at runtime before returning `Runtime.GetManifest`. That keeps the plugin installable without requiring a checked-out server repository or a sibling `manifest.json` file at upload time. The example plugin shows the pattern.

## Compatibility

Compatibility and versioning expectations are documented in [`docs/compatibility.md`](docs/compatibility.md).

## Releases

SDK releases are cut from semver tags such as `v0.1.0` and published through GitHub Actions.

- Additive public API changes belong in a new minor release.
- Compatible fixes and documentation updates belong in a patch release.
- Breaking public API, protobuf, or manifest contract changes require a new major version.

Before downstream repos stop using local workspace overrides, the required SDK commit must be pushed and tagged here first.

## Build & test

```bash
GOWORK=off go test ./...             # what CI runs; make test runs it without GOWORK=off
make proto                           # regenerate protobuf code (needs protoc; installs buf and the generators under ./bin/)
./scripts/verify-private-release.sh  # the private-release guard (needs ripgrep); run before committing docs too
./scripts/build-compat-probe.sh      # dist/compat-probe + checksum + manifest for a server smoke test
```

## Documentation

- [docs/admin-guide.md](docs/admin-guide.md) — for the SDK maintainer and server operator: repository layout, building and regenerating protobuf code, the guard tests, CI and the release workflow, versioning, plugin packaging and installation, the process and security model, troubleshooting.
- [docs/user-guide.md](docs/user-guide.md) — for plugin authors: from an empty directory to an installed plugin, then the reference for the manifest, settings, every capability, the host client, helpers, testing, packaging and versioning.
- [docs/compatibility.md](docs/compatibility.md) — the compatibility boundary and versioning rules for the SDK and its consumers.
- [docs/private-release.md](docs/private-release.md) — the step-by-step private release and rollback procedure.
- [docs/runtime-host.md](docs/runtime-host.md) — the `RuntimeHost.v1` RPC reference.

## License

Apache-2.0. See [LICENSE](LICENSE).

## Upstream attribution

This project is an independent fork of
[Silo Plugin SDK](https://github.com/Silo-Server/silo-plugin-sdk). See
[NOTICE](NOTICE) for the upstream version, revision, and affiliation statement.
