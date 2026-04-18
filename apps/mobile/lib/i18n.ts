// Thin i18n wrapper. Resolves a dotted key path against the current
// voice table, optionally interpolates {name} placeholders. The current
// language lives in secure-store; components usually read it via useLang().

import { useEffect, useState } from "react";
import * as SecureStore from "expo-secure-store";
import { Platform } from "react-native";
import { voice, type Lang, type Voice } from "@learn/shared-voice";

const LANG_KEY = "learn-lang-v1";
const DEFAULT_LANG: Lang = "es";

async function readLangPersistent(): Promise<Lang | null> {
  if (Platform.OS === "web") {
    if (typeof globalThis.localStorage === "undefined") return null;
    const raw = globalThis.localStorage.getItem(LANG_KEY);
    return raw === "es" || raw === "en" ? raw : null;
  }
  const raw = await SecureStore.getItemAsync(LANG_KEY);
  return raw === "es" || raw === "en" ? raw : null;
}

async function writeLangPersistent(l: Lang): Promise<void> {
  if (Platform.OS === "web") {
    if (typeof globalThis.localStorage === "undefined") return;
    globalThis.localStorage.setItem(LANG_KEY, l);
    return;
  }
  await SecureStore.setItemAsync(LANG_KEY, l);
}

export async function getSavedLang(): Promise<Lang> {
  return (await readLangPersistent()) ?? DEFAULT_LANG;
}

export async function setSavedLang(l: Lang): Promise<void> {
  await writeLangPersistent(l);
}

export function useLang(): [Lang, (l: Lang) => Promise<void>] {
  const [lang, setLangState] = useState<Lang>(DEFAULT_LANG);
  useEffect(() => {
    let mounted = true;
    readLangPersistent().then((l) => {
      if (mounted && l) setLangState(l);
    });
    return () => {
      mounted = false;
    };
  }, []);
  const setLang = async (l: Lang) => {
    await writeLangPersistent(l);
    setLangState(l);
  };
  return [lang, setLang];
}

function resolve(table: Voice, path: string): string {
  const parts = path.split(".");
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let node: any = table;
  for (const p of parts) {
    if (node == null || typeof node !== "object") return path;
    node = node[p];
  }
  return typeof node === "string" ? node : path;
}

export function tFor(lang: Lang, key: string, params?: Record<string, string | number>): string {
  const table = voice(lang);
  const raw = resolve(table, key);
  if (!params) return raw;
  return raw.replace(/\{(\w+)\}/g, (_match, name: string) => {
    const v = params[name];
    return v === undefined || v === null ? "" : String(v);
  });
}

export function useT(): (key: string, params?: Record<string, string | number>) => string {
  const [lang] = useLang();
  return (key, params) => tFor(lang, key, params);
}
