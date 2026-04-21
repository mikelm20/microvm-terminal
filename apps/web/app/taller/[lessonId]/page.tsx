import Link from "next/link";
import Terminal from "../../../components/Terminal";
import WizardSidebar from "../../../components/WizardSidebar";

type Params = { lessonId: string };

export default async function TallerLessonPage({
  params,
}: {
  params: Promise<Params>;
}) {
  const { lessonId } = await params;

  return (
    <main className="flex flex-1 flex-col gap-6 py-6">
      <nav className="text-sm text-ink-tertiary">
        <Link href="/" className="hover:text-ink-primary">
          Inicio
        </Link>
        <span className="mx-2">/</span>
        <span>Taller</span>
        <span className="mx-2">/</span>
        <span className="text-ink-primary">{lessonId}</span>
      </nav>

      <header className="flex flex-col gap-1">
        <span className="text-sm uppercase tracking-widest text-ink-tertiary">
          Terminal guiada
        </span>
        <h1 className="text-2xl font-semibold">Leccion {lessonId}</h1>
      </header>

      <div className="flex flex-1 flex-col gap-4 lg:flex-row">
        <section className="order-1 flex min-h-[360px] flex-1 flex-col rounded-card border border-surface-divider bg-surface-sunken p-3 lg:order-1">
          <Terminal lessonId={lessonId} />
        </section>
        <aside className="order-2 flex w-full flex-col gap-3 rounded-card border border-surface-divider bg-surface-raised p-4 lg:order-2 lg:w-96">
          <WizardSidebar lessonId={lessonId} />
        </aside>
      </div>
    </main>
  );
}
