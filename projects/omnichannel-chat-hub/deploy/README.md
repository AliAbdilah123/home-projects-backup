# Deployment notes (Oracle Linux + nginx + SELinux)

This project deploys at:
- Frontend: http://168.110.196.49/projects/omnichannel-chat-hub/
- API (project prefix): http://168.110.196.49/projects/omnichannel-chat-hub/api/v1/health

## 1) Build artifacts

From repo root:

```bash
make test
make build
```

## 2) Environment files

Create real env files from examples:

```bash
cp deploy/env/backend.env.example deploy/env/backend.env
cp deploy/env/worker.env.example deploy/env/worker.env
```

Set at least:
- `INTERNAL_WEBHOOK_TOKEN` in `deploy/env/backend.env`
- `WORKER_INTERNAL_SECRET` in `deploy/env/worker.env` (must match)

## 3) Runtime scripts

- API: `deploy/scripts/run-backend.sh`
- Worker: `deploy/scripts/run-worker.sh`
- Frontend deploy: `deploy/scripts/deploy-frontend.sh`

Make scripts executable:

```bash
chmod +x deploy/scripts/*.sh
```

## 4) nginx + SELinux

Use the `location` blocks from `deploy/nginx/omnichannel-chat-hub.conf` inside your existing default `server` block on port 80.

Deploy frontend files into nginx webroot (recommended under SELinux Enforcing):

```bash
./deploy/scripts/deploy-frontend.sh
```

If nginx proxies to backend on localhost, enable network connect:

```bash
sudo setsebool -P httpd_can_network_connect on
```

Validate + reload nginx:

```bash
sudo nginx -t
sudo nginx -s reload
```

## 5) Optional systemd services

Unit templates:
- `deploy/systemd/omnichannel-api.service`
- `deploy/systemd/omnichannel-worker.service`

Install:

```bash
sudo cp deploy/systemd/omnichannel-api.service /etc/systemd/system/
sudo cp deploy/systemd/omnichannel-worker.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now omnichannel-api omnichannel-worker
```

## 6) Verification checklist

```bash
# backend direct
curl -i http://127.0.0.1:8080/api/v1/health

# nginx api (project path)
curl -i http://127.0.0.1/projects/omnichannel-chat-hub/api/v1/health

# nginx frontend
curl -i http://127.0.0.1/projects/omnichannel-chat-hub/

# public frontend
curl -i http://168.110.196.49/projects/omnichannel-chat-hub/
```

Expect HTTP 200 for all checks.
