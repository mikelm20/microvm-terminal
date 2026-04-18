import type { MetadataRoute } from "next";

export default function robots(): MetadataRoute.Robots {
  const base = process.env.NEXT_PUBLIC_SITE_URL || "https://learn.example.com";
  return {
    rules: [
      {
        userAgent: "*",
        allow: [
          "/",
          "/about",
          "/changelog",
          "/status",
          "/p/",
          "/certificate/",
          "/verify/",
        ],
        disallow: ["/api/"],
      },
    ],
    sitemap: `${base}/sitemap.xml`,
  };
}
