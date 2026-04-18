import { ImageResponse } from "next/og";
import { getPublicProfile } from "@/lib/api";
import { SEED_PROFILE, SEED_PROFILE_UUID } from "@/lib/mock";
import { OG_HEIGHT, OG_WIDTH, OgTemplate } from "@/lib/og-template";
import { fmt, voiceFor, coerceLang } from "@/lib/i18n";

export const runtime = "nodejs";
export const revalidate = 60;

export async function GET(
  _req: Request,
  ctx: { params: Promise<{ uuid: string }> },
): Promise<Response> {
  const { uuid } = await ctx.params;
  let profile = null as Awaited<ReturnType<typeof getPublicProfile>>;
  try {
    profile = await getPublicProfile(uuid);
  } catch {
    profile = null;
  }
  if (!profile && uuid === SEED_PROFILE_UUID) profile = SEED_PROFILE;

  const lang = coerceLang(profile?.lang ?? "es");
  const voice = voiceFor(lang);
  const name = profile?.name ?? (lang === "es" ? "un aprendiz" : "a learner");
  const streak = profile?.streak_days ?? 0;
  const streakLine =
    streak === 0
      ? voice.path.streak_zero
      : streak === 1
      ? fmt(voice.path.streak_singular, { n: streak })
      : fmt(voice.path.streak_plural, { n: streak });
  const department = profile?.department
    ? voice.role_pick.departments[profile.department]
    : lang === "es"
    ? "aprendiendo Claude Code"
    : "learning Claude Code";

  return new ImageResponse(
    (
      <OgTemplate
        lang={lang}
        eyebrow={department}
        title={name}
        subtitle={streakLine}
        stat={{
          value: String(profile?.total_xp ?? 0),
          label: "XP",
        }}
        metaLeft="Platform Engineering"
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
