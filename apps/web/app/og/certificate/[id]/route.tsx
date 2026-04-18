import { ImageResponse } from "next/og";
import { getPublicCertificate } from "@/lib/api";
import { SEED_CERTIFICATE, SEED_CERTIFICATE_ID } from "@/lib/mock";
import { OG_HEIGHT, OG_WIDTH, OgTemplate } from "@/lib/og-template";
import { coerceLang } from "@/lib/i18n";

export const runtime = "nodejs";
export const revalidate = 60;

export async function GET(
  _req: Request,
  ctx: { params: Promise<{ id: string }> },
): Promise<Response> {
  const { id } = await ctx.params;
  let cert = null as Awaited<ReturnType<typeof getPublicCertificate>>;
  try {
    cert = await getPublicCertificate(id);
  } catch {
    cert = null;
  }
  if (!cert && id === SEED_CERTIFICATE_ID) cert = SEED_CERTIFICATE;

  const lang = coerceLang(cert?.lang ?? "es");
  const title = cert?.name ?? (lang === "es" ? "Aprendiz" : "Learner");
  const completedDate = cert ? cert.completed_at.slice(0, 10) : "";
  const subtitle =
    lang === "es"
      ? "Fundamentos de Claude Code"
      : "Claude Code Fundamentals";
  const modulesLabel = cert ? `${cert.modules.length} ${lang === "es" ? "modulos" : "modules"}` : undefined;

  return new ImageResponse(
    (
      <OgTemplate
        lang={lang}
        eyebrow={lang === "es" ? "Certificado" : "Certificate"}
        title={title}
        subtitle={subtitle}
        metaLeft={completedDate ? `${lang === "es" ? "Terminado" : "Completed"} ${completedDate}` : "Platform Engineering"}
        metaRight={modulesLabel}
      />
    ),
    {
      width: OG_WIDTH,
      height: OG_HEIGHT,
      headers: {
        "Cache-Control": "public, max-age=60, s-maxage=60, stale-while-revalidate=600",
      },
    },
  );
}
