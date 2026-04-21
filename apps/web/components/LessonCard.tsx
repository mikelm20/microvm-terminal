import Link from "next/link";

type LessonCardProps = {
  lessonId: string;
  title: string;
  subtitle?: string;
  estimatedMinutes?: number;
  stepCount: number;
};

export default function LessonCard({
  lessonId,
  title,
  subtitle,
  estimatedMinutes,
  stepCount,
}: LessonCardProps) {
  return (
    <Link
      href={`/taller/${encodeURIComponent(lessonId)}`}
      className="flex flex-col gap-2 rounded-chip border border-surface-divider bg-surface-sunken px-4 py-3 transition-colors hover:bg-chromatic-burgundyTop"
    >
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-base font-medium text-ink-primary">{title}</span>
        <span className="flex-shrink-0 text-xs text-ink-tertiary">
          {stepCount} paso{stepCount === 1 ? "" : "s"}
          {estimatedMinutes ? ` / ${estimatedMinutes} min` : ""}
        </span>
      </div>
      {subtitle ? (
        <span className="text-sm leading-snug text-ink-secondary">
          {subtitle}
        </span>
      ) : null}
    </Link>
  );
}
