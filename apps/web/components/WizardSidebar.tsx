"use client";

import Link from "next/link";
import { useEffect, useMemo, useRef, useState } from "react";
import type { WsEvent } from "@learn/shared-api/events";
import type { Lesson, LessonStep } from "@learn/shared-lessons";
import { connectWizard } from "../lib/ws";
import { markStepCompleted, readLessonProgress, touchLesson } from "../lib/progress";
import { track } from "../lib/track";
import MascadoPrompt from "./MascadoPrompt";

type WizardSidebarProps = {
  lesson: Lesson;
  pillarTitle?: string;
  wizardWsUrl?: string;
  nextLessonId?: string | null;
  nextLessonTitle?: string | null;
};

type WsStatus = "offline" | "connecting" | "connected" | "disconnected";

export default function WizardSidebar({
  lesson,
  pillarTitle,
  wizardWsUrl,
  nextLessonId,
  nextLessonTitle,
}: WizardSidebarProps) {
  const steps = lesson.steps;
  const [completedSet, setCompletedSet] = useState<Set<string>>(() => new Set());
  const [status, setStatus] = useState<WsStatus>("offline");
  const openedRef = useRef(false);

  // Hydrate progress from localStorage on mount and record that the lesson
  // was opened. Also emit the telemetry event once per mount.
  useEffect(() => {
    const progress = readLessonProgress(lesson.id);
    setCompletedSet(new Set(progress.completedStepIds));
    touchLesson(lesson.id);
    if (!openedRef.current) {
      openedRef.current = true;
      void track("lesson_opened", { lesson_id: lesson.id });
    }
  }, [lesson.id]);

  // WS wiring, only if we have a URL. Synthesized step_satisfied (via the
  // Siguiente button) is the offline fallback.
  useEffect(() => {
    if (!wizardWsUrl) {
      setStatus("offline");
      return;
    }
    setStatus("connecting");
    const handle = connectWizard(wizardWsUrl, {
      onOpen: () => setStatus("connected"),
      onClose: () => setStatus("disconnected"),
      onEvent: (ev: WsEvent) => {
        if (ev.type === "step_satisfied" && ev.lesson_id === lesson.id) {
          advance(ev.step_id, "ws");
        }
      },
    });
    return () => handle.close();
  }, [wizardWsUrl, lesson.id]);

  function advance(stepId: string, source: "ws" | "manual"): void {
    setCompletedSet((prev) => {
      if (prev.has(stepId)) return prev;
      const next = new Set(prev);
      next.add(stepId);
      markStepCompleted(lesson.id, stepId);
      void track("step_satisfied", {
        lesson_id: lesson.id,
        step_id: stepId,
        source,
      });
      return next;
    });
  }

  const currentIdx = useMemo(() => {
    const idx = steps.findIndex((s) => !completedSet.has(s.id));
    return idx === -1 ? steps.length : idx;
  }, [steps, completedSet]);

  const allDone = currentIdx >= steps.length;
  const currentStep: LessonStep | null = allDone ? null : (steps[currentIdx] ?? null);

  // Fire lesson_completed exactly once, on the tick where we cross the
  // finish line.
  const completedEmittedRef = useRef(false);
  useEffect(() => {
    if (allDone && !completedEmittedRef.current) {
      completedEmittedRef.current = true;
      void track("lesson_completed", { lesson_id: lesson.id });
    }
  }, [allDone, lesson.id]);

  const statusLabel: Record<WsStatus, string> = {
    offline: "Modo local",
    connecting: "Conectando",
    connected: "Conectado",
    disconnected: "Reconectando",
  };

  const stepPosition = allDone
    ? `${steps.length} de ${steps.length}`
    : `${currentIdx + 1} de ${steps.length}`;

  return (
    <div className="flex flex-col gap-5">
      <header className="flex flex-col gap-1">
        {pillarTitle ? (
          <span className="text-xs uppercase tracking-widest text-ink-tertiary">
            {pillarTitle}
          </span>
        ) : null}
        <h2 className="text-lg font-semibold leading-tight">{lesson.title}</h2>
        {lesson.subtitle ? (
          <p className="text-sm text-ink-secondary">{lesson.subtitle}</p>
        ) : null}
      </header>

      <div className="flex items-center justify-between text-xs text-ink-tertiary">
        <span>Paso {stepPosition}</span>
        <span>{statusLabel[status]}</span>
      </div>

      <ol className="flex flex-col gap-2">
        {steps.map((step, idx) => {
          const done = completedSet.has(step.id);
          const active = idx === currentIdx;
          return (
            <li
              key={step.id}
              className={`flex items-center gap-3 rounded-chip border px-3 py-2 text-sm transition-colors ${
                done
                  ? "border-signal-success/40 bg-signal-success/10 text-ink-primary"
                  : active
                  ? "border-flame-primary/60 bg-chromatic-burgundyTop text-ink-primary"
                  : "border-surface-divider text-ink-secondary"
              }`}
            >
              <span className="inline-flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-pill bg-surface-sunken text-xs">
                {done ? "OK" : idx + 1}
              </span>
              <span className="flex-1 leading-snug">{step.title}</span>
            </li>
          );
        })}
      </ol>

      {currentStep ? (
        <div className="flex flex-col gap-3 rounded-card bg-surface-sunken p-4">
          <div className="flex flex-col gap-1">
            <span className="text-xs uppercase tracking-widest text-ink-tertiary">
              Paso actual
            </span>
            <h3 className="text-base font-semibold leading-snug">
              {currentStep.title}
            </h3>
            {currentStep.body ? (
              <p className="mt-1 text-sm leading-relaxed text-ink-secondary">
                {currentStep.body}
              </p>
            ) : null}
          </div>
          {currentStep.prompt ? (
            <MascadoPrompt
              lessonId={lesson.id}
              stepId={currentStep.id}
              prompt={currentStep.prompt}
              hint={currentStep.hint}
            />
          ) : currentStep.hint ? (
            <p className="text-sm leading-relaxed text-ink-tertiary">
              {currentStep.hint}
            </p>
          ) : null}

          <button
            type="button"
            onClick={() => advance(currentStep.id, "manual")}
            disabled={status === "connected"}
            className="mt-1 rounded-pill bg-flame-primary px-4 py-2 text-sm font-medium text-chromatic-deep transition-colors hover:bg-flame-glow disabled:cursor-not-allowed disabled:bg-surface-sunken disabled:text-ink-tertiary"
          >
            {status === "connected" ? "Esperando a Claude" : "Siguiente"}
          </button>
          {status === "connected" ? (
            <span className="text-xs text-ink-tertiary">
              Se avanza cuando llega el evento.
            </span>
          ) : null}
        </div>
      ) : (
        <div className="flex flex-col gap-3 rounded-card border border-signal-success/30 bg-signal-success/10 p-4">
          <span className="text-sm font-semibold text-ink-primary">
            Leccion completada
          </span>
          <p className="text-sm text-ink-secondary">
            Has cerrado los {steps.length} pasos de {lesson.title}.
          </p>
          <div className="flex flex-wrap gap-2">
            <Link
              href="/taller"
              className="rounded-pill border border-surface-divider px-4 py-2 text-sm text-ink-secondary hover:text-ink-primary"
            >
              Volver al taller
            </Link>
            {nextLessonId ? (
              <Link
                href={`/taller/${encodeURIComponent(nextLessonId)}`}
                className="rounded-pill bg-flame-primary px-4 py-2 text-sm font-medium text-chromatic-deep hover:bg-flame-glow"
              >
                Siguiente: {nextLessonTitle ?? nextLessonId}
              </Link>
            ) : null}
          </div>
        </div>
      )}
    </div>
  );
}
