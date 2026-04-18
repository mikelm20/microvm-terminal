import es from "./es.json" with { type: "json" };
import en from "./en.json" with { type: "json" };

export type Voice = typeof es;
export type Lang = "es" | "en";

const tables: Record<Lang, Voice> = { es, en: en as Voice };

export function voice(lang: Lang): Voice {
  return tables[lang];
}

export { es, en };
