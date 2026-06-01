import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { test } from "node:test";
import { createServer, type IncomingMessage } from "node:http";

import { configFromEnv, sessionAuthDir } from "./config.js";
import { HttpEventSink, MockBaileysSession } from "./service.js";
import type { WorkerEvent } from "./events.js";

async function collectBody(req: IncomingMessage): Promise<unknown> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) {
    chunks.push(Buffer.from(chunk));
  }
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

test("configFromEnv exposes safe defaults and explicit mock controls", () => {
  const config = configFromEnv({
    WORKER_MODE: "mock",
    WORKER_SESSION_ID: "local-dev",
    BAILEYS_AUTH_DIR: "/tmp/och-auth",
    WORKER_API_BASE_URL: "http://127.0.0.1:8080",
    WORKER_INTERNAL_SECRET: "dev-secret",
    WORKER_EXIT_AFTER_CONNECTED: "true",
  });

  assert.equal(config.mode, "mock");
  assert.equal(config.sessionId, "local-dev");
  assert.equal(config.authDir, "/tmp/och-auth");
  assert.equal(config.apiBaseUrl, "http://127.0.0.1:8080");
  assert.equal(config.internalSecret, "dev-secret");
  assert.equal(config.exitAfterConnected, true);
  assert.equal(
    config.internalWebhookPath,
    "/api/v1/webhooks/whatsapp-baileys/internal",
  );
});

test("sessionAuthDir creates a per-session auth directory under the configured base path", async () => {
  const authBase = await mkdtemp(join(tmpdir(), "och-auth-"));
  try {
    const dir = sessionAuthDir({
      ...configFromEnv({
        BAILEYS_AUTH_DIR: authBase,
        WORKER_SESSION_ID: "../bad session",
      }),
      mode: "mock",
    });

    assert.equal(dir, resolve(authBase, "bad-session"));
    assert.ok(dir.startsWith(resolve(authBase)));
  } finally {
    await rm(authBase, { recursive: true, force: true });
  }
});

test("mock session emits starting, qr, and connected events and persists session state outside git", async () => {
  const authBase = await mkdtemp(join(tmpdir(), "och-auth-"));
  const events: WorkerEvent[] = [];
  try {
    const config = configFromEnv({
      WORKER_MODE: "mock",
      WORKER_SESSION_ID: "test-session",
      BAILEYS_AUTH_DIR: authBase,
      WORKER_MOCK_DELAY_MS: "1",
      WORKER_MOCK_QR: "mock-qr-for-test",
    });

    const session = new MockBaileysSession(config, async (event) => {
      events.push(event);
    });
    await session.start();

    assert.deepEqual(
      events.map((event) => event.type),
      ["status", "qr", "status"],
    );
    assert.equal(events[0].status, "starting");
    assert.equal(events[1].type, "qr");
    assert.equal(events[1].qr, "mock-qr-for-test");
    assert.equal(events[2].status, "connected");

    const statePath = join(sessionAuthDir(config), "mock-session.json");
    assert.equal((await stat(statePath)).isFile(), true);
    const state = JSON.parse(await readFile(statePath, "utf8"));
    assert.equal(state.sessionId, "test-session");
    assert.equal(state.status, "connected");
  } finally {
    await rm(authBase, { recursive: true, force: true });
  }
});

test("HttpEventSink posts normalized worker events to the Go internal webhook with bearer auth", async () => {
  const received: { url?: string; auth?: string; body?: unknown }[] = [];
  const server = createServer(async (req, res) => {
    received.push({
      url: req.url,
      auth: req.headers.authorization,
      body: await collectBody(req),
    });
    res.writeHead(202, { "content-type": "application/json" });
    res.end('{"ok":true}');
  });

  await new Promise<void>((resolveReady) =>
    server.listen(0, "127.0.0.1", resolveReady),
  );
  try {
    const address = server.address();
    assert.equal(typeof address, "object");
    assert.ok(address && "port" in address);

    const sink = new HttpEventSink(
      configFromEnv({
        WORKER_API_BASE_URL: `http://127.0.0.1:${address.port}`,
        WORKER_INTERNAL_SECRET: "top-secret",
        WORKER_SESSION_ID: "sink-test",
      }),
    );

    await sink.publish({
      type: "status",
      provider: "whatsapp_baileys",
      sessionId: "sink-test",
      status: "connected",
      at: "2026-01-01T00:00:00.000Z",
    });

    assert.equal(received.length, 1);
    assert.equal(received[0].url, "/api/v1/webhooks/whatsapp-baileys/internal");
    assert.equal(received[0].auth, "Bearer top-secret");
    assert.deepEqual(received[0].body, {
      type: "status",
      provider: "whatsapp_baileys",
      session_id: "sink-test",
      status: "connected",
      occurred_at: "2026-01-01T00:00:00.000Z",
    });
  } finally {
    await new Promise<void>((resolveClose) =>
      server.close(() => resolveClose()),
    );
  }
});
