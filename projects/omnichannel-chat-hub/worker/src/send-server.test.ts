import assert from "node:assert/strict";
import { test } from "node:test";
import { createServer } from "node:http";

import { configFromEnv } from "./config.js";
import { MockOutboundSender, WorkerSendServer } from "./send-server.js";

test("WorkerSendServer accepts authorized text sends and returns provider id", async () => {
  const sender = new MockOutboundSender();
  const server = new WorkerSendServer(
    configFromEnv({ WORKER_SESSION_ID: "wa-1", WORKER_INTERNAL_SECRET: "send-secret", WORKER_MODE: "mock" }),
    sender,
  );
  const httpServer = createServer(server.handler());
  await new Promise<void>((resolveReady) => httpServer.listen(0, "127.0.0.1", resolveReady));
  try {
    const address = httpServer.address();
    assert.equal(typeof address, "object");
    assert.ok(address && "port" in address);

    const response = await fetch(`http://127.0.0.1:${address.port}/v1/sessions/wa-1/messages`, {
      method: "POST",
      headers: { authorization: "Bearer send-secret", "content-type": "application/json" },
      body: JSON.stringify({ message_id: "msg_123", chat_id: "15551234567@s.whatsapp.net", type: "text", body: "hello" }),
    });

    assert.equal(response.status, 200);
    const payload = await response.json();
    assert.equal(payload.external_message_id, "mock_msg_123");
    assert.equal(payload.status, "sent");
    assert.deepEqual(sender.sent, [{ messageId: "msg_123", chatId: "15551234567@s.whatsapp.net", type: "text", body: "hello" }]);
  } finally {
    await new Promise<void>((resolveClose) => httpServer.close(() => resolveClose()));
  }
});

test("WorkerSendServer rejects invalid auth without calling the sender", async () => {
  let sendCalls = 0;
  const server = new WorkerSendServer(
    configFromEnv({ WORKER_SESSION_ID: "wa-1", WORKER_INTERNAL_SECRET: "send-secret", WORKER_MODE: "mock" }),
    {
      async sendText() {
        sendCalls++;
        throw new Error("should not send");
      },
    },
  );
  const httpServer = createServer(server.handler());
  await new Promise<void>((resolveReady) => httpServer.listen(0, "127.0.0.1", resolveReady));
  try {
    const address = httpServer.address();
    assert.equal(typeof address, "object");
    assert.ok(address && "port" in address);
    const response = await fetch(`http://127.0.0.1:${address.port}/v1/sessions/wa-1/messages`, {
      method: "POST",
      headers: { authorization: "Bearer wrong", "content-type": "application/json" },
      body: JSON.stringify({ message_id: "msg_123", chat_id: "chat", type: "text", body: "hello" }),
    });
    assert.equal(response.status, 401);
    assert.equal(sendCalls, 0);
  } finally {
    await new Promise<void>((resolveClose) => httpServer.close(() => resolveClose()));
  }
});
