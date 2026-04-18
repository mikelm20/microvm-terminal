import * as React from "react";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { coerceLang, isLang, voiceFor } from "@/lib/i18n";
import { VoiceProvider } from "@/components/VoiceProvider";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";

type LayoutParams = { lang: string };

export async function generateStaticParams(): Promise<LayoutParams[]> {
  return [{ lang: "es" }, { lang: "en" }];
}

export async function generateMetadata({
  params,
}: {
  params: Promise<LayoutParams>;
}): Promise<Metadata> {
  const { lang: rawLang } = await params;
  if (!isLang(rawLang)) return {};
  const lang = coerceLang(rawLang);
  const voice = voiceFor(lang);
  return {
    title: voice.welcome.title,
    description: voice.welcome.subtitle,
    openGraph: {
      title: voice.welcome.title,
      description: voice.welcome.subtitle,
      locale: lang === "es" ? "es_ES" : "en_US",
    },
    alternates: {
      languages: {
        es: "/es",
        en: "/en",
      },
    },
  };
}

export default async function LangLayout({
  params,
  children,
}: {
  params: Promise<LayoutParams>;
  children: React.ReactNode;
}): Promise<React.ReactElement> {
  const { lang: rawLang } = await params;
  if (!isLang(rawLang)) notFound();
  const lang = coerceLang(rawLang);
  const voice = voiceFor(lang);
  return (
    <VoiceProvider lang={lang} voice={voice}>
      <div className="min-h-screen flex flex-col">
        <SiteHeader lang={lang} />
        <main className="flex-1">{children}</main>
        <SiteFooter lang={lang} />
      </div>
    </VoiceProvider>
  );
}
