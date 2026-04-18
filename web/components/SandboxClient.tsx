"use client";

import Link from "next/link";
import { useState } from "react";
import dynamic from "next/dynamic";
import WizardSidebar from "@/components/WizardSidebar";

// xterm.js touches `self` at import time, which breaks Next.js SSR.
// Load the Terminal component only on the client.
const Terminal = dynamic(() => import("@/components/Terminal"), { ssr: false });

type Session = { id: string; ptyPath: string; wizardPath: string };

export default function SandboxClient() {
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function launch() {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch("/sessions", {
        method: "POST",
        credentials: "same-origin",
      });
      if (!res.ok) {
        throw new Error(`launch failed: ${res.status} ${await res.text()}`);
      }
      const data = await res.json();
      setSession({
        id: data.session_id,
        ptyPath: data.pty_path,
        wizardPath: data.wizard_path,
      });
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }

  async function endSession() {
    if (!session) return;
    try {
      await fetch(`/sessions/${session.id}`, {
        method: "DELETE",
        credentials: "same-origin",
      });
    } catch {
      /* best effort */
    }
    setSession(null);
  }

  if (!session) {
    return (
      <main className="min-h-screen flex items-center justify-center p-8 bg-learn-burgundy text-learn-warm">
        <div className="max-w-xl text-center space-y-6">
          <Link
            href="/"
            className="inline-block text-[11px] tracking-[0.22em] uppercase text-learn-warm/70 hover:text-learn-warmHi"
          >
            ← learn.example.com
          </Link>
          <h1 className="text-4xl font-normal text-learn-warmHi tracking-tight">
            Sandbox libre
          </h1>
          <p className="text-learn-warm/70">
            Una VM Linux real en tu navegador, con Claude Code preinstalado.
            Sin guion, sin lecciones. Si prefieres aprender paso a paso,{" "}
            <Link
              href="/lessons"
              className="text-learn-warmHi underline underline-offset-2"
            >
              ve al camino guiado
            </Link>
            .
          </p>
          <div>
            <button
              onClick={launch}
              disabled={loading}
              className="bg-learn-warmHi text-learn-burgundy-mid font-medium px-8 py-3 rounded-full disabled:opacity-60 shadow-[0_12px_36px_rgba(0,0,0,0.22)]"
            >
              {loading ? "Arrancando VM..." : "Lanzar sandbox"}
            </button>
          </div>
          {error && <p className="text-red-300 text-sm">{error}</p>}
          <p className="text-learn-warm/40 text-xs">
            MVP: 2 GB RAM, 2 vCPU. Efimera: la sandbox se destruye al cerrar
            la pestana.
          </p>
        </div>
      </main>
    );
  }

  return (
    <main className="h-screen flex bg-learn-bg">
      <WizardSidebar
        onEnd={endSession}
        sessionId={session.id}
        wsPath={session.wizardPath}
      />
      <div className="flex-1 term-wrap">
        <Terminal wsPath={session.ptyPath} />
      </div>
    </main>
  );
}
