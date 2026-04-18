import { WsEvent } from "@learn/shared-api/events";
import type { z } from "zod";

/**
 * Wizard WebSocket client with reconnect + transcript replay.
 *
 * Sequence on each open:
 * 1. If we have a lastSeq marker, request transcript from REST first so
 *    the UI has the full tail before the live feed resumes.
 * 2. Subscribe to the live channel; each message is Zod-validated.
 *
 * Reconnect backoff: [1, 2, 5, 10, 30] seconds with +/- 20% jitter.
 */

type WsEventT = z.infer<typeof WsEvent>;

export type WsHandler = (event: WsEventT) => void;
export type WsStatus =
  | { kind: "idle" }
  | { kind: "connecting" }
  | { kind: "open" }
  | { kind: "reconnecting"; delay_ms: number; attempt: number }
  | { kind: "closed"; reason?: string };
export type WsStatusHandler = (status: WsStatus) => void;

const BACKOFF_STEPS_MS = [1000, 2000, 5000, 10_000, 30_000];

export interface WsClientOptions {
  url: string;
  onEvent: WsHandler;
  onStatus?: WsStatusHandler;
  /** Called on each (re)connect; expected to resolve a transcript replay. */
  onReplayRequested?: () => Promise<void>;
}

export class WsClient {
  private socket: WebSocket | null = null;
  private closedByUser = false;
  private attempt = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(private opts: WsClientOptions) {}

  connect(): void {
    this.closedByUser = false;
    this.open();
  }

  close(): void {
    this.closedByUser = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.socket) {
      try {
        this.socket.close();
      } catch {
        // ignore
      }
      this.socket = null;
    }
    this.opts.onStatus?.({ kind: "closed" });
  }

  send(payload: unknown): void {
    if (!this.socket || this.socket.readyState !== 1) return;
    try {
      this.socket.send(JSON.stringify(payload));
    } catch {
      // ignore
    }
  }

  private open(): void {
    this.opts.onStatus?.({ kind: "connecting" });

    const replayPromise =
      this.opts.onReplayRequested?.() ?? Promise.resolve();

    replayPromise
      .catch(() => undefined)
      .finally(() => {
        if (this.closedByUser) return;
        try {
          this.socket = new WebSocket(this.opts.url);
        } catch {
          this.scheduleReconnect();
          return;
        }
        this.wire(this.socket);
      });
  }

  private wire(ws: WebSocket): void {
    ws.onopen = () => {
      this.attempt = 0;
      this.opts.onStatus?.({ kind: "open" });
    };

    ws.onmessage = (ev) => {
      const data = typeof ev.data === "string" ? ev.data : null;
      if (!data) return;
      let payload: unknown;
      try {
        payload = JSON.parse(data);
      } catch {
        return;
      }
      const parsed = WsEvent.safeParse(payload);
      if (!parsed.success) return;
      this.opts.onEvent(parsed.data);
    };

    ws.onerror = () => {
      // The browser/RN WS polyfill often fires onerror just before onclose.
      // We rely on onclose to drive the reconnect logic.
    };

    ws.onclose = (ev) => {
      this.socket = null;
      if (this.closedByUser) {
        this.opts.onStatus?.({ kind: "closed", reason: ev.reason });
        return;
      }
      this.scheduleReconnect(ev.reason);
    };
  }

  private scheduleReconnect(reason?: string): void {
    const idx = Math.min(this.attempt, BACKOFF_STEPS_MS.length - 1);
    const base = BACKOFF_STEPS_MS[idx]!;
    const jitter = base * (Math.random() * 0.4 - 0.2);
    const delay = Math.max(250, Math.round(base + jitter));
    this.attempt += 1;
    this.opts.onStatus?.({
      kind: "reconnecting",
      delay_ms: delay,
      attempt: this.attempt,
    });
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.open();
    }, delay);
    void reason;
  }
}
