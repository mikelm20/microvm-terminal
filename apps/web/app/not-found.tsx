import * as React from "react";
import Link from "next/link";

export default function NotFound(): React.ReactElement {
  return (
    <main className="min-h-screen flex items-center justify-center px-6">
      <section className="max-w-md text-center space-y-4">
        <p className="learn-eyebrow">404</p>
        <h1 className="text-3xl text-learn-warmHi">Aqui no hay nada</h1>
        <p className="text-learn-warm">
          La direccion que buscas no existe o expiro. Vuelve al principio.
        </p>
        <div className="pt-2">
          <Link href="/es" className="learn-button-primary">
            Empezar
          </Link>
        </div>
      </section>
    </main>
  );
}
