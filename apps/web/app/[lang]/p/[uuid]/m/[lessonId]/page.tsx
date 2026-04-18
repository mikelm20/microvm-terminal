import * as React from "react";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import Link from "next/link";
import { coerceLang, fmt, isLang, voiceFor } from "@/lib/i18n";
import { getPublicProfile } from "@/lib/api";
import { SEED_PROFILE, SEED_PROFILE_UUID } from "@/lib/mock";
import { appDeepLink } from "@/lib/deep-link";

type PageParams = { lang: string; uuid: string; lessonId: string };

async function load(uuid: string) {
  try {
    const profile = await getPublicProfile(uuid);
    if (profile) return profile;
  } catch {
    // fall through
  }
  return uuid === SEED_PROFILE_UUID ? SEED_PROFILE : null;
}

export async function generateMetadata({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<Metadata> {
  const { lang: rawLang, uuid, lessonId } = await params;
  if (!isLang(rawLang)) return {};
  const lang = coerceLang(rawLang);
  const profile = await load(uuid);
  const mod = profile?.modules_completed.find((m) => m.lesson_id === lessonId);
  if (!mod) return { title: lang === "es" ? "Modulo" : "Module" };
  const voice = voiceFor(lang);
  const desc = fmt(voice.share.body_module, { moduleTitle: mod.title });
  return {
    title: `${mod.title} . ${profile?.name ?? "learn.example.com"}`,
    description: desc,
    openGraph: {
      title: mod.title,
      description: desc,
      images: [
        {
          url: `/og/module/${uuid}/${lessonId}`,
          width: 1200,
          height: 630,
        },
      ],
    },
    twitter: {
      card: "summary_large_image",
      title: mod.title,
      description: desc,
      images: [`/og/module/${uuid}/${lessonId}`],
    },
  };
}

export default async function ModuleProofPage({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<React.ReactElement> {
  const { lang: rawLang, uuid, lessonId } = await params;
  if (!isLang(rawLang)) notFound();
  const lang = coerceLang(rawLang);
  const profile = await load(uuid);
  if (!profile) notFound();
  const mod = profile.modules_completed.find((m) => m.lesson_id === lessonId);
  if (!mod) notFound();

  return (
    <article className="mx-auto max-w-3xl px-6 py-16 space-y-10">
      <header className="space-y-2">
        <p className="learn-eyebrow">
          {lang === "es" ? "Modulo terminado" : "Module completed"}
        </p>
        <h1 className="text-4xl leading-tight text-learn-warmHi">{mod.title}</h1>
        <p className="text-sm text-learn-warm">
          {lang === "es" ? "por" : "by"}{" "}
          <Link href={`/${lang}/p/${uuid}`} className="underline hover:text-learn-warmHi">
            {profile.name ?? (lang === "es" ? "un aprendiz" : "a learner")}
          </Link>
          {" . "}
          <time className="font-mono">{mod.completed_at.slice(0, 10)}</time>
        </p>
      </header>

      {mod.takeaways.length > 0 ? (
        <section className="learn-card space-y-3">
          <h2 className="text-2xl text-learn-warmHi">
            {lang === "es" ? "Lo que me llevo" : "Takeaways"}
          </h2>
          <ul className="space-y-2 text-learn-warm">
            {mod.takeaways.map((t, i) => (
              <li key={i} className="flex gap-3">
                <span aria-hidden className="text-learn-flame">.</span>
                <span>{t}</span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="flex flex-wrap gap-3">
        <Link href={appDeepLink(`/m/${lessonId}`)} className="learn-button-primary">
          {lang === "es" ? "Hazlo tu en la app" : "Try it in the app"}
        </Link>
        <Link href={`/${lang}/p/${uuid}`} className="learn-button-secondary">
          {lang === "es" ? "Ver el perfil completo" : "See the full profile"}
        </Link>
      </section>
    </article>
  );
}
