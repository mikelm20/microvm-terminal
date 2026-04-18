import { ImageResponse } from "next/og";
import { OgTemplate } from "@/lib/og-template";

export const runtime = "nodejs";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

export default function Og(): ImageResponse {
  return new ImageResponse(
    (
      <OgTemplate
        lang="es"
        eyebrow="learn.example.com"
        title="Aprende Claude Code usandolo"
        subtitle="Tres minutos al dia. Claude aprendiendo contigo."
        metaLeft="Platform Engineering"
      />
    ),
    { ...size },
  );
}
