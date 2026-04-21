import Link from "next/link";
import LessonCard from "../../components/LessonCard";
import PillarCard from "../../components/PillarCard";
import { loadPillars } from "../../lib/registry";

export const dynamic = "force-dynamic";

export default async function TallerIndexPage() {
  const pillars = await loadPillars();
  const anyLessons = pillars.some((p) => p.lessons.length > 0);

  return (
    <main className="flex flex-1 flex-col gap-10 py-10">
      <nav className="text-sm text-ink-tertiary">
        <Link href="/" className="hover:text-ink-primary">
          Inicio
        </Link>
        <span className="mx-2">/</span>
        <span className="text-ink-primary">Taller</span>
      </nav>

      <header className="flex flex-col gap-3">
        <span className="text-sm uppercase tracking-widest text-ink-tertiary">
          Terminal guiada
        </span>
        <h1 className="text-3xl font-semibold tracking-tight">
          Tres pilares, en orden
        </h1>
        <p className="max-w-2xl text-ink-secondary">
          Comandos primero, luego a darle contexto a Claude, y al final a
          pedirle bien. Haz uno a la vez. Cada leccion es corta.
        </p>
      </header>

      {anyLessons ? (
        <section className="flex flex-col gap-6">
          {pillars.map((pillar, idx) => (
            <PillarCard
              key={pillar.id}
              index={idx + 1}
              title={pillar.title}
              subtitle={pillar.subtitle}
            >
              {pillar.lessons.length === 0 ? (
                <p className="text-sm text-ink-tertiary">
                  Contenido en preparacion para este pilar.
                </p>
              ) : (
                pillar.lessons.map((lesson) => (
                  <LessonCard
                    key={lesson.id}
                    lessonId={lesson.id}
                    title={lesson.title}
                    subtitle={lesson.subtitle}
                    estimatedMinutes={lesson.estimated_minutes}
                    stepCount={lesson.steps.length}
                  />
                ))
              )}
            </PillarCard>
          ))}
        </section>
      ) : (
        <section className="rounded-card border border-surface-divider bg-surface-raised p-6">
          <h2 className="text-lg font-semibold">Contenido en preparacion</h2>
          <p className="mt-2 text-sm text-ink-secondary">
            Aun no hay lecciones publicadas en los tres pilares del taller.
            Vuelve pronto.
          </p>
          <Link
            href="/"
            className="mt-4 inline-flex rounded-pill border border-surface-divider px-4 py-2 text-sm text-ink-secondary hover:text-ink-primary"
          >
            Volver al inicio
          </Link>
        </section>
      )}
    </main>
  );
}
