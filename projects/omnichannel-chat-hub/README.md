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
make e2e
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

Run the worker in documented mock mode (no real WhatsApp account required):

```bash
cd worker
WORKER_MODE=mock \
WORKER_SESSION_ID=local-dev \
BAILEYS_AUTH_DIR=../data/baileys-auth \
WORKER_EXIT_AFTER_CONNECTED=true \
WORKER_MOCK_DELAY_MS=25 \
WORKER_MOCK_QR=mock-whatsapp-qr-code \
  npm run dev
```

Mock mode emits normalized JSON status/QR events to stdout and writes session state under `BAILEYS_AUTH_DIR/<session-id>/`, which is ignored by git. Set `WORKER_API_BASE_URL` and `WORKER_INTERNAL_SECRET` to also POST each event to `POST /api/v1/webhooks/whatsapp-baileys/internal` with `Authorization: Bearer <secret>`.

Run a real Baileys pairing session:

```bash
cd worker
WORKER_MODE=baileys \
WORKER_SESSION_ID=primary \
BAILEYS_AUTH_DIR=../data/baileys-auth \
WORKER_API_BASE_URL=http://127.0.0.1:8080 \
WORKER_INTERNAL_SECRET=dev-secret \
  npm run dev
```

Real mode uses Baileys multi-file auth state in `BAILEYS_AUTH_DIR/<session-id>/`, emits QR/status updates, and automatically reconnects unless WhatsApp reports the session is logged out.


## E2E MVP verification

Run the end-to-end MVP smoke suite with one command:

```bash
make e2e
```

The suite runs in-process against a temporary SQLite database and a fake Baileys worker HTTP server. It verifies login, dev inbound WhatsApp injection, inbox/timeline display data, assignment/status updates, outbound reply delivery through the worker mock, and mocked Baileys QR session status polling. Failures include the HTTP method/path, status code, and response body so the broken step is actionable.

## API

- `GET /api/v1/health` returns backend and SQLite connectivity status.
- `POST /api/v1/webhooks/dev/inbound` injects a local inbound test message and creates normalized records (`channels`, `contacts`, `conversations`, `messages`).
  - Disabled by default; enable only for local/dev via `ENABLE_DEV_WEBHOOKS=true`.
  - Requires authenticated admin/owner bearer token.
  - `provider` defaults to `dev`; use `whatsapp_baileys` with a Baileys session id in `channel_external_id` when you need to test outbound reply wiring against the mock worker.
  - Minimum payload:

```json
{
  "provider": "dev",
  "channel_external_id": "dev-wa-1",
  "contact_external_id": "15551234567",
  "message_external_id": "msg-1",
  "body": "hello from local dev"
}
```

## Notes

- WhatsApp MVP integration must use Baileys, not WhatsApp Business Cloud API.
- Keep future APIs under `/api/v1/*`.
- Deployment/runbook for production-ish nginx exposure is documented in `deploy/README.md`.
