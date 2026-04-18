import { voice as loadVoice, type Lang, type Voice } from "@learn/shared-voice";

export const SUPPORTED_LANGS = ["es", "en"] as const;
export const DEFAULT_LANG: Lang = "es";

export function isLang(candidate: string): candidate is Lang {
  return (SUPPORTED_LANGS as readonly string[]).includes(candidate);
}

export function coerceLang(candidate: string | undefined | null): Lang {
  if (candidate && isLang(candidate)) return candidate;
  return DEFAULT_LANG;
}

/**
 * Reads Accept-Language, returns the best supported match or DEFAULT_LANG.
 * Keeps it dependency-free on purpose; we only have two locales.
 */
export function negotiateLang(acceptLanguageHeader: string | null | undefined): Lang {
  if (!acceptLanguageHeader) return DEFAULT_LANG;
  const tags = acceptLanguageHeader
    .split(",")
    .map((chunk) => {
      const [tag, qPart] = chunk.split(";");
      const q = qPart && qPart.trim().startsWith("q=") ? parseFloat(qPart.trim().slice(2)) : 1;
      return { tag: tag?.trim().toLowerCase() ?? "", q: Number.isFinite(q) ? q : 1 };
    })
    .filter((t) => t.tag.length > 0)
    .sort((a, b) => b.q - a.q);
  for (const { tag } of tags) {
    const primary = tag.split("-")[0];
    if (primary && isLang(primary)) return primary;
  }
  return DEFAULT_LANG;
}

export function voiceFor(lang: Lang): Voice {
  return loadVoice(lang);
}

/** Simple {placeholder} interpolation for voice strings. */
export function fmt(template: string, values: Record<string, string | number>): string {
  return template.replace(/\{(\w+)\}/g, (_, k) => {
    const v = values[k];
    return v === undefined || v === null ? "" : String(v);
  });
}

export type { Lang, Voice };
