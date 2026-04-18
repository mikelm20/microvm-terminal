import * as React from "react";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { coerceLang, isLang, type Lang } from "@/lib/i18n";
import { parseSimpleMarkdown } from "@/lib/markdown";

type PageParams = { lang: string };

export async function generateMetadata({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<Metadata> {
  const { lang: rawLang } = await params;
  if (!isLang(rawLang)) return {};
  const lang = coerceLang(rawLang);
  return { title: lang === "es" ? "Cambios" : "Changelog" };
}

async function loadChangelog(lang: Lang): Promise<string> {
  const path = join(process.cwd(), "content", `changelog.${lang}.md`);
  return readFile(path, "utf8");
}

export default async function ChangelogPage({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<React.ReactElement> {
  const { lang: rawLang } = await params;
  if (!isLang(rawLang)) notFound();
  const lang = coerceLang(rawLang);
  const raw = await loadChangelog(lang);
  const blocks = parseSimpleMarkdown(raw);

  return (
    <section className="mx-auto max-w-3xl px-6 py-16 space-y-8">
      <header>
        <p className="learn-eyebrow">{lang === "es" ? "Cambios" : "Changelog"}</p>
        <h1 className="mt-3 text-4xl leading-tight text-learn-warmHi">
          {lang === "es" ? "Lo ultimo que cambiamos" : "What we changed lately"}
        </h1>
      </header>
      <div className="space-y-6 text-learn-warm">
        {blocks.map((block, i) => {
          if (block.kind === "h2") {
            return (
              <h2 key={i} className="text-2xl text-learn-warmHi border-l-2 border-learn-flame pl-4">
                {block.text}
              </h2>
            );
          }
          return (
            <p key={i} className="max-w-2xl">
              {block.text}
            </p>
          );
        })}
      </div>
    </section>
  );
}
