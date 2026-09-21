---
title: Bloem Plugin SDK User Guide
description: Writing a Bloem server plugin in Go from an empty directory to an installed package — the manifest, settings forms, every capability interface with its signatures, calling back into the server, testing, packaging and sideloading.
summary: The developer's handbook for the Bloem Plugin SDK, written for someone who has never built a plugin before.
tags:
  - developer
  - sdk
  - plugins
  - api-reference
  - getting-started
audience:
  - developer
last_reviewed: 2026-09-05
related:
  - admin-guide.md
  - runtime-host.md
  - compatibility.md
---

# Bloem Plugin SDK User Guide

A Bloem plugin is a small Go program that the Bloem server starts and talks to. It can supply
metadata for films and series, run scheduled jobs, react to events, serve web pages, log people in,
route media requests to a downloader, tell the server which folders changed, or sync watch history
with an outside service. This guide takes you from an empty directory to a plugin installed on a
server, and then serves as the reference for every interface the SDK exposes.

You need Go 1.26, Git, and a Bloem server you are allowed to install plugins on. You do not need to
know gRPC or protobuf; the SDK hides both, and the few terms that leak through are defined in the
[glossary](#glossary).

**How this guide is organised.** Sections 1–3 build a working plugin. Sections 4–5 explain the
manifest and settings in full. Sections 6–9 are the reference: every capability, the host client,
and the helper packages. Sections 10–12 cover testing, packaging and versioning.

---

## 1. How a plugin works

Three facts shape everything below.

1. **A plugin is a separate process.** The server launches your binary, checks a handshake, and
   opens a gRPC connection to it. Your code runs in its own process with the server's OS
   permissions. When the installation is disabled, the process is killed.
2. **A plugin describes itself with a manifest.** `manifest.json` names the plugin, lists the
   capabilities it serves, and declares the settings an administrator can fill in. The server reads
   the manifest before it runs anything.
3. **Calls go both ways.** The server calls your capability methods (for example `Search`). Your code
   can call the server back through the *runtime host* client (for example to publish an event or
   list libraries).

One naming note. The Go import path of the SDK is `github.com/Bloem-Studios/bloem-plugin-sdk`, the
protobuf package is `silo.plugin.v1`, and the manifest's API-version field is `silo_api_version`.
These spellings predate the Bloem name and are frozen because servers and plugins already speak them.
Type them exactly as shown.

---

## 2. Your first plugin

### 2.1 Create the module and add the SDK

```sh
mkdir hello-plugin && cd hello-plugin
go mod init example.com/hello-plugin
go get github.com/Bloem-Studios/bloem-plugin-sdk@v0.13.3
```

Pin a released tag (`v0.13.3` is the first verified release; use the newest one your server
supports). The repository is private, so your Git credentials must be able to read it; set
`GOPRIVATE=github.com/Bloem-Studios` if Go tries the public proxy.

### 2.2 Write the manifest

Create `manifest.json`:

```json
{
  "plugin_id": "example.hello",
  "version": "0.1.0",
  "checksum": "__CHECKSUM__",
  "silo_api_version": "v1",
  "supported_platforms": [
    { "os": "linux", "arch": "amd64" }
  ],
  "capabilities": [
    {
      "type": "scheduled_task.v1",
      "id": "hello",
      "display_name": "Hello Task",
      "description": "Says hello each time the server runs it."
    }
  ]
}
```

`checksum` is a placeholder; the SDK replaces it at run time with the SHA-256 of the running binary.
`supported_platforms` is not required by the SDK's validator but **is** required by the server, so
always include it.

### 2.3 Write `main.go`

```go
package main

import (
    "context"
    _ "embed"

    pluginv1 "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
    sdkruntime "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginsdk/runtime"
)

// version is set at build time with -ldflags "-X main.version=1.2.3".
var version = "dev"

//go:embed manifest.json
var manifestJSON []byte

type helloTask struct {
    pluginv1.UnimplementedScheduledTaskServer
}

func (helloTask) Run(ctx context.Context, req *pluginv1.RunScheduledTaskRequest) (*pluginv1.RunScheduledTaskResponse, error) {
    if host := sdkruntime.Host(); host != nil {
        _ = host.PublishEvent(ctx, "hello", map[string]any{"task": req.GetTaskKey()})
    }
    return &pluginv1.RunScheduledTaskResponse{}, nil
}

func main() {
    sdkruntime.ServeManifest(manifestJSON, version, sdkruntime.CapabilityServers{
        ScheduledTask: helloTask{},
    })
}
```

`ServeManifest` does four things: loads and validates the embedded manifest, overrides its `version`
with the string you pass (if non-empty), stamps the checksum, and serves a default `Runtime`
service alongside the capability servers you supply. It never returns. If the manifest is invalid it
panics with the validation error, which is the behaviour you want from a misbuilt plugin.

### 2.4 Build and inspect

```sh
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.version=0.1.0" -o plugin .
./plugin manifest        # on a Linux box, or build natively first to try it
```

`manifest` is the one subcommand the SDK adds to every plugin. It prints the manifest as indented
JSON — with the real checksum — and exits 0. Running the binary with no arguments outside a server
prints a go-plugin notice and exits; that is expected (see the [Admin Guide](admin-guide.md#53-the-process-model)).

### 2.5 Package and install

```sh
./plugin manifest > manifest.json      # manifest with the real checksum
zip plugin.zip manifest.json plugin    # both entries at the zip root
curl -X POST "https://bloem.example/api/v1/admin/plugins/uploads" \
  -H "Authorization: Bearer $BLOEM_ADMIN_TOKEN" \
  -F archive=@plugin.zip
```

The server verifies the checksum, unpacks the package, starts the process and calls `Configure`. The
plugin then appears in **Admin → Plugins**. Section 11 covers packaging in more detail, including
assets and what each rejection message means.

---

## 3. The two ways to serve

The SDK offers a short path and a long path. Both end in the same gRPC server.

### 3.1 `ServeManifest` — the short path

```go
func ServeManifest(manifestBytes []byte, version string, servers CapabilityServers)
func ServeManifestWithOptions(manifestBytes []byte, version string, servers CapabilityServers, options ...ServeManifestOption)
```

Use it when your `Runtime` needs no custom behaviour. The built-in runtime answers `GetManifest`
with the loaded manifest, treats `Configure` as a no-op, and wires `BindHostBroker` so
`runtime.Host()` works. `ServeManifestWithOptions` accepts `WithWatchSyncDeviceAuthorization(server)`
for watch-sync plugins that use device-code login (see 6.10).

### 3.2 `Serve` — the long path

Use it when you need to react to `Configure` (most plugins with settings do) or build the manifest in
code.

```go
type runtimeServer struct {
    runtimedefault.Server                // supplies BindHostBroker
    manifest *pluginv1.PluginManifest
    cfg      atomic.Pointer[settings]
}

func (s *runtimeServer) GetManifest(context.Context, *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
    return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

func (s *runtimeServer) Configure(_ context.Context, req *pluginv1.ConfigureRequest) (*pluginv1.ConfigureResponse, error) {
    for _, entry := range req.GetConfig() {
        if entry.GetKey() == "connection" {
            values := entry.GetValue().AsMap()   // map[string]any
            s.cfg.Store(&settings{apiKey: values["api_key"].(string)})
        }
    }
    return &pluginv1.ConfigureResponse{}, nil
}

func main() {
    m, err := manifest.LoadWithChecksum(manifestJSON, version)
    if err != nil { panic(err) }
    sdkruntime.Serve(sdkruntime.ServeConfig{
        Logger: hclog.New(&hclog.LoggerOptions{Name: "hello"}),
        Servers: sdkruntime.CapabilityServers{
            Runtime:          &runtimeServer{manifest: m},
            MetadataProvider: &myProvider{},
        },
    })
}
```

Rules for the long path:

- `Servers.Runtime` is mandatory; `Serve` exits with `runtime server is required` if it is nil.
- Embed `runtimedefault.Server` (or implement `BindHostBroker` yourself by calling
  `runtime.SetHostBrokerID`). Without it `runtime.Host()` is always `nil`.
- `ServeConfig.Plugins` may be left empty; the SDK fills in `DefaultPluginSet(servers)`. Set it to
  `DefaultPluginSetWithWatchSyncDeviceAuthorization(servers, deviceAuth)` for device-code watch sync.
- `ServeConfig.Logger` is an `hclog.Logger`; its output reaches the server log.

### 3.3 `CapabilityServers`

One field per capability service. Leave a field nil to skip registration.

| Field | Interface | Manifest type |
|---|---|---|
| `Runtime` | `pluginv1.RuntimeServer` | (always) |
| `MetadataProvider` | `pluginv1.MetadataProviderServer` | `metadata_provider.v1` |
| `ImageResolver` | `pluginv1.ImageResolverServer` | `image_resolver.v1` |
| `MarkerProvider` | `pluginv1.MarkerProviderServer` | `marker_provider.v1` |
| `MediaAnalyzer` | `pluginv1.MediaAnalyzerServer` | `media_analyzer.v1` |
| `ScheduledTask` | `pluginv1.ScheduledTaskServer` | `scheduled_task.v1` |
| `ScanSource` | `pluginv1.ScanSourceServer` | `scan_source.v1` |
| `RequestRouter` | `pluginv1.RequestRouterServer` | `request_router.v1` |
| `EventConsumer` | `pluginv1.EventConsumerServer` | `event_consumer.v1` |
| `AuthProvider` | `pluginv1.AuthProviderServer` | `auth_provider.v1` |
| `HttpRoutes` | `pluginv1.HttpRoutesServer` | `http_routes.v1` |
| `WatchSyncProvider` | `pluginv1.WatchSyncProviderServer` | `watch_sync_provider.v1` |

Always construct it with field names, never positionally. The generated code was built with
`require_unimplemented_servers=false`, so embedding `pluginv1.Unimplemented…Server` is optional, but
it is the easy way to satisfy an interface while you implement methods one at a time.

---

## 4. The manifest, field by field

`manifest.json` is the protojson encoding of `PluginManifest`. Unknown keys are ignored when loaded
(`DiscardUnknown`), so a stray field will not break anything but will not do anything either.

### 4.1 `PluginManifest`

| Field | Type | Required | Meaning |
|---|---|---|---|
| `plugin_id` | string | SDK | Stable identity, e.g. `example.hello`. Used to address events and to group installations. `silo.builtin` is reserved by the server. |
| `version` | string | SDK | Your plugin's version. `ServeManifest`/`LoadWithChecksum` overwrite it with the `version` argument when that is non-empty. |
| `checksum` | string | server | Hex SHA-256 of the executable. Filled in at run time by `LoadWithChecksum`; the placeholder `__CHECKSUM__` is conventional in source. |
| `silo_api_version` | string | server | `"v1"`. The server rejects any other value. |
| `supported_platforms` | `[{os, arch}]` | server | Go `GOOS`/`GOARCH` pairs the binary runs on. |
| `capabilities` | `[CapabilityDescriptor]` | server (≥1) | What the plugin serves (4.2). |
| `global_config_schema` | `[ConfigSchema]` | no | Server-wide settings an administrator fills in (5). |
| `user_config_schema` | `[ConfigSchema]` | no | Per-user settings (5). |
| `http_routes` | `[HttpRouteDescriptor]` | no | Routes served by `http_routes.v1` (6.3). |
| `assets` | `[PackagedAsset]` | no | Static files shipped in the zip (6.3). |
| `metadata` | object | no | Free-form JSON. |
| `category` | string | no | Slash-separated path grouping the plugin in the admin sidebar, e.g. `Books/Audiobooks`. Unset renders under "Other". |
| `presentation` | `PluginPresentation` | no (catalog: yes) | Operator-facing copy and links (4.3). |

### 4.2 `CapabilityDescriptor`

| Field | Type | Meaning |
|---|---|---|
| `type` | string | One of the thirteen known types (see `capability.KnownTypes`). Unknown → `unknown type` validation error. |
| `id` | string | Unique within the plugin for that type. Shown to administrators and echoed back in requests such as `PollChanges.capability_id`. |
| `display_name`, `description` | string | Admin UI copy. |
| `subscriptions` | `[string]` | Event names an `event_consumer.v1` wants (6.2). |
| `config_schema` | `[ConfigSchema]` | Settings scoped to this capability. |
| `metadata` | object | Free-form; readable by peers via `runtimehost.CapabilityMetadata`. |
| `auth_modes` | `[string]` | `auth_provider.v1` only: `"password"`, `"oauth2"`. Defaults to `["password"]` server-side. |
| `icon_url` | string | `auth_provider.v1` only: plugin-served logo path for the login button. |
| `watch_sync_provider` | `WatchSyncProviderDescriptor` | `watch_sync_provider.v1` only, and required for it (6.10). Setting it on any other type is a validation error. |

### 4.3 `presentation`

Optional in general; a curated catalog requires every field (`manifest.ValidateCatalogPresentation`).
Limits enforced by `manifest.Validate`:

| Field | Limit |
|---|---|
| `display_name` | 120 characters, no leading/trailing whitespace, no control characters |
| `summary` | 240 characters, same rules |
| `publisher_name`, `license_spdx` | 120 characters, same rules |
| `description_markdown`, `setup_markdown` | 32 KiB each; newlines and tabs allowed, other control characters not |
| `homepage_url`, `source_url`, `support_url`, `changelog_url`, `publisher_url` | absolute `http`/`https`, host present, no embedded credentials, no whitespace, 2048 bytes |

`ValidateCatalogPresentation(manifest, repoURL)` additionally requires `source_url` to equal the
repository URL (case-insensitive, trailing slash ignored). Use `NOASSERTION` for `license_spdx` when
no licence is declared.

### 4.4 Loading and validating in code

| Function | Use |
|---|---|
| `manifest.Load([]byte) (*PluginManifest, error)` | Decode and validate. |
| `manifest.MustLoad([]byte)` | Same; panics on error. |
| `manifest.LoadFromDisk(path)` | Read a file, then `Load`. |
| `manifest.LoadWithChecksum(embedded []byte, version string)` | `Load`, override `version`, stamp `checksum` from `os.Executable()`. |
| `manifest.Validate(*PluginManifest) error` | Validate a manifest you built in code. |
| `manifest.RegisterHTTPRoutes(m, routes...)` / `manifest.RegisterAssets(m, assets...)` | Validate, then append routes or assets. |
| `manifest.Asset(path, fs.FS) (*PackagedAsset, error)` | Build an asset entry after checking the file exists in an embedded filesystem. |

---

## 5. Declaring settings

Settings are declared with `ConfigSchema` entries. Where you put one decides who edits it:

- `global_config_schema` — one value per server, edited by administrators, delivered to your
  `Runtime.Configure`.
- `user_config_schema` — one value per user.
- `capabilities[].config_schema` — scoped to that capability instance.

### 5.1 `ConfigSchema`

| Field | Meaning |
|---|---|
| `key` | The setting's name, e.g. `connection`. The value delivered to `Configure` is keyed by it. |
| `title`, `description` | Form copy. |
| `json_schema` | A JSON Schema (draft 2020-12) **as a string**. Values are validated against it. Empty means "anything". |
| `required` | Whether the administrator must fill it in. |
| `admin_form` | Optional typed form layout (5.2). Without it the server renders from the JSON Schema alone. |

### 5.2 `admin_form`

`AdminFormDescriptor` has `fields`, an optional `submit_label`, and optional `sections`. When
`admin_form` is present the SDK validator insists that `json_schema` describes an `object` and that
every field key exists under its `properties`.

**`AdminFormField`**

| Field | Meaning |
|---|---|
| `key` | Property name in the JSON Schema. Required, unique. |
| `label`, `description`, `placeholder` | Copy. |
| `control` | `ADMIN_FORM_CONTROL_TEXT`, `TEXTAREA`, `PASSWORD`, `NUMBER`, `SWITCH`, `SELECT`, `MULTI_SELECT` (JSON: `"ADMIN_FORM_CONTROL_TEXT"` etc.). |
| `required` | Must be filled. |
| `secret` | Stored encrypted; never echoed back in full. Use for API keys. |
| `multiline`, `rows` | Textarea sizing. |
| `default_value` | Must match the property type: boolean for `boolean`, number for `integer`/`number`, string for `string`. |
| `options` | `[{value, label, description}]`. Required for `SELECT`/`MULTI_SELECT` unless `dynamic_options` is true. |
| `dynamic_options` | The plugin supplies options at run time through `RequestRouter.ListConfigOptions` keyed by this field. |
| `show_when` | `[{field, equals: [..]}]` — show only when every named field's stringified value is in `equals`. The referenced field may be declared later. |
| `validation` | `has_min/min`, `has_max/max`, `pattern` (RE2), `min_length`, `max_length`. |
| `exclusive_group_field` | Name of another field whose value defines a group; at most one connection per group may have this field truthy. |

`MULTI_SELECT` additionally requires the property type to be `array`.

**`AdminFormSection`**: `key`, `title`, `description`, `collapsible`, `collapsed_default`,
`field_keys` (each must name a declared field), `show_when`.

### 5.3 A complete example

```json
"global_config_schema": [
  {
    "key": "connection",
    "title": "Upstream service",
    "required": true,
    "json_schema": "{\"type\":\"object\",\"required\":[\"base_url\",\"api_key\"],\"properties\":{\"base_url\":{\"type\":\"string\"},\"api_key\":{\"type\":\"string\"},\"verify_tls\":{\"type\":\"boolean\"},\"mode\":{\"type\":\"string\",\"enum\":[\"basic\",\"advanced\"]},\"retries\":{\"type\":\"integer\"}}}",
    "admin_form": {
      "submit_label": "Save connection",
      "fields": [
        { "key": "base_url", "label": "Base URL", "control": "ADMIN_FORM_CONTROL_TEXT", "required": true, "placeholder": "https://service.local" },
        { "key": "api_key", "label": "API key", "control": "ADMIN_FORM_CONTROL_PASSWORD", "required": true, "secret": true },
        { "key": "verify_tls", "label": "Verify TLS", "control": "ADMIN_FORM_CONTROL_SWITCH", "default_value": true },
        { "key": "mode", "label": "Mode", "control": "ADMIN_FORM_CONTROL_SELECT",
          "options": [ { "value": "basic", "label": "Basic" }, { "value": "advanced", "label": "Advanced" } ],
          "default_value": "basic" },
        { "key": "retries", "label": "Retries", "control": "ADMIN_FORM_CONTROL_NUMBER",
          "show_when": [ { "field": "mode", "equals": ["advanced"] } ],
          "validation": { "has_min": true, "min": 0, "has_max": true, "max": 10 } }
      ],
      "sections": [
        { "key": "basics", "title": "Connection", "field_keys": ["base_url", "api_key", "verify_tls"] },
        { "key": "tuning", "title": "Advanced", "collapsible": true, "collapsed_default": true, "field_keys": ["mode", "retries"] }
      ]
    }
  }
]
```

### 5.4 Receiving and validating values

The server sends `Runtime.Configure` at start and whenever an administrator saves, with one
`ConfigEntry{key, value}` per schema key. `value` is a `google.protobuf.Struct`; call `.AsMap()`.

To validate a value yourself (for example in a test), use the `config` package:

```go
err := config.ValidateManifestGlobalValue(m, "connection", map[string]any{"base_url": "x"})
// or: config.ValidateManifestUserValue, config.ValidateValue(schema, kind, key, value)
// config.FindSchema(schemas, key) locates a ConfigSchema by key.
```

An undeclared key fails with `… key "x" is not declared in the manifest schema`; a schema that
does not compile fails with `compile …`; a bad value fails with `validate …`.

To persist a value the administrator did not enter (a token you obtained, a cursor), call
`runtimehost.Client.SetGlobalConfigEntry(ctx, key, map[string]any)`.

---

## 6. Capability reference

Each subsection gives the manifest type, the Go interface, every RPC with its request and response
fields, what the server expects, and an example. All handlers have the shape
`func(ctx context.Context, req *pluginv1.XRequest) (*pluginv1.XResponse, error)`. Return a gRPC
error (`status.Error(codes.Unavailable, ...)`) for failures; return `codes.Unimplemented` for optional
methods you do not support.

### 6.1 `scheduled_task.v1` — `ScheduledTaskServer`

`Run(RunScheduledTaskRequest{task_key, input Struct}) → RunScheduledTaskResponse{output Struct}`

The server owns the schedule. `task_key` identifies which task fired when one plugin declares several.
Return quickly or respect `ctx` cancellation; long jobs should check `ctx.Done()`. The
`examples/hello-scheduled-task` plugin is the minimum implementation; `examples/hello-runtime-host`
publishes an event from `Run`.

### 6.2 `event_consumer.v1` — `EventConsumerServer`

`HandleEvent(HandleEventRequest{event_name, payload Struct}) → HandleEventResponse{}`

Declare the events you want in the capability's `subscriptions` list. Events published by other
plugins arrive as `plugin.<their plugin_id>.<name>`; the prefix is added by the server and cannot be
forged. Handle events idempotently — the server may redeliver.

### 6.3 `http_routes.v1` — `HttpRoutesServer`

`Handle(HandleHTTPRequest{method, path, headers map, body bytes, query Struct}) → HandleHTTPResponse{status_code, headers map, body bytes}`

The server proxies HTTP requests to your handler for routes you declared. Declare each one in
`http_routes`:

| `HttpRouteDescriptor` field | Meaning |
|---|---|
| `id` | Stable route id. |
| `method`, `path` | e.g. `GET`, `/api/status`. |
| `access` | Who may call it; the server enforces it before proxying. |
| `navigable`, `navigation_label`, `navigation_kind` | Whether and how the route appears as a page in the UI. |
| `static_asset` | The route serves a packaged file. |

Packaged files are declared in `assets` (`PackagedAsset{path, content_type, integrity}`) and must be
present in the zip at that path. Build the entries with `manifest.Asset(path, embeddedFS)` and
`manifest.RegisterAssets`. For links back to your own routes, get the base URL from
`runtimehost.Client.GetHostInfo` (`PluginProxyBaseURL`) rather than hard-coding it.

```go
func (h routes) Handle(_ context.Context, req *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
    if req.GetMethod() == "GET" && req.GetPath() == "/api/status" {
        body, _ := json.Marshal(map[string]any{"ok": true})
        return &pluginv1.HandleHTTPResponse{StatusCode: 200,
            Headers: map[string]string{"Content-Type": "application/json"}, Body: body}, nil
    }
    return &pluginv1.HandleHTTPResponse{StatusCode: 404}, nil
}
```

### 6.4 `metadata_provider.v1` — `MetadataProviderServer`

The largest interface. Implement what your source can answer and return `Unimplemented` for the rest.

| RPC | Request fields | Response |
|---|---|---|
| `Search` | `query`, `item_type`, `year`, `provider_ids` Struct, `language` | `results []ProviderSearchResult` |
| `GetMetadata` | `provider_id`, `item_type`, `provider_ids`, `language`, `file_path` | `item MetadataItem` |
| `GetPersonDetail` | `provider_ids`, `language` | `person PersonDetailRecord` |
| `GetSeasons` | `series_provider_id`, `provider_ids`, `language` | `seasons []SeasonRecord` |
| `GetEpisodes` | `series_provider_id`, `season_number`, `provider_ids`, `language` | `episodes []EpisodeRecord` |
| `GetImages` | `provider_id`, `item_type`, `provider_ids`, `language` | `images []ImageRecord{kind, url, language, width, height, metadata}` |
| `ResolveImageURL` | `path`, `variant` | `url` |
| `ResolveImageURLs` | `paths []`, `variant` | `urls map[path]url` |

`ProviderSearchResult`: `provider_id`, `item_type`, `title`, `original_title`, `year`, `overview`,
`provider_ids`, `image_url`, `title_aliases []TitleAlias{title, language, kind}`, `title_language`,
`title_is_fallback`, `original_language`.

`MetadataItem` carries everything the server can store: identity (`provider_id`, `item_type`,
`provider_ids`), text (`title`, `original_title`, `sort_title`, `tagline`, `overview`, `genres`,
`studios`, `networks`, `countries`, `content_rating`, `status`), numbers (`year`, `runtime`,
`season_count`, `ratings` Struct), dates (`release_date`, `first_air_date`, `last_air_date`,
`air_time`), images (`poster_path`, `backdrop_path`, `logo_path` plus `*_thumbhash`), `people
[]PersonRecord{name, kind, character, sort_order, tmdb_id, tvdb_id, imdb_id, plex_guid, photo_path,
photo_thumbhash}`, `videos []VideoRecord{provider_key, kind, site, site_key, name, language,
is_official, size_hint, published_at}`, `title_aliases`, `title_language`, `title_is_fallback`,
`title_aliases_complete`, and a free `metadata` Struct. `legacy_people` is deprecated; use `people`.

Rules from the contract comments: `kind`/`site`/alias `kind` are open lowercase strings so a newer
plugin can send values an older server maps to "other"; set `title_aliases_complete` only when your
alias list is authoritative, because a server merges rather than deletes when it is false.

Image paths you return (`poster_path` and friends) are opaque to the server; it calls your
`ResolveImageURL(s)` to turn them into fetchable URLs. A plugin that only resolves images for another
provider declares `image_resolver.v1` and implements `ImageResolverServer` (the last two RPCs only).

```go
func (p tmdbLike) Search(ctx context.Context, req *pluginv1.SearchMetadataRequest) (*pluginv1.SearchMetadataResponse, error) {
    hits, err := p.api.search(ctx, req.GetQuery(), req.GetItemType(), int(req.GetYear()), req.GetLanguage())
    if err != nil { return nil, status.Error(codes.Unavailable, err.Error()) }
    out := &pluginv1.SearchMetadataResponse{}
    for _, h := range hits {
        ids, _ := structpb.NewStruct(map[string]any{"tmdb": h.ID})
        out.Results = append(out.Results, &pluginv1.ProviderSearchResult{
            ProviderId: h.ID, ItemType: req.GetItemType(), Title: h.Title, Year: int32(h.Year),
            ProviderIds: ids, ImageUrl: h.Poster,
        })
    }
    return out, nil
}
```

### 6.5 `marker_provider.v1` — `MarkerProviderServer`

Supplies skip segments from an external database.

| RPC | Request | Response |
|---|---|---|
| `FetchMarkers` | `item_type` (`movie`/`episode`), `external_ids{tmdb_id, imdb_id, tvdb_id, provider_ids}`, `season_number`, `episode_number`, `duration_seconds`, `metadata` | `markers []MarkerSegment{segment, start_seconds?, end_seconds?, confidence, submission_count, algorithm, metadata}` |
| `SubmitMarker` | same identity fields + `segment`, `start_seconds?`, `end_seconds?`, `duration_seconds` | `submission_id`, `status` (`pending`/`accepted`/`rejected`), `weight` |
| `GetMarkerProviderStats` | — | `total`, `accepted`, `pending`, `rejected`, `acceptance_rate`, `current_streak`, `best_streak` |

`segment` is one of `intro`, `credits`, `recap`, `preview`. `start_seconds`/`end_seconds` are
`optional`, so leave them unset (nil) rather than zero when unknown.

### 6.6 `media_analyzer.v1` — `MediaAnalyzerServer`

`Analyze(AnalyzeMediaRequest{media_path, file_hash, duration_seconds, metadata}) →
AnalyzeMediaResponse{intro MarkerRange, credits MarkerRange, confidence, metadata}`

The server hands you a local file path; you analyse it and return `MarkerRange{start_seconds,
end_seconds}` values. Leave a range nil when not found.

### 6.7 `auth_provider.v1` — `AuthProviderServer`

| RPC | Request | Response |
|---|---|---|
| `Authenticate` | `username`, `password`, `metadata` | `AuthenticateResponse{external_subject, display_name, email, claims}` |
| `InitAuthorize` | `redirect_uri`, `state`, `linking`, `metadata` | `authorize_url`, `provider_state` Struct |
| `ExchangeCode` | `code`, `state`, `redirect_uri`, `provider_state` | `AuthenticateResponse` |
| `RefreshSession` | `external_subject`, `refresh_state` | `AuthenticateResponse` (or `Unimplemented`) |

Password-only providers implement `Authenticate` and set `auth_modes: ["password"]` (the default).
OAuth/OIDC providers set `auth_modes: ["oauth2"]`, return the redirect URL from `InitAuthorize`
(stash the PKCE verifier in `provider_state`; the server stores and returns it untouched), and
complete the login in `ExchangeCode`. `icon_url` on the capability points at a plugin-served asset
for the "Sign in with …" button. `external_subject` must be stable for the same person across logins.

### 6.8 `request_router.v1` — `RequestRouterServer`

The server owns request lifecycle, quotas and quality policy; the plugin owns talking to the
downloader. Credentials arrive per call in `RouterConnection{id, base_url, api_key, config Struct}`.

| RPC | Request | Response |
|---|---|---|
| `Fulfill` | `capability_id`, `request RequestDescriptor`, `qualities []RequestedQuality{id, is4k}`, `connections []RouterConnection` | `targets []FulfillmentTarget{quality, connection_id, external_id, external_status, status, message}`, `message` |
| `CheckStatus` | `capability_id`, `request`, `targets []TargetRef{quality, connection_id, external_id}`, `connections` | `statuses []TargetStatus{quality, connection_id, status, external_status, message}` |
| `ListConfigOptions` | `capability_id`, `connection` | `options_by_field map[field]ConfigOptionList{options[]{value,label}}` |
| `TestConnection` | `capability_id`, `connection` | `ok`, `message` |
| `Validate` | `capability_id`, `connection`, `siblings []RouterConnection` (id + config only) | `field_errors map`, `form_error` |

`RequestDescriptor`: `media_type` (`movie`/`series`), `title`, `year`, `external_ids map`,
`is_anime`, `requester_user_id`, `requester_profile_id`, `requester_email`, `requester_username`.
`status` values are host-normalised: `queued`, `downloading`, `completed`, `failed`; pass the raw
upstream value in `external_status`. `ListConfigOptions` backs `dynamic_options` form fields (5.2).
The `httpclient` package (section 8) was written for exactly this kind of plugin.

### 6.9 `scan_source.v1` — `ScanSourceServer` (Autoscan providers)

`PollChanges(PollChangesRequest{capability_id, marker, connection ResolvedConnection{base_url,
api_key}, source_config map}) → PollChangesResponse{source_paths [], next_marker, changes
[]ScanSourceChange{source_path, scope}}`

Pull only: the server owns the poll timer, marker storage, path rewrite rules and scan queueing.
Your job is to ask the upstream (a downloader's history endpoint, a filesystem watcher's journal) what
changed since `marker` and return absolute paths **in the upstream's namespace**; the server rewrites
them to its own mounts. An empty `marker` means first run — start from now, do not replay history.
`next_marker` is opaque to the server. Prefer `changes` over `source_paths`: the `scope`
(`SCAN_SOURCE_CHANGE_SCOPE_AUTO`, `FILE`, `SUBTREE`) lets the server treat deletes and renames whose
path no longer exists correctly. `connection` carries live credentials; never log the request
unredacted and never store it.

```go
func (s arrSource) PollChanges(ctx context.Context, req *pluginv1.PollChangesRequest) (*pluginv1.PollChangesResponse, error) {
    api := httpclient.New(req.GetConnection().GetBaseUrl(), req.GetConnection().GetApiKey(), nil)
    var hist struct{ Records []struct{ Path string `json:"path"`; ID int `json:"id"` } `json:"records"` }
    if err := api.GetJSON(ctx, "/api/v3/history?since="+req.GetMarker(), &hist); err != nil {
        return nil, status.Error(codes.Unavailable, err.Error())
    }
    out := &pluginv1.PollChangesResponse{NextMarker: req.GetMarker()}
    for _, r := range hist.Records {
        out.Changes = append(out.Changes, &pluginv1.ScanSourceChange{
            SourcePath: r.Path, Scope: pluginv1.ScanSourceChangeScope_SCAN_SOURCE_CHANGE_SCOPE_FILE})
        out.NextMarker = strconv.Itoa(r.ID)
    }
    return out, nil
}
```

### 6.10 `watch_sync_provider.v1` — `WatchSyncProviderServer`

Adapts an external watch-tracking service. The server owns credentials, flow state, retries and
ordering; the plugin is a stateless protocol adapter. The manifest must carry a
`watch_sync_provider` descriptor and the capability `id` must be a lowercase slug
(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`).

**Descriptor** (`WatchSyncProviderDescriptor`): `auth_methods` (≥1 of
`WATCH_SYNC_AUTH_METHOD_AUTHORIZATION_CODE`, `API_KEY`, `DEVICE_CODE`), operation flags
`export_watched`, `export_unwatched`, `import_watched`, `import_progress`, `import_favorites`,
`export_favorites`, `remove_favorites`, `import_watchlist`, `export_watchlist`, `remove_watchlist`,
`scrobble_playback` (≥1 must be true), `supported_media_types` (≥1 of `WATCH_SYNC_MEDIA_TYPE_MOVIE`,
`EPISODE`), `external_id_namespaces` (slugs), `max_batch_size` (1–100), `provides_watchlist_order`
(requires `import_watchlist`). The SDK validator enforces each of these.

**RPCs**

| RPC | Purpose |
|---|---|
| `InitAuthorize` / `ExchangeCode` | Authorization-code login. |
| `ExchangeAPIKey` | API-key login. |
| `RefreshCredentials` | Turn a refresh token into new credentials. |
| `GetAccount` | `WatchSyncAccount{external_subject, username, display_name, avatar_url, profile_url}`. |
| `ApplyEvents` | Push desired state (`WatchSyncEvent` list) upstream; return one `WatchSyncApplyResult{event_id, status, fault}` per event. |
| `ListRemoteState` | Page through the upstream's watched/progress/favorite/watchlist state. |

Every authenticated RPC receives `WatchSyncAuthenticatedContext{capability_id, provider_config
{values, secret_values}, credentials}`. Treat it as request data: never persist or log it.
`provider_config` keys are `<config key>.<field>` (e.g. `provider.client_id`); scalars are strings,
structured values JSON, and fields marked `secret` in the manifest arrive in `secret_values`.

Contract rules you must follow:

- `ApplyEvents` is at-least-once. `event_id` is stable across retries; make updates convergent
  (set state, never increment). A replayed scrobble stop must not create a second play.
- `completed` on an event is the server's authoritative watched decision; do not infer it.
- Result status: `APPLIED`/`NO_CHANGE` (no fault), `RETRY` with `TEMPORARY` or `RATE_LIMITED`
  (+ `retry_after`), `REJECTED` with `INVALID_REQUEST` or `PERMANENT`. Connection-wide faults such as
  `INVALID_CREDENTIAL` go on the response, not on a result.
- `updated_credentials` is a complete replacement; the server persists it before reading anything
  else in the response.
- `ListRemoteState`: the server fixes `cursor` for a phase and follows your page tokens; set
  `complete_snapshot=true` only when a traversal is authoritative. A removal is an item whose list
  state has `removed=true`; it may omit `media` if `provider_item_key` identifies it. When
  `provides_watchlist_order` is true, watchlist traversals must be complete snapshots in remote order.

**Device-code login** lives in a second service, `WatchSyncDeviceAuthorizationServiceServer`
(`Start`, `Poll`). Register it without touching `CapabilityServers`:

```go
sdkruntime.ServeManifestWithOptions(manifestJSON, version,
    sdkruntime.CapabilityServers{WatchSyncProvider: provider},
    sdkruntime.WithWatchSyncDeviceAuthorization(deviceAuth))
```

A `Poll` response may replace `provider_state`, the polling interval and expiry; an explicitly empty
`provider_state` clears it, omission keeps it. The user code and verification URL stay valid until
expiry.

### 6.11 `audiobook_backend.v1` and `ebook_backend.v1`

Only the type constants exist in this SDK; no gRPC service is defined for either. A manifest may
declare them (validation accepts the strings) but there is nothing to serve yet.

### 6.12 Subtitles

This SDK has no subtitle-provider capability. If you need one, it must be added as a new proto file
and capability constant in the SDK first.

---

## 7. Calling back into the server (`runtimehost`)

`sdkruntime.Host()` returns a `*runtimehost.Client` once the server has called `BindHostBroker`,
and `nil` before that or if the broker dial fails. Treat `nil` as transient: skip the call, return a
temporary error, or try again on the next invocation. The first successful call dials the broker
stream and caches the client; do not dial the broker yourself.

| Method | Signature | Notes |
|---|---|---|
| `PublishEvent` | `(ctx, name string, payload map[string]any) error` | Broadcast; server prefixes `plugin.<your id>.`. |
| `PublishEventTo` | `(ctx, targetPluginID, name string, payload) error` | To one plugin id. |
| `PublishEventToInstallation` | `(ctx, installationID int, name string, payload) error` | To one installation; id must be > 0. |
| `GetHostInfo` | `(ctx) (*HostInfo{PublicBaseURL, InternalBaseURL, PluginProxyBaseURL}, error)` | For callback URLs and links. |
| `ListLibraries` | `(ctx, userID string) ([]*pluginv1.Library{id, name, media_type}, error)` | Empty `userID` = all. |
| `CheckMediaPresence` | `(ctx, provider, mediaType string, ids []string) (map[string]MediaPresence, error)` | `provider` must be `tmdb`; `mediaType` `movie`/`tv`; ≤100 ids. |
| `ListInstalledPlugins` | `(ctx) ([]*pluginv1.InstalledPlugin, error)` | `installation_id`, `plugin_id`, `version`, `enabled`, `capabilities`. |
| `ListInstalledPluginsByCapability` | `(ctx, capabilityType string)` | Filtered form; use `capability.*` constants. |
| `SetGlobalConfigEntry` | `(ctx, key string, value map[string]any) error` | Persist plugin-owned config. |
| `ListLibraryMedia` | `(ctx, ListLibraryMediaRequest) (*ListLibraryMediaResponse, error)` | Filters: `LibraryIDs`, `MediaTypes`, `Query`, `Genre`, `YearMin/Max`, `Sort` (`title`,`year`,`added_at`,`runtime`,`rating`), `Descending`, `PageSize`, `PageToken`. Public-safe rows only. |
| `GetCatalogStats` | `(ctx, libraryIDs []string) (*CatalogStats, error)` | Totals by media type and library. |
| `ResolveCatalogImageURLs` | `(ctx, paths []string, variant string) (map[string]string, error)` | Empty variant = host default; unresolved paths omitted. |
| `MintScopedStream` | `(ctx, ScopedStreamRequest) (*ScopedStreamGrant{StreamURL, PlayMethod, ExpiresAt}, error)` | Fields: `MediaFileID`, `PlayMethod` (`direct`/`remux`/`auto`), `ExpiresAt`, `MaxWatchMinutes`, `MaxResolutionHeight`, `AllowDirectPlay`, `AllowDownloads`, `DisableSeeking`, `AuditSubject`, `Watermark*`. |
| `CallPluginHTTP` | `(ctx, CallPluginHTTPRequest{InstallationID, Method, Path, Headers, Body, Query}) (*CallPluginHTTPResponse, error)` | Raw call to a peer's `http_routes.v1`. |
| `CallPluginJSON` | `(ctx, CallPluginJSONRequest{InstallationID, Method, Path, Headers, Query, Request, Response, MaxResponseBytes}) error` | JSON convenience: sets headers, marshals `Request`, decodes into `Response`, returns `*HTTPStatusError` for status ≥ 400. Default `Method` is GET, or POST when `Request` is set. Response capped at 10 MiB by default. |

Helpers for peer discovery: `runtimehost.HasCapability(plugin, type)`, `Capability(plugin, type)`,
`CapabilityMetadata(cap)`, `CapabilityMetadataString(cap, key)`, `CapabilityMetadataStrings(cap, key)`.

```go
host := sdkruntime.Host()
if host == nil { return status.Error(codes.Unavailable, "host not bound yet") }
routers, err := host.ListInstalledPluginsByCapability(ctx, capability.RequestRouter)
if err != nil || len(routers) == 0 { return err }
var reply struct{ Accepted bool `json:"accepted"` }
err = host.CallPluginJSON(ctx, runtimehost.CallPluginJSONRequest{
    InstallationID: int(routers[0].GetInstallationId()),
    Path:           "/api/request",
    Request:        map[string]any{"title": "The Matrix"},
    Response:       &reply,
})
```

More patterns (presence badges, image URL resolution, addressing events) are in
[`docs/runtime-host.md`](runtime-host.md).

---

## 8. Outbound HTTP helper (`httpclient`)

For plugins that talk to a third-party JSON API authenticated with an `X-Api-Key` header.

```go
c := httpclient.New(baseURL, apiKey, nil)          // nil → shared client, 2-minute timeout
err := c.GetJSON(ctx, "/api/v3/system/status", &out)
err = c.PostJSON(ctx, "/api/v3/movie", body, &created)
err = c.DoJSON(ctx, http.MethodPut, "/path", body, nil)
```

Behaviour: base URL is right-trimmed of `/`, key is space-trimmed; both are required or the call
fails before sending. Any non-2xx returns `*httpclient.StatusError{StatusCode, Body, Message}`
(`Message` is the parsed `{"message": …}` field when present). Response bodies are capped at
1 MiB. An empty 2xx body decodes to the zero value with no error — check the decoded value (e.g.
`id == 0`) to detect "created but nothing returned".

---

## 9. Conversion helpers (`convert`)

Used by hosts and tooling that store capabilities as plain records:

- `convert.CapabilityRecordsFromManifest(m) ([]CapabilityRecord{Type, ID, Metadata}, error)`
- `convert.DecodeCapability(record) (*pluginv1.CapabilityDescriptor, error)`

The `Metadata` map is a compatibility boundary: `display_name`, `description`, `subscriptions`,
`auth_modes`, `icon_url`, `watch_sync_provider`, `config_schema` (with `key`, `title`,
`description`, `json_schema`, `required` always present) and `metadata`. Most plugin authors never
call these.

---

## 10. Testing

### 10.1 Unit tests without a server

Every handler is an ordinary Go method; call it directly.

```go
func TestSearch(t *testing.T) {
    resp, err := (&myProvider{api: fakeAPI{}}).Search(context.Background(),
        &pluginv1.SearchMetadataRequest{Query: "Matrix", ItemType: "movie"})
    if err != nil { t.Fatal(err) }
    if len(resp.GetResults()) != 1 { t.Fatalf("got %d results", len(resp.GetResults())) }
}
```

Validate the manifest in a test so a bad edit fails before it reaches a server:

```go
func TestManifest(t *testing.T) {
    m, err := manifest.Load(manifestJSON)
    if err != nil { t.Fatal(err) }
    if m.GetSiloApiVersion() != "v1" { t.Fatal("api version") }
    if err := config.ValidateManifestGlobalValue(m, "connection",
        map[string]any{"base_url": "https://x", "api_key": "k"}); err != nil { t.Fatal(err) }
}
```

Test the `manifest` subcommand end to end the way `cmd/compat-probe/main_test.go` does: `go build`
into `t.TempDir()`, run `<binary> manifest`, and `manifest.Load` the output.

The SDK ships no fake server. Code that needs `runtimehost` should take the client as a dependency
(an interface with the methods you use) so tests can substitute a stub.

### 10.2 Against a real server

1. Build for the server's platform (Linux/amd64 for most containers) as in 2.4.
2. Package and upload (2.5, 11). Install succeeds without a restart.
3. Watch the server log: your `hclog` output appears there, together with any handshake error.
4. Exercise the capability from the admin UI (run the task, refresh metadata, save the settings
   form, sign in) and confirm the effect.
5. To iterate, bump `version`, rebuild, re-package and upload again. The server replaces the
   installation.

`scripts/build-compat-probe.sh` in the SDK repository produces a known-good package
(`dist/compat-probe`, `.sha256`, `.manifest.json`). If the probe installs and yours does not, the
difference is in your plugin, not the transport.

---

## 11. Packaging and sideloading

The package is a zip with `manifest.json` and an executable named exactly `plugin` at the root, plus
every file listed in `assets`. A repeatable script:

```sh
#!/bin/sh
set -eu
VERSION=${1:?version}
OUT=dist/pkg
rm -rf "$OUT" && mkdir -p "$OUT"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" -o "$OUT/plugin" .
# On a Linux host the binary can write its own manifest:
#   "$OUT/plugin" manifest > "$OUT/manifest.json"
# Elsewhere, substitute the checksum and version into the source manifest:
SUM=$(shasum -a 256 "$OUT/plugin" | cut -d' ' -f1)
sed -e "s/__CHECKSUM__/$SUM/" \
    -e "s/\"version\": \"[^\"]*\"/\"version\": \"$VERSION\"/" \
    manifest.json > "$OUT/manifest.json"
cp -R assets "$OUT/" 2>/dev/null || true      # only if the manifest declares assets
( cd "$OUT" && rm -f ../plugin.zip && zip -r ../plugin.zip . )
```

Zipping from inside the output directory is what keeps `manifest.json` and `plugin` at the archive
root; the source `manifest.json` in your repository keeps its `__CHECKSUM__` placeholder.

Upload:

```sh
curl -X POST "$SERVER/api/v1/admin/plugins/uploads" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -F archive=@plugin.zip
```

The server answers `201` with the installation record. Common rejections:

| Message | Cause |
|---|---|
| `plugin archive is missing manifest.json` / `… missing plugin binary` | Wrong entry names or a top-level folder in the zip. |
| `plugin binary checksum does not match manifest` | Placeholder left in, or manifest from a different build. |
| `plugin manifest supported_platforms is required` | Add the platforms list. |
| `plugin manifest silo_api_version is required` / version mismatch | Set `"v1"`. |
| `plugin manifest capabilities are required` | Declare at least one capability. |
| `duplicate plugin capability` | Two capabilities share `type` and `id`. |
| `plugin id "silo.builtin" is reserved` | Choose another id. |
| `plugin capability "x": unknown type "y"` | Typo in `type`; use a `capability.*` constant value. |

Catalog publication is the alternative to sideloading: a catalog entry points at the package URL and
checksum, and the server offers it in **Admin → Plugins** when `silo_api_version` and
`supported_platforms` match. Catalog entries also need a complete `presentation` block (4.3).

---

## 12. Versioning your plugin and upgrading the SDK

- Your plugin's `version` is yours; the server shows it and uses it to detect updates. Set it from
  the build (`-X main.version=…`) so the manifest and binary never disagree.
- Pin an SDK tag in `go.mod`. Minor SDK releases are additive (new fields, new RPCs, new
  capabilities); upgrading is `go get …@vX.Y.Z`, rebuild, retest. A newer SDK does not require a
  newer server unless you *use* something the server lacks, in which case the RPC fails at run time
  — guard optional features accordingly.
- Never construct `CapabilityServers` positionally and never depend on `Unimplemented*Server` being
  required; both are how the SDK keeps old plugins compiling.
- When the server bumps its API version, `silo_api_version` in your manifest must follow, and the
  SDK will publish a matching release; until then old plugins are refused at install time rather
  than failing mid-flight.
- `docs/compatibility.md` in the SDK is the authoritative statement of what may change in which
  release type.

---

## Glossary

- **Capability** — a family of functions a plugin can serve, named by a string such as `metadata_provider.v1`, each backed by one gRPC service.
- **Checksum** — hex SHA-256 of the `plugin` executable; must match the manifest or the install is refused.
- **Config schema** — a JSON Schema plus optional form layout that declares one setting the administrator (or user) can fill in.
- **gRPC** — the remote-call protocol between server and plugin. The generated `pluginv1` package hides its details.
- **Handshake** — the start-up check (`SILO_PLUGIN=silo-rpc-plugin-v1`) proving the binary was launched by a server.
- **Manifest** — `manifest.json`, the plugin's self-description.
- **Marker** — an opaque continuation token (`scan_source.v1`) or a skip segment (`marker_provider.v1`), depending on context.
- **Protojson** — the JSON encoding of protobuf messages used for the manifest; snake_case keys, enums as their full names.
- **Runtime host** — the server-side gRPC service plugins call back into; `sdkruntime.Host()` returns its client.
- **Sideload** — installing a plugin by uploading the zip directly to `/api/v1/admin/plugins/uploads`.
- **Struct** — `google.protobuf.Struct`, a JSON object in protobuf form; `.AsMap()` converts it, `structpb.NewStruct` builds it.
- **Unimplemented server** — the generated `pluginv1.Unimplemented…Server` type you may embed so unhandled methods return `Unimplemented`.

## Source References

- `README.md` — package list, capability list, presentation rules, watch-sync and scan-source contract notes
- `pkg/pluginsdk/runtime/runtime.go`, `serve_manifest.go` — `Serve`, `ServeManifest`, `CapabilityServers`, `Host()`, `manifest` subcommand, handshake constants
- `pkg/pluginsdk/runtimedefault/runtime.go` — embeddable `BindHostBroker`
- `pkg/pluginsdk/manifest/manifest.go`, `checksum.go` — load, validate, presentation limits, admin-form rules, `RegisterHTTPRoutes`, `RegisterAssets`, `Asset`, `LoadWithChecksum`
- `pkg/pluginsdk/config/config.go` — `ValidateManifestGlobalValue`, `ValidateManifestUserValue`, `FindSchema`, `ValidateValue`
- `pkg/pluginsdk/capability/capability.go` — type constants and `KnownTypes`
- `pkg/pluginsdk/runtimehost/client.go`, `discovery.go`, `call_json.go`, `events.go`, `host_info.go` — host client API
- `pkg/pluginsdk/httpclient/httpclient.go` — outbound JSON client
- `pkg/pluginsdk/convert/convert.go` — capability record conversion
- `proto/silo/plugin/v1/common.proto` — `PluginManifest`, `CapabilityDescriptor`, `ConfigSchema`, admin form messages, `Runtime` service, `HttpRouteDescriptor`, `PackagedAsset`
- `proto/silo/plugin/v1/scheduled_task.proto`, `event_consumer.proto`, `http_routes.proto`, `metadata_provider.proto`, `marker_provider.proto`, `media_analyzer.proto`, `auth_provider.proto`, `request_router.proto`, `scan_source.proto`, `watch_sync_provider.proto`, `runtime_host.proto` — RPC and message definitions quoted in section 6 and 7
- `examples/hello-scheduled-task/`, `examples/hello-runtime-host/`, `cmd/compat-probe/` — working plugins and the `manifest` subcommand test
- `docs/runtime-host.md`, `docs/compatibility.md`
- Bloem server `internal/plugins/manifest.go`, `archive_cache.go`, `internal/api/router.go` — server-side manifest requirements, zip layout, upload routes
