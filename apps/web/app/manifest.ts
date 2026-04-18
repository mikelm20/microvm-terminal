import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "learn.example.com",
    short_name: "learn",
    description: "Aprende Claude Code usandolo. Tres minutos al dia.",
    start_url: "/",
    display: "standalone",
    background_color: "#6e2608",
    theme_color: "#6e2608",
    icons: [
      { src: "/icon", sizes: "512x512", type: "image/png" },
      { src: "/apple-icon", sizes: "180x180", type: "image/png" },
    ],
  };
}
