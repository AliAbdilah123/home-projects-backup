export const PROVIDER = "whatsapp_baileys" as const;

export type SessionStatus =
  | "starting"
  | "qr"
  | "connecting"
  | "connected"
  | "disconnected"
  | "reconnecting"
  | "logged_out"
  | "error";

export type BaseWorkerEvent = {
  provider: typeof PROVIDER;
  sessionId: string;
  at: string;
};

export type StatusEvent = BaseWorkerEvent & {
  type: "status";
  status: SessionStatus;
  reason?: string;
};

export type QrEvent = BaseWorkerEvent & {
  type: "qr";
  status: "qr";
  qr: string;
};

export type WorkerEvent = StatusEvent | QrEvent;

export function workerEventToApiPayload(
  event: WorkerEvent,
): Record<string, unknown> {
  const payload: Record<string, unknown> = {
    type: event.type,
    provider: event.provider,
    session_id: event.sessionId,
    status: event.status,
    occurred_at: event.at,
  };

  if (event.type === "qr") {
    payload.qr = event.qr;
  }

  if ("reason" in event && event.reason) {
    payload.reason = event.reason;
  }

  return payload;
}

export function nowIso(): string {
  return new Date().toISOString();
}
