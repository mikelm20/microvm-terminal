import { voice, type Lang } from "@learn/shared-voice";

/**
 * Dotted-key lookup with {placeholder} interpolation over the voice JSON.
 * Keys are not statically checked here; callers pass strings that the
 * voice table is expected to cover. A missing key returns the key itself,
 * which makes missing strings visible instead of silent.
 */

type Params = Record<string, string | number>;

function lookup(table: unknown, key: string): string | undefined {
  const parts = key.split(".");
  let node: unknown = table;
  for (const p of parts) {
    if (typeof node !== "object" || node === null) return undefined;
    node = (node as Record<string, unknown>)[p];
  }
  return typeof node === "string" ? node : undefined;
}

function interpolate(template: string, params?: Params): string {
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (_, name: string) => {
    const v = params[name];
    return v === undefined ? `{${name}}` : String(v);
  });
}

export function t(lang: Lang, key: string, params?: Params): string {
  const table = voice(lang);
  const raw = lookup(table, key) ?? lookup(voice("es"), key);
  if (!raw) return key;
  return interpolate(raw, params);
}
