"use client";

import { useEffect, useState } from "react";
import type { WsEvent } from "@learn/shared-api/events";
import { connectWizard } from "../lib/ws";

type Step = {
  id: string;
  title: string;
  satisfied: boolean;
};

type WizardSidebarProps = {
  lessonId: string;
  wizardWsUrl?: string;
  initialSteps?: Step[];
};

const PLACEHOLDER_STEPS: Step[] = [
  { id: "step-1", title: "Abre la terminal", satisfied: false },
  { id: "step-2", title: "Manda tu primer prompt", satisfied: false },
  { id: "step-3", title: "Observa los tool calls", satisfied: false },
];

export default function WizardSidebar({
  lessonId,
  wizardWsUrl,
  initialSteps,
}: WizardSidebarProps) {
  const [steps, setSteps] = useState<Step[]>(initialSteps ?? PLACEHOLDER_STEPS);
  const [lastEvent, setLastEvent] = useState<WsEvent | null>(null);
  const [status, setStatus] = useState<string>("Esperando conexion");

  useEffect(() => {
    if (!wizardWsUrl) {
      setStatus("Modo local, sin VM");
      return;
    }
    const handle = connectWizard(wizardWsUrl, {
      onOpen: () => setStatus("Conectado"),
      onClose: () => setStatus("Desconectado"),
      onEvent: (ev) => {
        setLastEvent(ev);
        if (ev.type === "step_satisfied") {
          setSteps((prev) =>
            prev.map((s) =>
              s.id === ev.step_id ? { ...s, satisfied: true } : s,
            ),
          );
        }
      },
    });
    return () => handle.close();
  }, [wizardWsUrl]);

  return (
    <div className="flex flex-col gap-4">
      <header className="flex flex-col gap-1">
        <span className="text-xs uppercase tracking-widest text-ink-tertiary">
          Tu progreso
        </span>
        <h2 className="text-lg font-semibold">{lessonId}</h2>
        <span className="text-xs text-ink-tertiary">{status}</span>
      </header>

      <ol className="flex flex-col gap-2">
        {steps.map((step, idx) => (
          <li
            key={step.id}
            className={`flex items-center gap-3 rounded-chip border border-surface-divider px-3 py-2 text-sm ${
              step.satisfied
                ? "bg-signal-success/20 text-ink-primary"
                : "text-ink-secondary"
            }`}
          >
            <span className="inline-flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-pill bg-surface-sunken text-xs">
              {idx + 1}
            </span>
            <span className="flex-1">{step.title}</span>
            {step.satisfied ? <span aria-hidden>Listo</span> : null}
          </li>
        ))}
      </ol>

      {lastEvent ? (
        <pre className="max-h-32 overflow-auto rounded-chip bg-surface-sunken p-2 text-[11px] text-ink-tertiary">
          {JSON.stringify(lastEvent, null, 2)}
        </pre>
      ) : null}
    </div>
  );
}
