"use client";

import { useEffect, useState } from "react";

type Auth = "checking" | "locked" | "unlocked";

export default function AuthGate({ children }: { children: React.ReactNode }) {
  const [auth, setAuth] = useState<Auth>("checking");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetch("/auth/check", { credentials: "same-origin" }).then((r) => {
      setAuth(r.ok ? "unlocked" : "locked");
    });
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      const res = await fetch("/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ password }),
      });
      if (res.status === 429) throw new Error("Demasiados intentos. Espera un minuto.");
      if (res.status === 401) throw new Error("Contrasena incorrecta.");
      if (!res.ok) throw new Error(`login fallo: ${res.status}`);
      setAuth("unlocked");
      setPassword("");
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }

  if (auth === "checking") {
    return (
      <main className="min-h-screen flex items-center justify-center bg-learn-burgundy">
        <p className="text-learn-warm/60 text-sm tracking-wider uppercase">cargando...</p>
      </main>
    );
  }

  if (auth === "locked") {
    return (
      <main className="min-h-screen flex items-center justify-center p-8 bg-learn-burgundy">
        <form onSubmit={submit} className="max-w-sm w-full space-y-4 text-center">
          <h1 className="text-3xl font-normal text-learn-warmHi tracking-tight">
            learn.example.com
          </h1>
          <p className="text-learn-warm/70 text-sm">
            MVP cerrado. Introduce la contrasena para entrar.
          </p>
          <input
            type="password"
            autoFocus
            required
            placeholder="contrasena"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="w-full px-3 py-2 rounded bg-black/30 border border-learn-warm/20 text-learn-warmHi placeholder-learn-warm/40 focus:outline-none focus:border-learn-warm/50"
          />
          <button
            type="submit"
            disabled={loading || !password}
            className="w-full bg-learn-warmHi text-learn-burgundy-mid font-medium py-2 rounded disabled:opacity-60"
          >
            {loading ? "Comprobando..." : "Entrar"}
          </button>
          {error && <p className="text-red-300 text-sm">{error}</p>}
        </form>
      </main>
    );
  }

  return <>{children}</>;
}
