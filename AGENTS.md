# Agent notes

## Server API

Use Silo's `/api/v2` and Bloem's own `/api/bloem/v2` routes, in plugin code, scripts, tests and docs.

- A plugin talks to the server through the runtime host (gRPC), not the HTTP API.
- A plugin's own web pages are served under `/api/v2/plugin-content/plugins/<installation id>`;
  read the base from the host's `plugin_proxy_base_url` instead of building it.
- Admin examples use `/api/v2/admin/plugins/...` (upload, installations, config, update).
- An API path that belongs to another service the plugin talks to (TMDB, Seerr, Floppy,
  Pastime, Bookwarehouse, Sportarr and so on) is that service's own API and is fine.
