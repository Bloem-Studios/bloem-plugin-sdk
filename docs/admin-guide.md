---
title: Bloem Plugin SDK Admin Guide
description: How to build, test, version and release the Bloem plugin authoring SDK, how a finished plugin is packaged and installed on a Bloem server, and where the security boundary between server and plugin sits.
summary: Repository layout, build and protobuf regeneration, the contract guard tests, CI and the private release workflow, versioning and server compatibility, plugin packaging and installation, the process and security model, and troubleshooting.
tags:
  - admin
  - sdk
  - release
  - ci
  - plugins
audience:
  - maintainer
  - operator
last_reviewed: 2026-09-05
related:
  - user-guide.md
  - compatibility.md
  - private-release.md
  - runtime-host.md
---

# Bloem Plugin SDK Admin Guide

This guide is for the person who looks after the Bloem Plugin SDK repository: builds it, keeps the
protobuf contract stable, cuts releases, and decides which server versions a plugin built with it
can run on. It also explains, for a server operator, how a finished plugin is packaged, how the
server installs and runs it, and what the plugin can and cannot reach.

If you want to *write* a plugin, read the [User Guide](user-guide.md) instead. It is the step-by-step
authoring reference. This guide assumes nothing beyond a working Go toolchain and Git.

**How this guide is organised.** Part 1 explains what the repository is. Part 2 covers building and
testing it. Part 3 is CI, releases and versioning. Part 4 is the compatibility contract with Bloem
servers. Part 5 is packaging, installation and the security boundary. Part 6 is troubleshooting.
A glossary and a list of source references close the document.

---

## Part 1 — What this repository is

### 1.1 A library, not a plugin

The Bloem Plugin SDK is a **Go library**. It is not a program you run on a server. A plugin author
adds it to their own Go module with `go get`, and it gives them:

- the generated protobuf and gRPC code for every plugin capability the Bloem server understands;
- helpers to load and validate a plugin's `manifest.json`;
- a `Serve` function that turns a plugin into a process the server can launch and talk to;
- a typed client for calling back into the server (the "runtime host").

The compiled output of a plugin project is a single static binary. The server launches that binary as
a child process and speaks gRPC to it over a local connection. Nothing from this repository is
installed on the server itself; the server carries its own copy of the same contract.

### 1.2 Names that keep an older spelling

The product is Bloem. A few technical identifiers in this repository keep spellings from the
project's origins, because changing them would break compatibility with existing servers and
existing plugins. They are not mistakes and must not be "tidied up":

| Identifier | Value | Why it stays |
|---|---|---|
| Go module path | `github.com/Bloem-Studios/bloem-plugin-sdk` | It is the literal import path every plugin's `go.mod` uses. `internal/projectidentity/identity_test.go` and `scripts/verify-private-release.sh` both fail if it changes. |
| Protobuf package | `silo.plugin.v1` | It is the wire name of every message and service. `compat/v1_contract_test.go` fails if it changes. |
| Manifest field | `silo_api_version` | The server checks it before installing a plugin (see 4.1). Field number 4 is pinned by the contract test. |
| Handshake cookie | `SILO_PLUGIN=silo-rpc-plugin-v1` | The environment variable the server sets so the plugin binary knows it was launched by a real host. |
| Plugin set name | `silo` | The name under which the gRPC plugin is registered with the process-launch library. |
| Example plugin ids | `bloem.example.hello-task`, `bloem.example.runtime-host`, `bloem.compat.probe` | Pinned by `examples/examples_test.go` and `cmd/compat-probe/main_test.go`. |

The `NOTICE` file records the upstream project and revision this SDK was forked from; the identity
test checks that it still does. See the top-level `README.md` for the attribution wording.

### 1.3 Repository layout

| Path | What it holds |
|---|---|
| `proto/silo/plugin/v1/*.proto` | The source of truth: one `.proto` file per capability family plus `common.proto` (manifest, runtime) and `runtime_host.proto` (host call-backs). |
| `pkg/pluginproto/silo/plugin/v1/` | Generated Go code (`*.pb.go`, `*_grpc.pb.go`). Committed; regenerate with `make proto`. |
| `pkg/pluginsdk/capability` | String constants for every capability type (`metadata_provider.v1` and so on) and the `KnownTypes` list. |
| `pkg/pluginsdk/manifest` | Manifest loading, validation, checksum stamping, presentation rules, HTTP route and asset registration. |
| `pkg/pluginsdk/config` | JSON Schema validation of configuration values against a manifest. |
| `pkg/pluginsdk/convert` | Conversion between capability descriptors and plain Go maps (used by hosts that store capabilities in a database). |
| `pkg/pluginsdk/runtime` | `Serve`, `ServeManifest`, the `manifest` subcommand, handshake constants, `CapabilityServers`, and `Host()`. |
| `pkg/pluginsdk/runtimedefault` | An embeddable `Runtime` server that handles `BindHostBroker` for you. |
| `pkg/pluginsdk/runtimehost` | Typed client for the server's `RuntimeHost` service. |
| `pkg/pluginsdk/httpclient` | Small outbound JSON HTTP client for plugins that talk to a third-party API with an `X-Api-Key` header. |
| `cmd/compat-probe` | A tiny plugin (`metadata_provider.v1`, empty search) used to prove that a server can launch an SDK-built binary. |
| `compat/` | Tests that lock the wire contract and the proto `go_package` options. |
| `internal/projectidentity` | Test that the module path and `NOTICE` attribution are intact. |
| `examples/` | Two complete plugins: `hello-scheduled-task` and `hello-runtime-host`. |
| `scripts/` | `build-compat-probe.sh`, `verify-private-release.sh`, `test-private-release.sh`. |
| `.github/workflows/` | `ci.yml` (push to `main` and pull requests) and `release.yml` (tags `v*`). |
| `docs/` | `compatibility.md`, `private-release.md`, `runtime-host.md`, and the two guides. |
| `Makefile`, `buf.yaml`, `buf.gen.yaml` | Protobuf toolchain configuration. |

### 1.4 What you need on your machine

| Tool | Version | Used for |
|---|---|---|
| Go | 1.26 (`go 1.26.0` in `go.mod`; CI installs `1.26`) | Everything. |
| Git | any recent | Cloning, tagging. |
| `protoc` | any recent | Only for `make proto`. The Makefile refuses to run without it. |
| `buf`, `protoc-gen-go` v1.36.11, `protoc-gen-go-grpc` v1.6.1 | installed automatically into `./bin/` by `make proto` | Only for `make proto`. |
| `rg` (ripgrep) | any | Required by `scripts/verify-private-release.sh`; the guard fails with `required command not found: rg` without it. |
| `gh` (GitHub CLI) | any recent | Only for cutting a release (Part 3). |

`./bin/`, `dist/`, `go.work` and `go.work.sum` are ignored by Git (see `.gitignore`), so tooling
installed by the Makefile and local workspace files never end up in a commit.

---

## Part 2 — Building and testing the SDK

### 2.1 Clone and run the tests

```sh
git clone git@github.com:Bloem-Studios/bloem-plugin-sdk.git
cd bloem-plugin-sdk
GOWORK=off go test ./...
```

`GOWORK=off` tells Go to ignore any `go.work` file you may have created for multi-repository
development (see 3.6). CI always runs with it, so running the same way locally avoids surprises.
`make test` runs `go test ./...` without that variable.

The test run is quick and has no external dependencies. One test, `cmd/compat-probe`'s
`TestManifestSubcommand`, builds the probe binary into a temporary directory and executes it with the
`manifest` argument, so it needs a working `go build`.

### 2.2 Build the examples and the probe

```sh
GOWORK=off go build ./examples/hello-scheduled-task
GOWORK=off go build ./examples/hello-runtime-host
./scripts/build-compat-probe.sh
```

`build-compat-probe.sh` builds `cmd/compat-probe` with `CGO_ENABLED=0 -trimpath` into
`dist/compat-probe`, writes `dist/compat-probe.sha256`, and runs the binary's `manifest` subcommand
to produce `dist/compat-probe.manifest.json`. That manifest has the real checksum filled in, so the
three files together are a ready-to-install plugin package for a server smoke test (see 5.2).

### 2.3 Regenerating the protobuf code

Edit the `.proto` files under `proto/silo/plugin/v1/`, then:

```sh
make proto
```

What the target does, in order:

1. Checks `protoc` is on your `PATH`; exits with `protoc is required` if not.
2. Installs `buf` (latest), `protoc-gen-go` v1.36.11 and `protoc-gen-go-grpc` v1.6.1 into `./bin/` if
   they are missing.
3. Runs `buf generate` with `./bin` first on the `PATH`.

`buf.gen.yaml` writes both plugins' output to `pkg/pluginproto` with `paths=source_relative`, and
passes `require_unimplemented_servers=false` to the gRPC generator. That last option matters: it means
a plugin may implement a capability server without embedding the `Unimplemented*Server` struct, which
keeps older plugins compiling when a service gains a method.

`buf.yaml` enables the `STANDARD` lint rules and the `FILE` breaking-change rules for the `proto`
module. Run `./bin/buf lint` and `./bin/buf breaking --against <git ref>` before committing a proto
change; the release rules in 3.5 forbid breaking changes inside `v1`.

> **Commit the generated code.** `pkg/pluginproto` is checked in. Plugin authors `go get` the module
> and never run the generator, so a proto change without regenerated Go files is invisible to them.

### 2.4 The guard tests and what each one protects

Several tests exist only to stop accidental contract or identity changes. Know them, because a
failing guard is a signal to revert, not to "fix the test".

| Test | File | Fails when |
|---|---|---|
| `TestV1WireContract` | `compat/v1_contract_test.go` | The proto package is not `silo.plugin.v1`; `PluginManifest.silo_api_version` is not field 4; the `Runtime`, `MetadataProvider`, `ScanSource` or `RuntimeHost` services are renamed; the handshake constants change; `metadata_provider.v1`, `image_resolver.v1` or `scan_source.v1` disappear from `capability.KnownTypes`. |
| `TestProtoSourcesPreserveWireIdentity` | `compat/source_guard_test.go` | Any `.proto` file contains `package bloem.plugin` or `bloem_api_version`, or declares an active `go_package` outside `github.com/Bloem-Studios/bloem-plugin-sdk/` (comments are stripped first, so a commented-out declaration cannot fool it). |
| `TestBloemModuleAndAttribution` | `internal/projectidentity/identity_test.go` | `go.mod` does not start with the expected module line, or `NOTICE` loses the upstream name, version, revision, licence or affiliation sentence. |
| `TestExampleManifestIdentity` | `examples/examples_test.go` | Either example manifest changes its `plugin_id` or its `silo_api_version`. |
| `TestManifestSubcommand` | `cmd/compat-probe/main_test.go` | The probe no longer prints a valid manifest with `plugin_id` `bloem.compat.probe`, `silo_api_version` `v1` and exactly one `metadata_provider.v1` capability. |
| `capability_servers_compat_test.go` | `pkg/pluginsdk/runtime/` | The unkeyed field order of `runtime.CapabilityServers` changes. Plugins written against v0.12 construct it positionally, so new servers must be added through options (see 4.4), not new struct fields. |

### 2.5 The private-release guard

`scripts/verify-private-release.sh` runs in CI before the tests and again during a release. It exits
non-zero, printing `private release guard failed: <reason>`, when:

| Check | Failure text |
|---|---|
| `rg` is missing | `required command not found: rg` |
| The first line of `go.mod` is not `module github.com/Bloem-Studios/bloem-plugin-sdk` | `unexpected module` |
| `go.mod` contains a `replace` directive | `go.mod contains a replace directive` |
| Any workflow under `.github` matches `gh repo edit.*visibility`, `npm publish`, `docker push` or `pkg.go.dev` | `workflow contains a public publication path` |
| Any file in the repository (except the guard itself) contains an absolute macOS or Linux home-directory path, or a GitHub URL with `user:password@` embedded | `repository contains a local path or credential-bearing URL` |

> **Documentation counts.** The path scan covers every file, including Markdown. Never paste an
> absolute home-directory path into a doc, an example or a comment in this repository; write
> `~/projects/...` or a relative path instead.

`scripts/test-private-release.sh` is the guard's own test. It checks that the release workflow pins
its checkout to `${{ github.sha }}`, resolves the remote tag with `git ls-remote origin`, compares it
to the event SHA, and installs ripgrep before every guard step in both workflows; it then simulates a
missing `rg` and an `rg` that exits 2 and expects the right failure messages; finally it runs the real
guard on the clean tree. Run it after touching either script or either workflow:

```sh
./scripts/test-private-release.sh          # all cases
./scripts/test-private-release.sh workflow # or: workflow-rg, missing-rg, rg-error
```

---

## Part 3 — CI, releases and versioning

### 3.1 The CI workflow

`.github/workflows/ci.yml` runs on every push to `main` and on every pull request, on
`ubuntu-latest` with Go `1.26`:

1. `apt-get install ripgrep`
2. `./scripts/verify-private-release.sh`
3. `GOWORK=off go test ./...`
4. `GOWORK=off go build ./examples/hello-scheduled-task`
5. `GOWORK=off go build ./examples/hello-runtime-host`

There is no lint job, no coverage upload and no artifact publication in CI.

### 3.2 The release workflow

`.github/workflows/release.yml` runs only when a tag matching `v*` is pushed. It has `contents:
write` permission (needed to create the GitHub Release) and two jobs:

- **test** — identical to CI, but checks out `${{ github.sha }}` explicitly.
- **release** (needs `test`) — checks out the same SHA, then runs a guard step that asks the remote
  for `refs/tags/<tag>` and its peeled form and compares the result to the SHA the workflow was
  started for. If somebody moved the tag while the tests were running, the step fails with
  `release guard failed: tag <tag> resolves to <sha>, expected <sha>` and no release is created.
  Otherwise `softprops/action-gh-release@v2` creates a GitHub Release for the tag with
  auto-generated release notes.

The workflow never changes repository visibility and never publishes to a package registry. A
"release" here is a Git tag plus a GitHub Release record in the private repository; consumers fetch
the module through Git with credentials that can read the repository.

### 3.3 Cutting a release, step by step

The canonical procedure is in [`docs/private-release.md`](private-release.md); this is the same
sequence with each step explained.

1. **Choose the version.** Semver, `vMAJOR.MINOR.PATCH`. Rules are in 3.5. Two constraints from the
   repository's history: `v0.13.2` is a failed historical candidate that must never be reused or
   moved, and `v0.13.3` is the first verified release of this SDK.
2. **Start from a clean checkout of `main`.** `git status --porcelain` must print nothing.
3. **Prove the tag is unused** three ways: not in `git tag --list`, not on the remote
   (`git ls-remote --tags origin refs/tags/<tag> refs/tags/<tag>^{}` prints nothing), and no GitHub
   Release with that tag (`gh api --include repos/<org>/<repo>/releases/tags/<tag>` returns HTTP 404).
   The script in `private-release.md` does exactly these checks and stops on any other answer,
   including a network error, because "could not check" is not "unused".
4. **Confirm the repository is private**: `gh api repos/<org>/<repo> --jq '.visibility'` prints
   `private`.
5. **Run the full local gate**: `GOWORK=off go test ./...` then `./scripts/verify-private-release.sh`.
6. **Tag and push only the tag**: `git tag -a <tag> -m "Bloem Plugin SDK <tag>"` then
   `git push origin <tag>`.
7. **Watch the workflow**, then confirm with `gh release view <tag>` and re-check visibility.

> The documented script uses the repository's Git identity (`Bloem-Studios/bloem-plugin-sdk`) in its
> `gh api` calls. Use whatever `gh repo view --json nameWithOwner` reports for the remote you push to;
> this checkout's `origin` is `Bloem-Studios/bloem-plugin-sdk`.

### 3.4 Rolling a release back

From `private-release.md`: delete the GitHub Release and its tag **only if no downstream repository
has pinned that tag**. Remove the release first, then the remote tag, then the local tag. If anything
already depends on it, leave it in place and publish a new patch tag with the fix. Tags that have been
consumed are immutable in practice, because Go's module cache and checksum database entries would
disagree with a moved tag.

### 3.5 Versioning policy

The SDK is a semver boundary. From `README.md` and `docs/compatibility.md`:

| Change | Release type |
|---|---|
| Compatible fix, documentation, example | patch |
| Additive public Go API, new proto field, new capability family, new `RuntimeHost` RPC | minor |
| Renaming or removing a proto field, service or enum value in `v1`; changing the manifest contract; breaking a public Go symbol | major (and, for the wire, a new proto package version) |

Rules that follow from this:

- Prefer additive protobuf evolution. Never renumber a field.
- New plugin functionality arrives as new capability families or new fields, not rewrites.
- New gRPC services that a plugin can serve are added through `runtime.ServeManifestOption` values
  (4.4), never by appending fields to `CapabilityServers`.
- Consumers (servers and plugins) pin a released tag in `go.mod`. A first-party consumer must not
  merge code that needs a new SDK symbol until the tag that contains it exists.
- CI and release builds of consumers run with `GOWORK=off` and never check this repository out as a
  sibling source dependency.

### 3.6 Developing against a server or a plugin locally

When you are changing the SDK and a plugin (or the server) at the same time, use a Go workspace so
the other module compiles against your working tree instead of a published tag:

```sh
cd ~/projects/my-plugin
go work init . ../bloem-plugin-sdk
```

`go.work` is git-ignored here, and the release guard rejects a `replace` directive in `go.mod`, so
neither mechanism can leak into a release. Before the plugin or server repository drops its workspace
override, the SDK commit it needs must be pushed **and tagged** here first.

---

## Part 4 — Compatibility with Bloem servers

### 4.1 The `silo_api_version` gate

Every manifest carries `silo_api_version`. It is a coarse compatibility switch: the server compares
it with the API version it implements and refuses to install a plugin whose value differs. Every
manifest in this repository declares `"v1"`, and that is the only value a current server accepts.
The SDK's own validator does not require the field, but the server does, so treat it as mandatory.

### 4.2 What "the v1 wire contract" means

A plugin built with this SDK and a server that speaks v1 agree on:

- the protobuf package `silo.plugin.v1` and every message and field number in it;
- the gRPC service names (`Runtime`, `MetadataProvider`, `ImageResolver`, `MarkerProvider`,
  `MediaAnalyzer`, `ScheduledTask`, `ScanSource`, `RequestRouter`, `EventConsumer`, `AuthProvider`,
  `HttpRoutes`, `WatchSyncProvider`, `WatchSyncDeviceAuthorizationService`, and the host-side
  `RuntimeHost`);
- the process handshake: protocol version `1`, environment variable `SILO_PLUGIN` with value
  `silo-rpc-plugin-v1`, plugin set name `silo`;
- the manifest JSON shape (protojson encoding of `PluginManifest`; unknown keys are ignored on load).

Because the server embeds the same contract, an SDK release that only *adds* things is safe for
every existing server: a newer plugin may send fields an older server ignores, and an older plugin
never sends fields a newer server requires.

### 4.3 Capability families known to this SDK

`capability.KnownTypes` lists exactly these thirteen strings. A manifest that names any other type
fails validation with `unknown type`.

| Type string | gRPC service in this SDK | Notes |
|---|---|---|
| `metadata_provider.v1` | `MetadataProvider` | Search, details, seasons, episodes, images, people, image URL resolution. |
| `image_resolver.v1` | `ImageResolver` | Image URL resolution only. |
| `marker_provider.v1` | `MarkerProvider` | Intro/credits/recap/preview segments from an external source. |
| `media_analyzer.v1` | `MediaAnalyzer` | Local analysis of a media file for intro and credits ranges. |
| `scheduled_task.v1` | `ScheduledTask` | Host-scheduled jobs. |
| `event_consumer.v1` | `EventConsumer` | Receives host and plugin events. |
| `auth_provider.v1` | `AuthProvider` | Password and OAuth/OIDC login. |
| `http_routes.v1` | `HttpRoutes` | Plugin-served HTTP endpoints and pages. |
| `request_router.v1` | `RequestRouter` | Media request fulfilment through downstream services. |
| `scan_source.v1` | `ScanSource` | Autoscan change sources. |
| `watch_sync_provider.v1` | `WatchSyncProvider` (+ `WatchSyncDeviceAuthorizationService`) | External watch-history sync. |
| `audiobook_backend.v1` | none | Constant only; no service definition ships in this SDK. |
| `ebook_backend.v1` | none | Constant only; no service definition ships in this SDK. |

There is no subtitle capability in this SDK version.

### 4.4 Adding a service without breaking existing plugins

`runtime.CapabilityServers` is an unkeyed struct that plugins written for v0.12 fill positionally.
The watch-sync device-authorization service was added after that release, so it is registered through
an option rather than a new field:

```go
runtime.ServeManifestWithOptions(manifestJSON, version, servers,
    runtime.WithWatchSyncDeviceAuthorization(deviceAuthServer))
```

`DefaultPluginSetWithWatchSyncDeviceAuthorization` does the same for plugins that call `Serve`
directly. Follow this pattern for any future service.

---

## Part 5 — Packaging, installation and the security boundary

This part describes what the Bloem server does with a plugin. The behaviour lives in the server
repository (`internal/plugins` and `internal/pluginhost`); it is summarised here because SDK
maintainers and server operators both need it, and because the SDK's helpers exist to satisfy it.

### 5.1 The plugin package

A plugin package is a **zip archive** with these entries at the root (no sub-directory):

| Entry | Required | Content |
|---|---|---|
| `manifest.json` | yes | The manifest, protojson-encoded. |
| `plugin` | yes | The statically linked executable, named exactly `plugin`. |
| any path listed under `assets[].path` | yes if declared | Static files the plugin serves. The server refuses the archive if a declared asset is missing. |

Server-side manifest validation (`internal/plugins/manifest.go`) is stricter than the SDK's
`manifest.Validate`. On top of the SDK rules it requires:

- `silo_api_version` present (and equal to the server's, see 4.1);
- at least one capability, with no duplicate `(type, id)` pair;
- `checksum` present and equal to the lowercase hex SHA-256 of the `plugin` file
  (`plugin binary checksum does not match manifest` otherwise);
- `supported_platforms` non-empty;
- `plugin_id` not equal to the reserved `silo.builtin`.

Plugin repositories typically commit `manifest.json` with `"checksum": "__CHECKSUM__"` and let their
release job substitute the real value. `manifest.LoadWithChecksum` (used by `runtime.ServeManifest`)
fills the checksum in at run time, so a binary's `manifest` subcommand always prints the correct
value for itself; the examples do the same by hand and also fill `supported_platforms` from the
running OS and architecture when the manifest omits it.

**There is no code signing.** Integrity is the SHA-256 checksum, verified when the archive is opened.
Provenance (who published it, whether it is approved) is assigned by the server or a catalog, never by
the manifest's `presentation` block, which is self-declared.

### 5.2 How a plugin gets onto a server

Two paths exist, both administrator-only:

- **Catalog install.** The server's plugin catalog (`plugin_repositories`) points at packages by
  URL and checksum; the admin picks one in **Admin → Plugins**. The catalog service compares
  `silo_api_version` and `supported_platforms` against the host before offering an entry.
- **Direct upload ("sideload").** `POST /api/v1/admin/plugins/uploads` with a multipart form field
  named `archive` containing the zip. Large packages can use the chunked variant
  (`POST /uploads/chunked`, `PUT /uploads/chunked/{upload_id}/chunks/{chunk_index}`,
  `POST /uploads/chunked/{upload_id}/complete`, `DELETE /uploads/chunked/{upload_id}`). The call
  needs an administrator session or API key and goes to the server's native API port, not to a
  compatibility port.

On a successful install the server unpacks the package under its plugin directory, records the
installation and its capabilities in the database, launches the process, and calls
`Runtime.Configure` with the current configuration. No server restart is involved.

### 5.3 The process model

The server launches the plugin with the `go-plugin` library (`internal/pluginhost/host.go`):

- `exec.Command(<binary path>)` with no arguments. The `manifest` subcommand is only used when the
  server (or you) wants the manifest without starting the plugin.
- The handshake from `runtime.HandshakeConfig()`: the server sets `SILO_PLUGIN=silo-rpc-plugin-v1`
  in the child's environment; a binary started without it prints the standard go-plugin message and
  exits, which is why running a plugin by hand "does nothing".
- `AllowedProtocols` is gRPC only.
- The plugin's stdout and stderr are wired to the server's own stdout and stderr, so anything the
  plugin logs (for example through `hclog`) appears in the server log.
- After the connection is up, the server dispenses the `silo` plugin set, calls
  `Runtime.GetManifest`, opens a broker stream for `RuntimeHost`, and calls
  `Runtime.BindHostBroker` with that stream id.

One plugin per process, one process per installation. Stopping or disabling an installation kills
the process; re-enabling launches it again.

### 5.4 The security boundary

What the plugin can and cannot do, and where each rule is enforced:

| Concern | Rule | Enforced in |
|---|---|---|
| Installing plugins | Only a server administrator can install, enable, disable or remove a plugin; there is no self-install and no install from a viewer session. | server admin routes |
| Binary integrity | Checksum verified before unpacking; mismatch rejects the package. | server `archive_cache.go` |
| Process trust | The plugin runs as the same OS user as the server and can do anything that user can (open network connections, read the filesystem). The SDK does not sandbox it. Install only plugins you trust. | operating system |
| Events | Plugin-published event names are always prefixed server-side with `plugin.<plugin_id>.`, so a plugin cannot forge a core event. | server `RuntimeHost` |
| Plugin-to-plugin HTTP | `CallPluginHTTP` is treated as authenticated service traffic, never as administrator traffic. | server `RuntimeHost` |
| Catalog reads | `ListLibraryMedia` and `GetCatalogStats` return public-safe rows only: no filesystem paths, no stream URLs, no playback grants. | server `RuntimeHost` |
| Streams | `MintScopedStream` issues a short-lived, narrowly scoped grant (expiry, resolution cap, seek/download flags, watermark, audit subject). | server `RuntimeHost` |
| Secrets | For `scan_source.v1`, `request_router.v1` and `watch_sync_provider.v1` the server stores credentials and passes them **per RPC**; the plugin must not persist or log them. Watch-sync secret config travels in `secret_values`, never in `Configure`. | contract (proto comments) and server storage |
| Configuration | Admin-entered settings are validated against the manifest's JSON Schema (`config.ValidateValue`, draft 2020-12) before being accepted. Fields marked `secret` are stored encrypted. | SDK `config` package, server |
| HTTP routes | Each route declares an `access` level in the manifest; the server enforces it before proxying to `HttpRoutes.Handle`. | server HTTP proxy |

### 5.5 Configuration surfaces

The SDK has **no environment variables, config files or command-line flags of its own**. The only
command-line behaviour it adds to a plugin binary is the `manifest` subcommand. Everything a plugin
needs at run time arrives through gRPC:

- `Runtime.Configure(ConfigEntry[])` — the manifest-declared global settings, as `key` → JSON object.
- Per-RPC request fields for capability-specific and connection-specific values.
- `RuntimeHost.SetGlobalConfigEntry` when the plugin wants to persist a value the admin did not set.

Plugin authors are free to read their own environment variables, but the server sets none besides
the handshake cookie.

### 5.6 Logs and diagnostics

- Plugin logs go to the server's stderr/stdout (5.3). Use `hclog` with a named logger so lines are
  attributable; the `hello-runtime-host` example shows the pattern.
- `./plugin manifest` prints the manifest the plugin will present, checksum included, and exits 0.
  Errors go to stderr with exit code 1 (`failed to get manifest: ...` or `failed to encode manifest`).
- `dist/compat-probe` (2.2) is the reference binary for "can this server launch an SDK-built plugin at
  all". If the probe installs and its capability appears in **Admin → Plugins**, the transport is fine
  and any problem is in the plugin under test.

---

## Part 6 — Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `private release guard failed: required command not found: rg` | ripgrep not installed. | `brew install ripgrep` / `apt-get install ripgrep`. |
| `private release guard failed: repository contains a local path or credential-bearing URL` | A doc, comment or fixture contains an absolute home-directory path (macOS or Linux style) or a URL with embedded credentials. | Run the guard; the `rg` pattern inside `scripts/verify-private-release.sh` shows the exact match. Rewrite the path with `~` or a relative path. |
| `private release guard failed: go.mod contains a replace directive` | A local override was committed. | Remove it; use `go.work` (git-ignored) for local development. |
| `unexpected module declaration` from `internal/projectidentity` | `go.mod` module line changed. | Restore `module github.com/Bloem-Studios/bloem-plugin-sdk`. The import path is part of the contract. |
| `compat` tests report `go-plugin handshake contract changed` or `protobuf package = …` | Someone renamed a wire identifier. | Revert. These values are shared with every deployed server. |
| `make proto` prints `protoc is required` | `protoc` not on `PATH`. | Install protoc; the other generators are fetched automatically. |
| Generated code differs from a colleague's | Different `protoc-gen-go` / `protoc-gen-go-grpc` versions. | Delete `./bin/` and rerun `make proto` so the pinned versions are installed. |
| `release guard failed: tag … resolves to …, expected …` in the Release workflow | The tag was moved or re-pushed after the workflow started. | Never move tags. Create a new patch tag. |
| Release workflow did not run | The tag does not match `v*`, or it was created on GitHub without pushing a Git tag. | Push an annotated tag from a clean checkout as in 3.3. |
| A plugin binary "does nothing" when run by hand | It was launched without the `SILO_PLUGIN` handshake variable and exited. | That is expected. Use `./plugin manifest` to inspect it; only a server can run it. |
| Upload rejected: `plugin binary checksum does not match manifest` | `manifest.json` in the zip still says `__CHECKSUM__`, or was generated from a different build. | Recompute: `shasum -a 256 plugin` and put the hex digest in `checksum`, or use the output of `./plugin manifest`. |
| Upload rejected: `plugin manifest supported_platforms is required` | The SDK validator allows an empty list; the server does not. | Add `"supported_platforms": [{"os": "linux", "arch": "amd64"}]` to `manifest.json`. |
| Upload rejected: `plugin archive is missing plugin binary` | The executable is not named `plugin`, or the zip has a top-level folder. | `zip plugin.zip manifest.json plugin` from inside the build directory. |
| Server refuses install because of the API version | `silo_api_version` is missing or not `v1`. | Set it to `"v1"`. |
| `runtime.Host()` returns `nil` inside a handler | `BindHostBroker` has not been called yet (very early in start-up), the `Runtime` server does not implement it, or the broker dial failed. | Embed `runtimedefault.Server` or use `ServeManifest`; treat `nil` as transient and retry later. |
| Every `RuntimeHost` call after the first hangs | A plugin dialled the broker itself per call. | Always go through `runtime.Host()`, which caches one client on the single stream the server listens on. |
| A plugin compiled against a newer SDK fails on an older server | It uses a `RuntimeHost` RPC or capability the server does not have. | Check the server's SDK tag; keep additive features optional or gate on `ListInstalledPlugins` / errors. |

---

## Glossary

- **Broker** — the multiplexed connection inside a go-plugin session over which the server exposes `RuntimeHost` to the plugin.
- **Capability** — one kind of service a plugin offers (metadata provider, scheduled task…). A plugin advertises capabilities in its manifest and serves the matching gRPC service.
- **Catalog** — a server-side list of installable plugin packages, keyed by URL and checksum.
- **Checksum** — hex SHA-256 of the plugin executable; the server verifies it before installing.
- **Compat probe** — `cmd/compat-probe`, a minimal plugin used to prove a server can launch SDK-built binaries.
- **Guard test** — a test whose only job is to fail when a contract or identity value changes.
- **Handshake** — the go-plugin start-up exchange (protocol version, magic cookie) that proves a binary was launched by a real host.
- **Manifest** — `manifest.json`, the plugin's self-description: id, version, checksum, API version, platforms, capabilities, settings schema, routes, assets.
- **Private release** — a Git tag plus a GitHub Release in the private repository; nothing is published to a public registry.
- **RuntimeHost** — the gRPC service the server exposes to plugins for events, catalog reads, peer discovery and scoped streams.
- **Semver** — `MAJOR.MINOR.PATCH` versioning; additive = minor, compatible fix = patch, breaking = major.
- **Sideload** — installing a plugin by uploading its zip directly rather than from a catalog.
- **Wire contract** — the protobuf package, messages, field numbers, service names and handshake values that server and plugin must agree on.

## Source References

- `README.md` — package list, capability families, author workflow, presentation rules, release rules
- `go.mod`, `Makefile`, `buf.yaml`, `buf.gen.yaml`, `.gitignore` — toolchain and generation settings
- `.github/workflows/ci.yml`, `.github/workflows/release.yml` — CI and release jobs
- `scripts/verify-private-release.sh`, `scripts/test-private-release.sh`, `scripts/build-compat-probe.sh`
- `docs/compatibility.md`, `docs/private-release.md`, `docs/runtime-host.md`
- `compat/v1_contract_test.go`, `compat/source_guard_test.go`, `internal/projectidentity/identity_test.go`, `examples/examples_test.go`, `cmd/compat-probe/main_test.go`, `pkg/pluginsdk/runtime/capability_servers_compat_test.go` — guard tests
- `pkg/pluginsdk/runtime/runtime.go`, `serve_manifest.go` — handshake constants, `Serve`, `manifest` subcommand, `Host()`
- `pkg/pluginsdk/manifest/manifest.go`, `checksum.go` — validation and checksum stamping
- `pkg/pluginsdk/capability/capability.go` — `KnownTypes`
- `proto/silo/plugin/v1/common.proto`, `runtime_host.proto`, `scan_source.proto`, `request_router.proto`, `watch_sync_provider.proto` — contract comments on secrets, event stamping and public-safe reads
- Bloem server: `internal/pluginhost/host.go` (launch), `internal/plugins/manifest.go` (server-side validation), `internal/plugins/archive_cache.go` (zip layout and checksum), `internal/plugins/catalog_service.go` (API version and platform gate), `internal/api/router.go` (`/api/v1/admin/plugins/...` routes)
