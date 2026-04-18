import * as React from "react";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import Link from "next/link";
import { coerceLang, fmt, isLang, voiceFor } from "@/lib/i18n";
import { getPublicProfile } from "@/lib/api";
import { SEED_PROFILE, SEED_PROFILE_UUID } from "@/lib/mock";
import { APP_STORE_URL, PLAY_STORE_URL, appDeepLink } from "@/lib/deep-link";

type PageParams = { lang: string; uuid: string };

async function loadProfile(uuid: string): Promise<{
  profile: Awaited<ReturnType<typeof getPublicProfile>>;
  fromSeed: boolean;
}> {
  try {
    const profile = await getPublicProfile(uuid);
    if (profile) return { profile, fromSeed: false };
  } catch {
    // Fall through to seed.
  }
  // TODO: remove seed fallback once Agent-API ships `/p/:uuid/public`.
  if (uuid === SEED_PROFILE_UUID) {
    return { profile: SEED_PROFILE, fromSeed: true };
  }
  return { profile: null, fromSeed: false };
}

export async function generateMetadata({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<Metadata> {
  const { lang: rawLang, uuid } = await params;
  if (!isLang(rawLang)) return {};
  const lang = coerceLang(rawLang);
  const { profile } = await loadProfile(uuid);
  if (!profile) return { title: lang === "es" ? "Perfil" : "Profile" };
  const name = profile.name ?? "learner";
  const voice = voiceFor(lang);
  const desc = fmt(voice.share.body_profile, { streak: profile.streak_days });
  return {
    title: lang === "es" ? `${name} . aprendiendo Claude Code` : `${name} . learning Claude Code`,
    description: desc,
    openGraph: {
      title: lang === "es" ? `${name} . aprendiendo Claude Code` : `${name} . learning Claude Code`,
      description: desc,
      images: [
        {
          url: `/og/profile/${uuid}`,
          width: 1200,
          height: 630,
        },
      ],
    },
    twitter: {
      card: "summary_large_image",
      title: `${name} . learn.example.com`,
      description: desc,
      images: [`/og/profile/${uuid}`],
    },
  };
}

export default async function ProfilePage({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<React.ReactElement> {
  const { lang: rawLang, uuid } = await params;
  if (!isLang(rawLang)) notFound();
  const lang = coerceLang(rawLang);
  const voice = voiceFor(lang);
  const { profile, fromSeed } = await loadProfile(uuid);
  if (!profile) notFound();

  const streakLabel =
    profile.streak_days === 0
      ? voice.path.streak_zero
      : profile.streak_days === 1
      ? fmt(voice.path.streak_singular, { n: profile.streak_days })
      : fmt(voice.path.streak_plural, { n: profile.streak_days });

  const departmentLabel = profile.department
    ? voice.role_pick.departments[profile.department]
    : null;

  const deepLink = appDeepLink(`/p/${uuid}`);

  return (
    <article className="mx-auto max-w-3xl px-6 py-16 space-y-10">
      <header className="space-y-3">
        <p className="learn-eyebrow">learn.example.com</p>
        <h1 className="text-4xl leading-tight text-learn-warmHi">
          {profile.name ?? (lang === "es" ? "Un aprendiz" : "A learner")}
        </h1>
        <div className="flex flex-wrap items-center gap-2 text-sm text-learn-warm">
          {departmentLabel ? <span className="learn-pill">{departmentLabel}</span> : null}
          <span className="learn-pill">{streakLabel}</span>
          <span className="learn-pill">
            {profile.total_xp} XP
          </span>
        </div>
        {fromSeed ? (
          <p className="text-xs text-learn-warm/60">
            {lang === "es" ? "Datos de demostracion" : "Seed data"}
          </p>
        ) : null}
      </header>

      <section className="space-y-4">
        <h2 className="text-2xl text-learn-warmHi">
          {lang === "es" ? "Modulos terminados" : "Completed modules"}
        </h2>
        <ul className="space-y-4">
          {profile.modules_completed.map((mod) => (
            <li key={mod.lesson_id} className="learn-card space-y-3">
              <div className="flex items-baseline justify-between gap-4">
                <Link
                  href={`/${lang}/p/${uuid}/m/${mod.lesson_id}`}
                  className="text-lg text-learn-warmHi hover:underline"
                >
                  {mod.title}
                </Link>
                <time className="text-sm text-learn-warm font-mono">
                  {mod.completed_at.slice(0, 10)}
                </time>
              </div>
              {mod.takeaways.length > 0 ? (
                <ul className="list-disc pl-5 space-y-1 text-sm text-learn-warm">
                  {mod.takeaways.map((t, i) => (
                    <li key={i}>{t}</li>
                  ))}
                </ul>
              ) : null}
            </li>
          ))}
        </ul>
      </section>

      <section className="learn-card space-y-4">
        <h2 className="text-2xl text-learn-warmHi">
          {lang === "es" ? "Empieza tu propio camino" : "Start your own path"}
        </h2>
        <p className="text-learn-warm">
          {lang === "es"
            ? "Abre la app y te llevamos directo al primer prompt."
            : "Open the app and we take you straight to the first prompt."}
        </p>
        <div className="flex flex-wrap gap-3">
          <Link href={deepLink} className="learn-button-primary">
            {lang === "es" ? "Abrir en la app" : "Open in the app"}
          </Link>
          <Link href={APP_STORE_URL} className="learn-button-secondary" target="_blank" rel="noreferrer">
            {lang === "es" ? "iOS" : "iOS"}
          </Link>
          <Link href={PLAY_STORE_URL} className="learn-button-secondary" target="_blank" rel="noreferrer">
            {lang === "es" ? "Android" : "Android"}
          </Link>
        </div>
      </section>
    </article>
  );
}
