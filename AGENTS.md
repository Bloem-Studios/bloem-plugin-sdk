# Agent notes

## Server API: Silo v1 is retired

Silo's `/api/v1` is retired for Bloem. Never call it or document it, whether in plugin code,
scripts, tests or docs. Use Silo's `/api/v2` or Bloem's own `/api/bloem/v1`.

- A plugin talks to the server through the runtime host (gRPC), not the HTTP API.
- A plugin's own web pages are served under `/api/v2/plugin-content/plugins/<installation id>`;
  read the base from the host's `plugin_proxy_base_url` instead of building it.
- Admin examples use `/api/v2/admin/plugins/...` (upload, installations, config, update).
- An `/api/v1/...` path that belongs to another service the plugin talks to (TMDB, Seerr,
  Floppy, Pastime, Bookwarehouse, Sportarr and so on) is that service's own API and is fine.
