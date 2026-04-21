"use client";

import { mintIdentity } from "./api";

const STORAGE_KEY = "learn.identity.uuid";

function randomUuid(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  // Extremely unlikely fallback for ancient runtimes.
  const hex = "0123456789abcdef";
  const parts = [8, 4, 4, 4, 12].map((len) =>
    Array.from({ length: len }, () =>
      hex[Math.floor(Math.random() * hex.length)],
    ).join(""),
  );
  return parts.join("-");
}

export function readCachedIdentity(): string | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

export function writeCachedIdentity(uuid: string): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(STORAGE_KEY, uuid);
  } catch {
    // ignore storage errors (private mode, quota, etc.)
  }
}

export async function ensureIdentity(lang: "es" | "en" = "es"): Promise<string> {
  const cached = readCachedIdentity();
  try {
    const res = await mintIdentity({
      uuid: cached ?? undefined,
      lang,
    });
    writeCachedIdentity(res.uuid);
    return res.uuid;
  } catch {
    // Offline fallback, keep the user moving locally.
    const fallback = cached ?? randomUuid();
    writeCachedIdentity(fallback);
    return fallback;
  }
}
