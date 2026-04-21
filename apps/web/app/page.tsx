import Link from "next/link";
import { t } from "@learn/shared-voice";

type Phase = {
  href: string;
  label: string;
  description: string;
};

const phases: Phase[] = [
  {
    href: "/fundamentos/1",
    label: "Fundamentos",
    description: "Las ideas base. Sin terminal, sin prisa.",
  },
  {
    href: "/taller/hello-claude",
    label: "Terminal guiada",
    description: "Tu primer prompt con Claude. Los tool calls en vivo.",
  },
  {
    href: "/autonomia",
    label: "Autonomia",
    description: "Lleva Claude Code a tu maquina.",
  },
];

export default function HomePage() {
  const lang = "es" as const;
  return (
    <main className="flex flex-1 flex-col gap-12 py-10">
      <header className="flex flex-col gap-3">
        <h1 className="text-4xl font-semibold tracking-tight">
          {t(lang, "welcome.title")}
        </h1>
        <p className="text-lg text-ink-secondary">
          {t(lang, "welcome.subtitle")}
        </p>
      </header>

      <section className="grid gap-4 md:grid-cols-3">
        {phases.map((phase) => (
          <Link
            key={phase.href}
            href={phase.href}
            className="flex flex-col gap-2 rounded-card border border-surface-divider bg-surface-raised p-6 transition-colors hover:bg-chromatic-burgundyTop"
          >
            <span className="text-sm uppercase tracking-widest text-ink-tertiary">
              {phase.label}
            </span>
            <span className="text-xl text-ink-primary">{phase.description}</span>
          </Link>
        ))}
      </section>
    </main>
  );
}
