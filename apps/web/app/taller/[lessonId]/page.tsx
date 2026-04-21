import Link from "next/link";
import { notFound } from "next/navigation";
import LessonRuntime from "../../../components/LessonRuntime";
import { loadLessonById, loadPillars } from "../../../lib/registry";

export const dynamic = "force-dynamic";

type Params = { lessonId: string };

export default async function TallerLessonPage({
  params,
}: {
  params: Promise<Params>;
}) {
  const { lessonId } = await params;
  const lesson = await loadLessonById(lessonId);
  if (!lesson) {
    notFound();
  }

  // Resolve pillar membership + next lesson in the same pillar for the
  // completion moment. Missing registry simply yields empty pillars and the
  // header falls back to "Terminal guiada".
  const pillars = await loadPillars();
  const pillar = pillars.find((p) => p.lessons.some((l) => l.id === lesson.id));
  const pillarTitle = pillar?.title ?? "Terminal guiada";
  let nextLessonId: string | null = null;
  let nextLessonTitle: string | null = null;
  if (pillar) {
    const idx = pillar.lessons.findIndex((l) => l.id === lesson.id);
    const next = idx >= 0 ? pillar.lessons[idx + 1] : undefined;
    if (next) {
      nextLessonId = next.id;
      nextLessonTitle = next.title;
    }
  }

  return (
    <main className="flex flex-1 flex-col gap-6 py-6">
      <nav className="text-sm text-ink-tertiary">
        <Link href="/" className="hover:text-ink-primary">
          Inicio
        </Link>
        <span className="mx-2">/</span>
        <Link href="/taller" className="hover:text-ink-primary">
          Taller
        </Link>
        <span className="mx-2">/</span>
        <span className="text-ink-primary">{lesson.title}</span>
      </nav>

      <header className="flex flex-col gap-1">
        <span className="text-sm uppercase tracking-widest text-ink-tertiary">
          {pillarTitle}
        </span>
        <h1 className="text-2xl font-semibold tracking-tight">{lesson.title}</h1>
        {lesson.subtitle ? (
          <p className="text-sm text-ink-secondary">{lesson.subtitle}</p>
        ) : null}
      </header>

      <LessonRuntime
        lesson={lesson}
        pillarTitle={pillarTitle}
        nextLessonId={nextLessonId}
        nextLessonTitle={nextLessonTitle}
      />
    </main>
  );
}
