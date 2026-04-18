import es from "./es.json" with { type: "json" };
import en from "./en.json" with { type: "json" };

export type { Lang, Voice, VoiceKey } from "./types";
export { t, voiceKeys } from "./types";

const tables = { es, en } as const;

export function voice(lang: "es" | "en") {
  return tables[lang];
}

export { es, en };
