import Link from "next/link";

type Params = { stepId: string };

export default async function FundamentosStepPage({
  params,
}: {
  params: Promise<Params>;
}) {
  const { stepId } = await params;
  return (
    <main className="flex flex-1 flex-col gap-8 py-10">
      <nav className="text-sm text-ink-tertiary">
        <Link href="/" className="hover:text-ink-primary">
          Inicio
        </Link>
        <span className="mx-2">/</span>
        <span>Fundamentos</span>
      </nav>

      <header className="flex flex-col gap-2">
        <span className="text-sm uppercase tracking-widest text-ink-tertiary">
          Fundamentos
        </span>
        <h1 className="text-3xl font-semibold">Paso {stepId}</h1>
        <p className="text-ink-secondary">
          Lee con calma. Luego sigues al siguiente paso.
        </p>
      </header>

      <article className="rounded-card bg-surface-raised p-6 text-ink-primary">
        <p>
          Aqui va el contenido conceptual del paso. Sin terminal. Se presenta en
          tarjetas cortas con un solo punto claro por tarjeta.
        </p>
      </article>

      <footer className="flex items-center justify-between">
        <Link
          href="/"
          className="rounded-pill border border-surface-divider px-4 py-2 text-sm text-ink-secondary hover:text-ink-primary"
        >
          Volver
        </Link>
        <Link
          href="/taller/hello-claude"
          className="rounded-pill bg-flame-primary px-4 py-2 text-sm text-chromatic-deep hover:bg-flame-glow"
        >
          Siguiente
        </Link>
      </footer>
    </main>
  );
}
