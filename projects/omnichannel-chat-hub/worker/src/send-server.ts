import type { IncomingMessage, ServerResponse } from "node:http";

import type { WorkerConfig } from "./config.js";

export type SendTextRequest = {
  messageId: string;
  chatId: string;
  type: "text";
  body: string;
};

export type SendTextResult = {
  externalMessageId: string;
  status: "sent";
};

export interface OutboundSender {
  sendText(request: SendTextRequest): Promise<SendTextResult>;
}

export class MockOutboundSender implements OutboundSender {
  readonly sent: SendTextRequest[] = [];

  async sendText(request: SendTextRequest): Promise<SendTextResult> {
    this.sent.push(request);
    return { externalMessageId: `mock_${request.messageId}`, status: "sent" };
  }
}

export class WorkerSendServer {
  constructor(
    private readonly config: WorkerConfig,
    private readonly sender: OutboundSender,
  ) {}

  handler(): (req: IncomingMessage, res: ServerResponse) => void {
    return (req, res) => {
      void this.handle(req, res).catch((error: unknown) => {
        writeJSON(res, 500, {
          error: error instanceof Error ? error.message : String(error),
        });
      });
    };
  }

  private async handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
    if (req.method !== "POST") {
      writeJSON(res, 405, { error: "method not allowed" });
      return;
    }

    if (!this.validBearer(req.headers.authorization)) {
      writeJSON(res, 401, { error: "unauthorized" });
      return;
    }

    const expectedPath = `/v1/sessions/${encodeURIComponent(this.config.sessionId)}/messages`;
    if ((req.url ?? "").split("?", 1)[0] !== expectedPath) {
      writeJSON(res, 404, { error: "not found" });
      return;
    }

    const raw = await readBody(req);
    let body: { message_id?: string; chat_id?: string; type?: string; body?: string };
    try {
      body = JSON.parse(raw) as typeof body;
    } catch {
      writeJSON(res, 400, { error: "invalid json" });
      return;
    }

    const request = normalizeSendRequest(body);
    if (typeof request === "string") {
      writeJSON(res, 400, { error: request });
      return;
    }

    try {
      const result = await this.sender.sendText(request);
      writeJSON(res, 200, {
        external_message_id: result.externalMessageId,
        status: result.status,
      });
    } catch (error: unknown) {
      writeJSON(res, 503, {
        error: error instanceof Error ? error.message : String(error),
      });
    }
  }

  private validBearer(header: string | undefined): boolean {
    if (!this.config.internalSecret) {
      return false;
    }
    const prefix = "Bearer ";
    return header === `${prefix}${this.config.internalSecret}`;
  }
}

function normalizeSendRequest(body: {
  message_id?: string;
  chat_id?: string;
  type?: string;
  body?: string;
}): SendTextRequest | string {
  const messageId = body.message_id?.trim() ?? "";
  const chatId = body.chat_id?.trim() ?? "";
  const text = body.body?.trim() ?? "";
  if (!messageId) {
    return "message_id is required";
  }
  if (!chatId) {
    return "chat_id is required";
  }
  if (body.type !== "text") {
    return "only text messages are supported";
  }
  if (!text) {
    return "body is required";
  }
  return { messageId, chatId, type: "text", body: text };
}

async function readBody(req: IncomingMessage): Promise<string> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) {
    chunks.push(Buffer.from(chunk));
  }
  return Buffer.concat(chunks).toString("utf8");
}

function writeJSON(
  res: ServerResponse,
  statusCode: number,
  payload: Record<string, unknown>,
): void {
  res.writeHead(statusCode, { "content-type": "application/json" });
  res.end(JSON.stringify(payload));
}
