import {
  PublicProfileResponse,
  PublicCertificateResponse,
} from "@learn/shared-api/schemas";
import type {
  PublicProfileResponse as PublicProfile,
  PublicCertificateResponse as PublicCertificate,
} from "@learn/shared-api/schemas";

const DEFAULT_BASE = "https://api.learn.example.com";

export function controlPlaneBase(): string {
  return process.env.CONTROL_PLANE_URL || process.env.NEXT_PUBLIC_CONTROL_PLANE_URL || DEFAULT_BASE;
}

class ApiError extends Error {
  constructor(public status: number, public body: string) {
    super(`control-plane ${status}: ${body.slice(0, 200)}`);
  }
}

function timeoutMs(): number {
  const v = Number(process.env.CONTROL_PLANE_TIMEOUT_MS || "");
  return Number.isFinite(v) && v > 0 ? v : 4000;
}

class TimeoutError extends Error {
  constructor() {
    super("control-plane timeout");
  }
}

async function raceWithTimeout<T>(p: Promise<T>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const t = setTimeout(() => reject(new TimeoutError()), timeoutMs());
    p.then(
      (v) => {
        clearTimeout(t);
        resolve(v);
      },
      (e) => {
        clearTimeout(t);
        reject(e);
      },
    );
  });
}

async function fetchJson(path: string, init?: RequestInit): Promise<unknown> {
  const base = controlPlaneBase();
  const url = base.replace(/\/$/, "") + path;
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), timeoutMs());
  try {
    const res = await raceWithTimeout(
      fetch(url, {
        ...init,
        headers: {
          accept: "application/json",
          ...(init?.headers ?? {}),
        },
        signal: ctrl.signal,
        next: { revalidate: 60 },
      }),
    );
    if (!res.ok) {
      const text = await res.text().catch(() => "");
      throw new ApiError(res.status, text);
    }
    return res.json();
  } finally {
    clearTimeout(t);
  }
}

function isUpstreamDown(err: unknown): boolean {
  if (err instanceof ApiError && err.status === 404) return true;
  if (err instanceof TimeoutError) return true;
  if (err instanceof Error && (err.name === "AbortError" || err.name === "TypeError")) return true;
  // Node fetch TypeErrors wrap network errors (ECONNREFUSED, ENOTFOUND).
  return false;
}

/**
 * Public profile read. Control plane surface: `GET /p/:uuid/public`.
 * Returns `null` on 404 or any upstream failure so callers can render a
 * fallback or 404 without leaking control-plane unavailability into the page.
 */
export async function getPublicProfile(uuid: string): Promise<PublicProfile | null> {
  try {
    const raw = await fetchJson(`/p/${encodeURIComponent(uuid)}/public`);
    return PublicProfileResponse.parse(raw);
  } catch (err) {
    if (isUpstreamDown(err)) return null;
    throw err;
  }
}

/**
 * Public certificate read. Control plane surface: `GET /certificate/:id/public`.
 */
export async function getPublicCertificate(id: string): Promise<PublicCertificate | null> {
  try {
    const raw = await fetchJson(`/certificate/${encodeURIComponent(id)}/public`);
    return PublicCertificateResponse.parse(raw);
  } catch (err) {
    if (isUpstreamDown(err)) return null;
    throw err;
  }
}

/**
 * Control plane healthz (no-cache). Used by the Status page client.
 */
export async function getHealth(): Promise<{ ok: boolean }> {
  const base = controlPlaneBase();
  const url = base.replace(/\/$/, "") + "/healthz";
  const res = await fetch(url, { cache: "no-store" });
  if (!res.ok) return { ok: false };
  try {
    const data = (await res.json()) as { ok?: boolean };
    return { ok: !!data?.ok };
  } catch {
    return { ok: false };
  }
}
