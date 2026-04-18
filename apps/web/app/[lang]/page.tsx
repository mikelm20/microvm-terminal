import * as React from "react";
import Link from "next/link";
import { coerceLang, isLang, voiceFor } from "@/lib/i18n";
import { APP_STORE_URL, PLAY_STORE_URL } from "@/lib/deep-link";
import { notFound } from "next/navigation";

type PageParams = { lang: string };

export default async function LandingPage({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<React.ReactElement> {
  const { lang: rawLang } = await params;
  if (!isLang(rawLang)) notFound();
  const lang = coerceLang(rawLang);
  const voice = voiceFor(lang);
  return (
    <section className="mx-auto max-w-3xl px-6 py-20 sm:py-28">
      <p className="learn-eyebrow">
        {lang === "es" ? "Aprende Claude Code" : "Learn Claude Code"}
      </p>
      <h1 className="mt-5 text-4xl sm:text-5xl leading-[1.04] tracking-[-0.02em] text-learn-warmHi">
        {voice.welcome.title}
      </h1>
      <p className="mt-5 text-lg text-learn-warm max-w-xl">
        {voice.welcome.subtitle}
      </p>

      <div className="mt-10 flex flex-wrap gap-3">
        <Link
          href={APP_STORE_URL}
          className="learn-button-primary"
          rel="noreferrer"
          target="_blank"
        >
          {lang === "es" ? "Descargar en iOS" : "Download on iOS"}
        </Link>
        <Link
          href={PLAY_STORE_URL}
          className="learn-button-secondary"
          rel="noreferrer"
          target="_blank"
        >
          {lang === "es" ? "Android en Play Store" : "Android on Play Store"}
        </Link>
      </div>

      <p className="mt-8 text-sm text-learn-warm/80 max-w-lg">
        {lang === "es"
          ? "Tres minutos al dia. Claude ejecuta en tu propia maquina, fresca y solo tuya. Nunca ve tu codigo ni tus datos."
          : "Three minutes a day. Claude runs on your own machine, fresh and only yours. It never sees your code or your data."}
      </p>
    </section>
  );
}
