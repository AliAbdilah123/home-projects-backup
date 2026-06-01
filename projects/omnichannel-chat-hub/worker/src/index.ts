import { createServer } from "node:http";

import { configFromEnv } from "./config.js";
import {
  CompositeEventSink,
  ConsoleEventSink,
  createSession,
  HttpEventSink,
} from "./service.js";
import type { WorkerEvent } from "./events.js";
import { WorkerSendServer } from "./send-server.js";

async function main() {
  const config = configFromEnv();
  const sink = new CompositeEventSink([
    new ConsoleEventSink(),
    new HttpEventSink(config),
  ]);
  const session = createSession(config, async (event: WorkerEvent) => {
    await sink.publish(event);
  });

  const sendHttpServer = createServer(
    new WorkerSendServer(config, session).handler(),
  );
  const [host, portText] = config.sendListenAddr.split(":");
  await new Promise<void>((resolveReady) =>
    sendHttpServer.listen(Number(portText), host, resolveReady),
  );

  await session.start();

  if (config.mode === "mock" && config.exitAfterConnected) {
    return;
  }

  await waitForever();
}

function waitForever(): Promise<never> {
  return new Promise(() => undefined);
}

main().catch((error: unknown) => {
  console.error(error);
  process.exit(1);
});
