import * as React from "react";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { coerceLang, isLang } from "@/lib/i18n";
import { StatusLive } from "@/components/StatusLive";

type PageParams = { lang: string };

export async function generateMetadata({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<Metadata> {
  const { lang: rawLang } = await params;
  if (!isLang(rawLang)) return {};
  const lang = coerceLang(rawLang);
  return { title: lang === "es" ? "Estado" : "Status" };
}

export default async function StatusPage({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<React.ReactElement> {
  const { lang: rawLang } = await params;
  if (!isLang(rawLang)) notFound();
  const lang = coerceLang(rawLang);
  return (
    <section className="mx-auto max-w-3xl px-6 py-16 space-y-8">
      <header>
        <p className="learn-eyebrow">{lang === "es" ? "Estado" : "Status"}</p>
        <h1 className="mt-3 text-4xl leading-tight text-learn-warmHi">
          {lang === "es" ? "Que esta funcionando ahora" : "What is running right now"}
        </h1>
        <p className="mt-3 text-learn-warm max-w-xl">
          {lang === "es"
            ? "Si algo no va, lo veras aqui antes que en ningun otro sitio."
            : "If something is off, you will see it here before anywhere else."}
        </p>
      </header>
      <StatusLive lang={lang} />
    </section>
  );
}
