import * as React from "react";
import Link from "next/link";
import type { Lang } from "@/lib/i18n";

export function SiteFooter({ lang }: { lang: Lang }): React.ReactElement {
  const labels =
    lang === "es"
      ? {
          tagline: "Platform Engineering LLC . Aprende Claude Code usandolo.",
          about: "Sobre nosotros",
          status: "Estado",
          changelog: "Cambios",
        }
      : {
          tagline: "Platform Engineering LLC . Learn Claude Code by using it.",
          about: "About",
          status: "Status",
          changelog: "Changelog",
        };
  return (
    <footer className="border-t border-white/10 mt-16">
      <div className="mx-auto max-w-5xl flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 px-6 py-8 text-sm text-learn-warm">
        <div>{labels.tagline}</div>
        <nav className="flex items-center gap-5">
          <Link href={`/${lang}/about`} className="hover:text-learn-warmHi">
            {labels.about}
          </Link>
          <Link href={`/${lang}/status`} className="hover:text-learn-warmHi">
            {labels.status}
          </Link>
          <Link href={`/${lang}/changelog`} className="hover:text-learn-warmHi">
            {labels.changelog}
          </Link>
        </nav>
      </div>
    </footer>
  );
}
