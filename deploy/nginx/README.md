# Separated frontend / API proxy

This directory is the official reverse-proxy reference for the default
separated same-origin topology. Nginx owns the public listener; the API remains
inside the Compose network or on a loopback-only host port.

## Same-origin compose (recommended)

Nginx serves the static console and proxies control-plane plus gateway traffic
to the API container. The browser talks to one origin, so CORS is not required.

```bash
# From deploy/, after copying .env.example → .env
docker compose -f docker-compose.yml up -d
```

Access: `http://localhost:8080`. Pin `BACKEND_IMAGE` and `FRONTEND_IMAGE`
separately. Updating only `FRONTEND_IMAGE` recreates only `sub2api-web`; updating
only `BACKEND_IMAGE` recreates only `sub2api`.

Host releases use immutable directories under `/opt/sub2api/frontend/releases`
and atomically switch `/opt/sub2api/frontend/current`. See
`../SAFE_BLUE_GREEN_DEPLOY_CN.md`; a frontend-only switch never restarts API.

## Split-origin

Host the same frontend zip on any static server. Point the API `CORS_ALLOWED_ORIGINS`
at that console origin (comma-separated if several). Gateway clients can keep
calling the API origin directly.

After boot the console fetches `GET /api/v1/public/config` and `GET /api/v1/public/version`. Runtime and deployment preflight check both version floors.

## What this config does

- `underscores_in_headers on` so OpenAI-style `x-stainless-*` headers survive
- WebSocket `Upgrade` / `Connection` mapping
- `proxy_buffering off` for SSE and other streamed gateway responses
- SPA fallback to `index.html`; hashed `/assets/` cached as immutable

The route-contract test is `python3 -m unittest deploy/tests/test_nginx_route_contract.py`.
Run `nginx -t` (or the frontend image health check) before any traffic switch.
