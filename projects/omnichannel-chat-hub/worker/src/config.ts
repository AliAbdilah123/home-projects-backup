import { resolve } from "node:path";

export type WorkerMode = "baileys" | "mock";

export type WorkerConfig = {
  logLevel: string;
  authDir: string;
  sessionId: string;
  mode: WorkerMode;
  apiBaseUrl?: string;
  internalWebhookPath: string;
  internalSecret?: string;
  sendListenAddr: string;
  exitAfterConnected: boolean;
  mockDelayMs: number;
  mockQr: string;
};

function parseMode(value: string | undefined): WorkerMode {
  if (
    value === "mock" ||
    value === "baileys" ||
    value === undefined ||
    value === ""
  ) {
    return value === "mock" ? "mock" : "baileys";
  }

  throw new Error(
    `Invalid WORKER_MODE=${value}. Expected "baileys" or "mock".`,
  );
}

function parseBoolean(value: string | undefined): boolean {
  return ["1", "true", "yes", "on"].includes((value ?? "").toLowerCase());
}

function parsePositiveInt(value: string | undefined, fallback: number): number {
  if (!value) {
    return fallback;
  }

  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed) || parsed < 0) {
    throw new Error(`Invalid non-negative integer: ${value}`);
  }
  return parsed;
}

export function sanitizeSessionId(sessionId: string): string {
  const sanitized = sessionId
    .trim()
    .replace(/[^a-zA-Z0-9._-]+/g, "-")
    .replace(/^[.-]+|[.-]+$/g, "");

  if (!sanitized) {
    throw new Error(
      "WORKER_SESSION_ID must contain at least one safe filename character.",
    );
  }

  return sanitized;
}

export function configFromEnv(
  env: NodeJS.ProcessEnv = process.env,
): WorkerConfig {
  return {
    logLevel: env.WORKER_LOG_LEVEL ?? "info",
    authDir: env.BAILEYS_AUTH_DIR ?? "./data/baileys-auth",
    sessionId: sanitizeSessionId(env.WORKER_SESSION_ID ?? "default"),
    mode: parseMode(env.WORKER_MODE),
    apiBaseUrl: env.WORKER_API_BASE_URL,
    internalWebhookPath:
      env.WORKER_INTERNAL_WEBHOOK_PATH ??
      "/api/v1/webhooks/whatsapp-baileys/internal",
    internalSecret: env.WORKER_INTERNAL_SECRET,
    sendListenAddr: env.WORKER_SEND_ADDR ?? "127.0.0.1:8090",
    exitAfterConnected: parseBoolean(env.WORKER_EXIT_AFTER_CONNECTED),
    mockDelayMs: parsePositiveInt(env.WORKER_MOCK_DELAY_MS, 750),
    mockQr: env.WORKER_MOCK_QR ?? "mock-whatsapp-qr-code",
  };
}

export function sessionAuthDir(config: WorkerConfig): string {
  return resolve(config.authDir, sanitizeSessionId(config.sessionId));
}
