import AsyncStorage from "@react-native-async-storage/async-storage";
import { v4 as uuid } from "uuid";

/**
 * AsyncStorage-backed outbound queue. Items are JSON-serializable and
 * idempotent via Idempotency-Key. Drains on demand (on network restore
 * or on explicit flush), and reconciles with server echoes via the WS
 * event stream.
 */

export interface QueuedRequest<B = unknown> {
  id: string;
  endpoint: string; // absolute URL or path
  method: "POST" | "PATCH" | "PUT";
  headers: Record<string, string>;
  body: B;
  idempotency_key: string;
  created_at: string;
  attempts: number;
}

const STORAGE_KEY = "learn.outbound.queue.v1";

async function read(): Promise<QueuedRequest[]> {
  try {
    const raw = await AsyncStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

async function write(items: QueuedRequest[]): Promise<void> {
  await AsyncStorage.setItem(STORAGE_KEY, JSON.stringify(items));
}

export const offlineQueue = {
  /** Push a new request to the end of the queue. Returns the assigned id. */
  async enqueue<B>(
    input: Omit<QueuedRequest<B>, "id" | "created_at" | "attempts" | "idempotency_key"> &
      Partial<Pick<QueuedRequest<B>, "idempotency_key">>,
  ): Promise<QueuedRequest<B>> {
    const items = await read();
    const item: QueuedRequest<B> = {
      id: uuid(),
      created_at: new Date().toISOString(),
      attempts: 0,
      idempotency_key: input.idempotency_key ?? uuid(),
      endpoint: input.endpoint,
      method: input.method,
      headers: input.headers,
      body: input.body,
    };
    items.push(item as QueuedRequest);
    await write(items);
    return item;
  },

  /** Peek at the current queue without mutating. */
  async peek(): Promise<QueuedRequest[]> {
    return read();
  },

  /** Remove a single item by id. */
  async remove(id: string): Promise<void> {
    const items = await read();
    await write(items.filter((it) => it.id !== id));
  },

  /** Mark an item as attempted. */
  async bumpAttempts(id: string): Promise<void> {
    const items = await read();
    const next = items.map((it) => (it.id === id ? { ...it, attempts: it.attempts + 1 } : it));
    await write(next);
  },

  /** Wipe the queue. */
  async clear(): Promise<void> {
    await AsyncStorage.removeItem(STORAGE_KEY);
  },

  /**
   * Drain the queue by calling `sender` sequentially. Any item that throws
   * stays in the queue and its attempts counter is incremented.
   */
  async drain(
    sender: (item: QueuedRequest) => Promise<void>,
  ): Promise<{ sent: number; failed: number }> {
    const items = await read();
    let sent = 0;
    let failed = 0;
    for (const item of items) {
      try {
        await sender(item);
        await offlineQueue.remove(item.id);
        sent += 1;
      } catch {
        await offlineQueue.bumpAttempts(item.id);
        failed += 1;
        // Stop on first failure to preserve ordering; the caller can retry.
        break;
      }
    }
    return { sent, failed };
  },
};
