import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";

import { loadBaileys } from "./baileys.js";
import { sessionAuthDir, type WorkerConfig } from "./config.js";
import {
  nowIso,
  PROVIDER,
  workerEventToApiPayload,
  type SessionStatus,
  type WorkerEvent,
} from "./events.js";
import type {
  OutboundSender,
  SendTextRequest,
  SendTextResult,
} from "./send-server.js";

export type EventPublisher = (event: WorkerEvent) => Promise<void> | void;

export interface EventSink {
  publish(event: WorkerEvent): Promise<void>;
}

export class ConsoleEventSink implements EventSink {
  async publish(event: WorkerEvent): Promise<void> {
    console.log(JSON.stringify(workerEventToApiPayload(event)));
  }
}

export class HttpEventSink implements EventSink {
  constructor(private readonly config: WorkerConfig) {}

  async publish(event: WorkerEvent): Promise<void> {
    if (!this.config.apiBaseUrl) {
      return;
    }

    const url = new URL(
      this.config.internalWebhookPath,
      this.config.apiBaseUrl,
    );
    const headers: Record<string, string> = {
      "content-type": "application/json",
    };
    if (this.config.internalSecret) {
      headers.authorization = `Bearer ${this.config.internalSecret}`;
    }

    const response = await fetch(url, {
      method: "POST",
      headers,
      body: JSON.stringify(workerEventToApiPayload(event)),
    });

    if (!response.ok) {
      const body = await response.text().catch(() => "");
      throw new Error(
        `Internal webhook POST failed: ${response.status} ${response.statusText} ${body}`,
      );
    }
  }
}

export class CompositeEventSink implements EventSink {
  constructor(private readonly sinks: EventSink[]) {}

  async publish(event: WorkerEvent): Promise<void> {
    for (const sink of this.sinks) {
      await sink.publish(event);
    }
  }
}

export abstract class BaseSession {
  protected constructor(
    protected readonly config: WorkerConfig,
    private readonly publishEvent: EventPublisher,
  ) {}

  protected async emitStatus(
    status: SessionStatus,
    reason?: string,
  ): Promise<void> {
    await this.publishEvent({
      type: "status",
      provider: PROVIDER,
      sessionId: this.config.sessionId,
      status,
      reason,
      at: nowIso(),
    });
  }

  protected async emitQr(qr: string): Promise<void> {
    await this.publishEvent({
      type: "qr",
      provider: PROVIDER,
      sessionId: this.config.sessionId,
      status: "qr",
      qr,
      at: nowIso(),
    });
  }

  abstract start(): Promise<void>;
  abstract sendText(request: SendTextRequest): Promise<SendTextResult>;
}

export class MockBaileysSession extends BaseSession {
  readonly sentMessages: SendTextRequest[] = [];

  constructor(config: WorkerConfig, publishEvent: EventPublisher) {
    super(config, publishEvent);
  }

  async start(): Promise<void> {
    const authDir = sessionAuthDir(this.config);
    await mkdir(authDir, { recursive: true, mode: 0o700 });

    await this.emitStatus("starting");
    await sleep(this.config.mockDelayMs);
    await this.emitQr(this.config.mockQr);
    await sleep(this.config.mockDelayMs);

    const state = {
      provider: PROVIDER,
      mode: "mock",
      sessionId: this.config.sessionId,
      status: "connected",
      connectedAt: nowIso(),
    };
    await writeFile(
      join(authDir, "mock-session.json"),
      JSON.stringify(state, null, 2),
      {
        mode: 0o600,
      },
    );
    await this.emitStatus("connected");
  }

  async sendText(request: SendTextRequest): Promise<SendTextResult> {
    this.sentMessages.push(request);
    return { externalMessageId: `mock_${request.messageId}`, status: "sent" };
  }
}

export class BaileysSession extends BaseSession implements OutboundSender {
  private reconnecting = false;
  private stopped = false;
  private socket?: { sendMessage: (chatId: string, message: { text: string }) => Promise<{ key?: { id?: string } }> };

  constructor(config: WorkerConfig, publishEvent: EventPublisher) {
    super(config, publishEvent);
  }

  async start(): Promise<void> {
    this.stopped = false;
    await mkdir(sessionAuthDir(this.config), { recursive: true, mode: 0o700 });
    await this.emitStatus(this.reconnecting ? "reconnecting" : "starting");

    const baileys = await loadBaileys();
    const { state, saveCreds } = await baileys.useMultiFileAuthState(
      sessionAuthDir(this.config),
    );
    const makeWASocket = baileys.default ?? baileys.makeWASocket;
    const socket = makeWASocket({
      auth: state,
      printQRInTerminal: false,
      logger: makeBaileysLogger(this.config.logLevel) as never,
      browser: ["Omnichannel Chat Hub", "Chrome", "1.0.0"],
    });
    this.socket = socket as typeof this.socket;

    socket.ev.on("creds.update", saveCreds);
    socket.ev.on(
      "connection.update",
      async (update: Record<string, unknown>) => {
        if (typeof update.qr === "string") {
          await this.emitQr(update.qr);
        }

        if (update.connection === "connecting") {
          await this.emitStatus("connecting");
        }

        if (update.connection === "open") {
          this.reconnecting = false;
          await this.emitStatus("connected");
        }

        if (update.connection === "close") {
          const statusCode = disconnectStatusCode(update.lastDisconnect);
          const loggedOut = statusCode === baileys.DisconnectReason.loggedOut;
          await this.emitStatus(
            loggedOut ? "logged_out" : "disconnected",
            `status_code=${statusCode ?? "unknown"}`,
          );

          if (!loggedOut && !this.stopped) {
            this.reconnecting = true;
            setTimeout(() => {
              void this.start().catch(async (error: unknown) => {
                await this.emitStatus(
                  "error",
                  error instanceof Error ? error.message : String(error),
                );
              });
            }, 2500);
          }
        }
      },
    );
  }

  stop(): void {
    this.stopped = true;
  }

  async sendText(request: SendTextRequest): Promise<SendTextResult> {
    if (!this.socket) {
      throw new Error("Baileys socket is not connected");
    }
    const response = await this.socket.sendMessage(request.chatId, { text: request.body });
    const externalMessageId = response.key?.id;
    if (!externalMessageId) {
      throw new Error("Baileys send did not return a message id");
    }
    return { externalMessageId, status: "sent" };
  }
}

export function createSession(
  config: WorkerConfig,
  publisher: EventPublisher,
): BaseSession {
  return config.mode === "mock"
    ? new MockBaileysSession(config, publisher)
    : new BaileysSession(config, publisher);
}

function disconnectStatusCode(lastDisconnect: unknown): number | undefined {
  const error = (
    lastDisconnect as
      | { error?: { output?: { statusCode?: number } } }
      | undefined
  )?.error;
  return error?.output?.statusCode;
}

function makeBaileysLogger(level: string): Record<string, unknown> {
  const log = (...args: unknown[]) => {
    if (level !== "silent") {
      console.error(...args);
    }
  };
  const logger: Record<string, unknown> = {
    level,
    child: () => logger,
    trace: log,
    debug: log,
    info: log,
    warn: log,
    error: log,
    fatal: log,
  };
  return logger;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
