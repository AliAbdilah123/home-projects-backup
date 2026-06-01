# Omnichannel Chat Hub

Monorepo scaffold for the approved Omnichannel Chat Hub MVP.

Tech stack:
- Frontend: React + TypeScript + Vite
- Backend: Go HTTP API with SQLite
- WhatsApp worker: Node + TypeScript using Baileys (`@whiskeysockets/baileys`)

This initial scaffold intentionally implements only health checks and package boundaries. Feature work should build on the API contract/PRD, not replace the approved stack.

## Layout

```text
backend/     Go API server, migrations, tests
frontend/    Vite React client
worker/      Node/TypeScript Baileys worker boundary
scripts/     Helper scripts
```

## Prerequisites

- Go 1.24+ (verified with Go 1.25)
- Node 22+ and npm 10+
- SQLite is used through the pure-Go `modernc.org/sqlite` driver; no system SQLite dev package is required.

## Quick start

```bash
cp .env.example .env
make install
make test
make build
```

Run the backend:

```bash
make backend-run
curl http://127.0.0.1:8080/api/v1/health
```

Run the frontend dev server:

```bash
make frontend-dev
```

Run the worker skeleton:

```bash
make worker-dev
```

## API

- `GET /api/v1/health` returns backend and SQLite connectivity status.

## Notes

- WhatsApp MVP integration must use Baileys, not WhatsApp Business Cloud API.
- Keep future APIs under `/api/v1/*`.
- The backend serves only API health in this scaffold; production frontend/nginx wiring can be added in a separate deployment card.
