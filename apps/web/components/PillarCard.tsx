import type { ReactNode } from "react";

type PillarCardProps = {
  index: number;
  title: string;
  subtitle: string;
  children: ReactNode;
};

export default function PillarCard({
  index,
  title,
  subtitle,
  children,
}: PillarCardProps) {
  return (
    <section className="flex flex-col gap-4 rounded-card border border-surface-divider bg-surface-raised p-6">
      <header className="flex items-baseline gap-3">
        <span className="inline-flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-pill bg-chromatic-deep text-sm text-flame-primary">
          {index}
        </span>
        <div className="flex flex-col gap-1">
          <h2 className="text-xl font-semibold tracking-tight">{title}</h2>
          <p className="text-sm text-ink-tertiary">{subtitle}</p>
        </div>
      </header>
      <div className="flex flex-col gap-2">{children}</div>
    </section>
  );
}
