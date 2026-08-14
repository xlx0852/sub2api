# Separated frontend / API proxy

This directory is the official reverse-proxy reference for the optional
separated topology. Default one-container compose, `install.sh`, and admin
self-update are unchanged.

## Same-origin compose (recommended)

Nginx serves the static console and proxies control-plane plus gateway traffic
to the API container. The browser talks to one origin, so CORS is not required.

```bash
# From deploy/, after copying .env.example → .env
mkdir -p frontend-dist
# Unpack the GitHub Release asset sub2api-frontend-<version>.zip into frontend-dist/
docker compose -f docker-compose.yml -f docker-compose.separated.yml up -d
```

Access: `http://localhost:8080` (same port as the default stack).

`FRONTEND_DIST` can point at another directory that already contains `index.html`.

## Split-origin

Host the same frontend zip on any static server. Point the API `CORS_ALLOWED_ORIGINS`
at that console origin (comma-separated if several). Gateway clients can keep
calling the API origin directly.

After boot the console fetches `GET /api/v1/public/config` and `GET /api/v1/public/version`. If the unpacked zip is older than `min_frontend_version`, it shows a warning banner.

## What this config does

- `underscores_in_headers on` so OpenAI-style `x-stainless-*` headers survive
- WebSocket `Upgrade` / `Connection` mapping
- `proxy_buffering off` for SSE and other streamed gateway responses
- SPA fallback to `index.html`; hashed `/assets/` cached as immutable
