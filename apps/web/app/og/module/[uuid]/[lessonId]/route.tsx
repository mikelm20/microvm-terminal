import { ImageResponse } from "next/og";
import { getPublicProfile } from "@/lib/api";
import { SEED_PROFILE, SEED_PROFILE_UUID } from "@/lib/mock";
import { OG_HEIGHT, OG_WIDTH, OgTemplate } from "@/lib/og-template";
import { coerceLang } from "@/lib/i18n";

export const runtime = "nodejs";
export const revalidate = 60;

export async function GET(
  _req: Request,
  ctx: { params: Promise<{ uuid: string; lessonId: string }> },
): Promise<Response> {
  const { uuid, lessonId } = await ctx.params;
  let profile = null as Awaited<ReturnType<typeof getPublicProfile>>;
  try {
    profile = await getPublicProfile(uuid);
  } catch {
    profile = null;
  }
  if (!profile && uuid === SEED_PROFILE_UUID) profile = SEED_PROFILE;
  const mod = profile?.modules_completed.find((m) => m.lesson_id === lessonId);
  const lang = coerceLang(profile?.lang ?? "es");

  const title = mod?.title ?? lessonId;
  const sub = profile?.name
    ? (lang === "es" ? `terminado por ${profile.name}` : `completed by ${profile.name}`)
    : undefined;

  return new ImageResponse(
    (
      <OgTemplate
        lang={lang}
        eyebrow={lang === "es" ? "Modulo terminado" : "Module completed"}
        title={title}
        subtitle={sub}
        metaLeft="Platform Engineering"
        metaRight={mod ? mod.completed_at.slice(0, 10) : undefined}
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
