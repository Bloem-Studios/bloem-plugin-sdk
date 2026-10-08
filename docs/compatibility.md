# Compatibility and Versioning

## Scope

`bloem-plugin-sdk` is the public Bloem build-time contract for Go plugin
authors. Its plugins target Bloem and compatible official Silo servers through
the preserved v1 wire contract.

This repository is public and released as a semver-governed Go module. Bloem
plugins and first-party consumers should depend on tagged releases, not on
sibling repo checkouts or workspace-only overrides.

The compatibility boundary includes:

- public protobuf messages and gRPC services under `pkg/pluginproto/silo/plugin/v1`
- independent Bloem storage messages and service under `pkg/pluginproto/bloem/plugin/v1`
- runtime bootstrap behavior in `pkg/pluginsdk/runtime`
- manifest helpers in `pkg/pluginsdk/manifest`
- config validation helpers in `pkg/pluginsdk/config`
- generic capability metadata conversion helpers in `pkg/pluginsdk/convert`
- canonical image-variant strings in `pkg/pluginsdk/imagevariant`

`WatchSyncAuthenticatedContext.connection_settings` (Silo SDK v0.24.0, Bloem
SDK v0.26.0) is a map, so it has no presence: a host that predates it sends an
empty map. A plugin must treat a missing key as the setting's
default rather than as an error. A host skips a declared setting whose type it
does not know, including one an older SDK decoded as `UNSPECIFIED`, and sends no
value for it.

## Release lineage

Bloem `v0.16.1` contains the compatible upstream Silo `v0.16.1` public contract.
Bloem `v0.24.0` carries the public contract and helper/runtime changes through
Silo `v0.23.0` (`96074c2c57bb726ce4cedcb2cb70434a5b1b9667`), plus Bloem's separate
native-storage service. Bloem `v0.25.0` adds ebook metadata, cover entries and a
change feed to native-storage listings. Bloem `v0.26.0` carries the public
contract through Silo `v0.24.0` (`f9827a3a7efe15d2eb6612615508a9be335aa38f`):
watch-sync connection settings and the documented scan-source contract with its
`hello-scan-source` example. The version numbers identify different modules; a Bloem
host or plugin does not need matching module versions merely to share the wire.

The current Bloem Server pins `github.com/Silo-Server/silo-plugin-sdk v0.24.0`
for public plugin capabilities. Its native-storage protocol is owned in the
server repository, under the same `bloem.plugin.v1` wire namespace; the SDK's
storage proto is byte-identical to that contract. There is no server dependency
on a sibling SDK checkout. Do not link both SDK modules into one Go process:
they register the same public protobuf descriptor names. Plugin and host
executables may use different module paths and communicate over gRPC.

The `v0.24.0` additions include auth account checks/network identity, a
`WithConfigure` callback, request seasons/progress/wording, watch ratings/series,
dropped shows and page warnings. Bloem module paths, attribution, examples,
wire identity and released struct layouts remain preserved. New service options
compose with `WithStorageProvider` on the existing connection.

The private storage service is separate from the public capability vocabulary.
"Private" describes its host admission and independent extension boundary; the
SDK repository and its Go module tags are public. See
[storage-provider.md](storage-provider.md). Bloem's bundled promotions and
ambience processes use a server-owned JSON bridge and require no SDK capability.

## Versioning Rules

- Treat the SDK as a semver boundary.
- Publish semver tags from this repository and consume those tags from downstream repos.
- Prefer additive protobuf evolution.
- Avoid renaming or removing protobuf fields, services, or enum values in `v1`.
- Keep plugin capability expansion additive: new functionality should arrive as new capability families or additive fields, not breaking rewrites of existing ones.
- First-party consumers should not merge code that depends on new SDK packages or symbols until the required SDK tag exists.

## Consumer Rules

- Plugin authors should pin released Bloem SDK tags in `go.mod`; hosts may use the compatible upstream SDK as described above.
- CI and release pipelines should build with `GOWORK=off` and without checking out this repo as a sibling source dependency.
- Local `go.work` files and temporary `replace` directives are acceptable for development, but they must not be committed as the release path.

## Open Vocabularies

Some contract fields carry an open string vocabulary rather than an enum, so
Silo can add values without a breaking protobuf change. Plugins must tolerate
values they do not recognize.

Image variants (`ResolveImageURLRequest.variant`,
`ResolveImageURLsRequest.variant`, `ResolveCatalogImageURLsRequest.variant`) are
the current example. The canonical values are exported from
`pkg/pluginsdk/imagevariant`: `card`, `featured`, `large`, `full`, `original`,
listed smallest to largest. `large` (~780px posters and stills, ~1280px logos
and backdrops) was added between `featured` and `full` once Silo gained
client-selectable image sizes; adding it is an additive change and does not
require a plugin update.

A plugin receiving an unknown variant MUST degrade gracefully to its nearest
supported size — or its default size — and MUST NOT return an error. Returning
an error turns a slightly-wrong image size into a missing image. Use a `switch`
with a `default` arm rather than an exhaustive match, and do not assume the
constants shipped in any given SDK tag are the complete set. The same rule
applies to any future open vocabulary added to `v1`.

## Presence-Sensitive Optional Fields

Some contract fields use proto3 `optional` because absence and zero have
different meanings. Current examples are `WatchSyncEvent.list_position`,
`GetImagesRequest.season_number`, and `ImageRecord.season_number`.

Consumers must check presence (`nil` pointer in Go or `HasField` in reflective
APIs), never infer it from the numeric value. Calling
`GetSeasonNumber() != 0` silently conflates "no season scope" with "Specials
(season zero) requested."

A season-scoped `GetImagesRequest` is a scope, not a guarantee. Plugins that
can filter by season should do so, and plugins should populate
`ImageRecord.season_number` whenever the season is known. Hosts must bucket and
verify images by the per-image field rather than assume a filtered response.

## Runtime Compatibility

- `silo_api_version` is the preserved coarse runtime compatibility gate between
  a Bloem or compatible official Silo host and a plugin binary.
- Host installs should reject incompatible API versions before runtime startup.
- A plugin binary should return the same manifest shape that Silo installs, except that binaries may compute their checksum dynamically at runtime.

## Go Support

The supported Bloem authoring path today is Go-only.

The protobuf and gRPC contracts are the long-term compatibility source of truth, but non-Go authoring is not an official support target in this release.

## Self-Describing Binary Guidance

If a plugin should be installable by direct binary upload:

- embed a manifest template in the binary
- compute the executable checksum at runtime
- return that populated manifest from `Runtime.GetManifest`

This keeps the plugin installable without requiring external repo state at upload time.
