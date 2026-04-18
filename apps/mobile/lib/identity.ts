// Local identity. A UUID minted on first launch, stored in secure-store.
// On every launch we re-attest with the server via POST /identity so it can
// refresh the signed cookie. The anonymous UUID is the identity a learner
// carries until they claim an email.

import * as SecureStore from "expo-secure-store";
import { Platform } from "react-native";
import type { Lang } from "@learn/shared-api/schemas";

const KEY = "learn-identity-v1";
const COOKIE_KEY = "learn-cookie-v1";
const CLAIM_KEY = "learn-claimed-v1";

export type Identity = {
  uuid: string;
  cookie: string | null;
  email: string | null;
};

// Cheap UUID v4. Avoids an extra dep. expo-crypto would also work.
function uuidv4(): string {
  const hex = "0123456789abcdef";
  let out = "";
  for (let i = 0; i < 36; i++) {
    if (i === 8 || i === 13 || i === 18 || i === 23) {
      out += "-";
    } else if (i === 14) {
      out += "4";
    } else if (i === 19) {
      out += hex[(Math.random() * 4) | 8] ?? "8";
    } else {
      out += hex[(Math.random() * 16) | 0] ?? "0";
    }
  }
  return out;
}

async function getItem(key: string): Promise<string | null> {
  if (Platform.OS === "web") {
    if (typeof globalThis.localStorage === "undefined") return null;
    return globalThis.localStorage.getItem(key);
  }
  return SecureStore.getItemAsync(key);
}

async function setItem(key: string, value: string): Promise<void> {
  if (Platform.OS === "web") {
    if (typeof globalThis.localStorage === "undefined") return;
    globalThis.localStorage.setItem(key, value);
    return;
  }
  await SecureStore.setItemAsync(key, value);
}

async function deleteItem(key: string): Promise<void> {
  if (Platform.OS === "web") {
    if (typeof globalThis.localStorage === "undefined") return;
    globalThis.localStorage.removeItem(key);
    return;
  }
  await SecureStore.deleteItemAsync(key);
}

export async function getIdentity(): Promise<Identity> {
  const uuid = (await getItem(KEY)) ?? (await mintNew());
  const cookie = await getItem(COOKIE_KEY);
  const email = await getItem(CLAIM_KEY);
  return { uuid, cookie, email };
}

async function mintNew(): Promise<string> {
  const uuid = uuidv4();
  await setItem(KEY, uuid);
  return uuid;
}

export async function saveCookie(cookie: string): Promise<void> {
  await setItem(COOKIE_KEY, cookie);
}

export async function saveClaim(email: string, uuid?: string): Promise<void> {
  if (uuid) await setItem(KEY, uuid);
  await setItem(CLAIM_KEY, email);
}

export async function clearIdentity(): Promise<void> {
  await deleteItem(KEY);
  await deleteItem(COOKIE_KEY);
  await deleteItem(CLAIM_KEY);
}

// Re-attest: calls POST /identity with the current uuid so the server can
// refresh the signed cookie. Best-effort: if offline we keep the local uuid.
export async function reattestIdentity(
  apiBaseUrl: string,
  lang: Lang,
): Promise<Identity> {
  const id = await getIdentity();
  try {
    const res = await fetch(`${apiBaseUrl}/identity`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ uuid: id.uuid, lang }),
    });
    if (!res.ok) return id;
    const body = (await res.json()) as { uuid: string; cookie: string };
    if (body.uuid !== id.uuid) {
      await setItem(KEY, body.uuid);
    }
    await saveCookie(body.cookie);
    return { uuid: body.uuid, cookie: body.cookie, email: id.email };
  } catch {
    return id;
  }
}
