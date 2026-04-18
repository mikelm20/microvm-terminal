import type { MetadataRoute } from "next";

const STATIC_ROUTES = ["", "/about", "/status", "/changelog"];
const LANGS = ["es", "en"] as const;

export default function sitemap(): MetadataRoute.Sitemap {
  const base = process.env.NEXT_PUBLIC_SITE_URL || "https://learn.example.com";
  const now = new Date();
  const entries: MetadataRoute.Sitemap = [];
  for (const lang of LANGS) {
    for (const route of STATIC_ROUTES) {
      entries.push({
        url: `${base}/${lang}${route}`,
        lastModified: now,
        changeFrequency: route === "/changelog" ? "weekly" : "monthly",
        priority: route === "" ? 1.0 : 0.7,
      });
    }
  }
  return entries;
}
