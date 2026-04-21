import { WsEvent } from "@learn/shared-api/events";

type WizardHandlers = {
  onOpen?: () => void;
  onClose?: (ev: CloseEvent) => void;
  onError?: (err: Event) => void;
  onEvent?: (event: WsEvent) => void;
  onParseError?: (err: unknown, raw: string) => void;
};

export type WizardHandle = {
  close(): void;
};

const INITIAL_BACKOFF_MS = 500;
const MAX_BACKOFF_MS = 15_000;

function nextBackoff(current: number): number {
  const jitter = Math.random() * 0.3 + 0.85;
  return Math.min(MAX_BACKOFF_MS, Math.round(current * 2 * jitter));
}

export function connectWizard(
  url: string,
  handlers: WizardHandlers,
): WizardHandle {
  let ws: WebSocket | null = null;
  let closed = false;
  let backoff = INITIAL_BACKOFF_MS;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  const open = (): void => {
    if (closed) return;
    ws = new WebSocket(url);

    ws.addEventListener("open", () => {
      backoff = INITIAL_BACKOFF_MS;
      handlers.onOpen?.();
    });

    ws.addEventListener("message", (msg) => {
      const raw = typeof msg.data === "string" ? msg.data : "";
      try {
        const parsed: unknown = JSON.parse(raw);
        const result = WsEvent.safeParse(parsed);
        if (result.success) {
          handlers.onEvent?.(result.data);
        } else {
          handlers.onParseError?.(result.error, raw);
        }
      } catch (err) {
        handlers.onParseError?.(err, raw);
      }
    });

    ws.addEventListener("error", (err) => {
      handlers.onError?.(err);
    });

    ws.addEventListener("close", (ev) => {
      handlers.onClose?.(ev);
      if (closed) return;
      reconnectTimer = setTimeout(() => {
        backoff = nextBackoff(backoff);
        open();
      }, backoff);
    });
  };

  open();

  return {
    close() {
      closed = true;
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
      if (ws) {
        ws.close();
        ws = null;
      }
    },
  };
}

// PTY WS: raw bytes, no JSON parsing. Used by the xterm wrapper.
type PtyHandlers = {
  onOpen?: () => void;
  onClose?: (ev: CloseEvent) => void;
  onError?: (err: Event) => void;
  onData: (data: ArrayBuffer | string) => void;
};

export type PtyHandle = {
  send(data: string | ArrayBuffer): void;
  close(): void;
};

export function connectPty(url: string, handlers: PtyHandlers): PtyHandle {
  const ws = new WebSocket(url);
  ws.binaryType = "arraybuffer";

  ws.addEventListener("open", () => handlers.onOpen?.());
  ws.addEventListener("close", (ev) => handlers.onClose?.(ev));
  ws.addEventListener("error", (err) => handlers.onError?.(err));
  ws.addEventListener("message", (msg) => {
    handlers.onData(msg.data as ArrayBuffer | string);
  });

  return {
    send(data) {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data);
      }
    },
    close() {
      ws.close();
    },
  };
}
