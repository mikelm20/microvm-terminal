import Link from "next/link";

export default function AutonomiaPage() {
  return (
    <main className="flex flex-1 flex-col gap-8 py-10">
      <nav className="text-sm text-ink-tertiary">
        <Link href="/" className="hover:text-ink-primary">
          Inicio
        </Link>
        <span className="mx-2">/</span>
        <span>Autonomia</span>
      </nav>

      <header className="flex flex-col gap-3">
        <span className="text-sm uppercase tracking-widest text-ink-tertiary">
          Autonomia
        </span>
        <h1 className="text-3xl font-semibold">Lleva Claude a tu maquina</h1>
        <p className="max-w-xl text-ink-secondary">
          Estas listo para usar Claude Code sin red de seguridad. Instala el
          cliente en tu equipo y sigue donde lo dejaste.
        </p>
      </header>

      <section className="flex flex-col gap-4 rounded-card bg-surface-raised p-6">
        <h2 className="text-xl font-semibold">Instalar Claude Code</h2>
        <p className="text-ink-secondary">
          Descarga el cliente, autentica con tu cuenta y arranca en el
          repositorio donde quieras trabajar.
        </p>
        <Link
          href="https://docs.anthropic.com/en/docs/claude-code"
          className="self-start rounded-pill bg-flame-primary px-4 py-2 text-sm text-chromatic-deep hover:bg-flame-glow"
        >
          Ver guia oficial
        </Link>
      </section>
    </main>
  );
}
