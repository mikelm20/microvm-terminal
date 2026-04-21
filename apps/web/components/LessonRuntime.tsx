"use client";

import { useEffect, useState } from "react";
import type { Lesson } from "@learn/shared-lessons";
import { createSession } from "../lib/api";
import { ensureIdentity } from "../lib/identity";
import { track } from "../lib/track";
import Terminal from "./Terminal";
import WizardSidebar from "./WizardSidebar";

type LessonRuntimeProps = {
  lesson: Lesson;
  pillarTitle?: string;
  nextLessonId?: string | null;
  nextLessonTitle?: string | null;
};

type SessionState = {
  ptyWsUrl?: string;
  wizardWsUrl?: string;
};

export default function LessonRuntime({
  lesson,
  pillarTitle,
  nextLessonId,
  nextLessonTitle,
}: LessonRuntimeProps) {
  const [session, setSession] = useState<SessionState>({});
  const [wizardOpenOnMobile, setWizardOpenOnMobile] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function mint(): Promise<void> {
      try {
        // The identity helper already no-ops gracefully when the API is down.
        await ensureIdentity(lesson.language);
        const res = await createSession({
          lesson_id: lesson.id,
          lang: lesson.language,
        });
        if (cancelled) return;
        setSession({
          ptyWsUrl: res.pty_ws_url,
          wizardWsUrl: res.wizard_ws_url,
        });
      } catch (err) {
        // Stay in stub mode. Track so we can see the fallback rate later.
        void track("session_stub_fallback", {
          lesson_id: lesson.id,
          reason: err instanceof Error ? err.message : String(err),
        });
      }
    }

    void mint();
    return () => {
      cancelled = true;
    };
  }, [lesson.id, lesson.language]);

  return (
    <div className="flex flex-1 flex-col gap-4 lg:flex-row">
      <section className="order-1 flex min-h-[320px] flex-1 flex-col rounded-card border border-surface-divider bg-surface-sunken p-3 lg:min-h-[520px]">
        <Terminal lessonId={lesson.id} ptyWsUrl={session.ptyWsUrl} />
      </section>

      <aside className="order-2 flex w-full flex-col gap-3 rounded-card border border-surface-divider bg-surface-raised p-4 lg:w-[360px] lg:flex-shrink-0">
        <button
          type="button"
          onClick={() => setWizardOpenOnMobile((v) => !v)}
          className="flex items-center justify-between rounded-chip bg-surface-sunken px-3 py-2 text-xs uppercase tracking-widest text-ink-tertiary lg:hidden"
          aria-expanded={wizardOpenOnMobile}
        >
          <span>Guia</span>
          <span>{wizardOpenOnMobile ? "Ocultar" : "Mostrar"}</span>
        </button>
        <div className={wizardOpenOnMobile ? "flex flex-col gap-3" : "hidden lg:flex lg:flex-col lg:gap-3"}>
          <WizardSidebar
            lesson={lesson}
            pillarTitle={pillarTitle}
            wizardWsUrl={session.wizardWsUrl}
            nextLessonId={nextLessonId}
            nextLessonTitle={nextLessonTitle}
          />
        </div>
      </aside>
    </div>
  );
}
