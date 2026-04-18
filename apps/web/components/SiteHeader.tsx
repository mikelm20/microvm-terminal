import * as React from "react";
import Link from "next/link";
import type { Lang } from "@/lib/i18n";

export function SiteHeader({ lang }: { lang: Lang }): React.ReactElement {
  const otherLang: Lang = lang === "es" ? "en" : "es";
  const otherLabel = otherLang.toUpperCase();
  const navLabels =
    lang === "es"
      ? {
          about: "Sobre nosotros",
          status: "Estado",
          changelog: "Cambios",
        }
      : {
          about: "About",
          status: "Status",
          changelog: "Changelog",
        };
  return (
    <header className="w-full border-b border-white/10">
      <div className="mx-auto max-w-5xl flex items-center justify-between px-6 py-5">
        <Link href={`/${lang}`} className="flex items-center gap-3">
          <span
            aria-hidden
            className="inline-block h-5 w-5 rounded-full"
            style={{
              backgroundImage:
                "linear-gradient(135deg, #ffc591 0%, #ff9b5a 50%, #e6724a 100%)",
              boxShadow: "0 0 0 4px rgba(255, 197, 145, 0.18)",
            }}
          />
          <span className="text-learn-warmHi text-sm tracking-[0.18em] uppercase">
            learn.example.com
          </span>
        </Link>
        <nav className="flex items-center gap-5 text-sm text-learn-warm">
          <Link href={`/${lang}/about`} className="hover:text-learn-warmHi">
            {navLabels.about}
          </Link>
          <Link href={`/${lang}/status`} className="hover:text-learn-warmHi">
            {navLabels.status}
          </Link>
          <Link href={`/${lang}/changelog`} className="hover:text-learn-warmHi">
            {navLabels.changelog}
          </Link>
          <Link
            href={`/${otherLang}`}
            aria-label={`Switch to ${otherLabel}`}
            className="hover:text-learn-warmHi"
          >
            {otherLabel}
          </Link>
        </nav>
      </div>
    </header>
  );
}
