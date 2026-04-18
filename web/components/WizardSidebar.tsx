"use client";

import { useEffect, useState } from "react";

type Step = {
  id: string;
  title: string;
  hint: string;
  predicate: (events: WizardEvent[]) => boolean;
};

type WizardEvent = {
  type: string;
  ts: string;
  payload?: Record<string, unknown>;
};

// Phase-1 heuristics. Real predicates live server-side once the lesson engine
// lands; for now the frontend evaluates the event stream directly.
const steps: Step[] = [
  {
    id: "agent-online",
    title: "Sandbox is ready",
    hint: "The guest agent inside the VM has connected. You can start typing in the terminal.",
    predicate: (evs) => evs.some((e) => e.type === "agent_online"),
  },
  {
    id: "talk-to-claude",
    title: "Start Claude Code",
    hint: "Type `claude` in the terminal and press Enter. Say hi, or ask it to build something.",
    predicate: (evs) =>
      evs.some(
        (e) =>
          e.type === "process_started" &&
          typeof e.payload?.name === "string" &&
          (e.payload.name as string) === "claude",
      ),
  },
  {
    id: "run-something",
    title: "Run a dev server",
    hint: "Once Claude has built something, start it. A port will open, we'll detect it here.",
    predicate: (evs) => evs.some((e) => e.type === "port_listening"),
  },
];

export default function WizardSidebar({
  sessionId,
  wsPath,
  onEnd,
}: {
  sessionId: string;
  wsPath: string;
  onEnd: () => void;
}) {
  const [events, setEvents] = useState<WizardEvent[]>([]);
  const [agentOnline, setAgentOnline] = useState(false);

  // First opened port we detect -> show a preview link.
  const previewPort = (() => {
    for (const e of events) {
      if (e.type === "port_listening") {
        const p = e.payload?.port;
        if (typeof p === "number" && p > 0 && p !== 22) return p;
      }
    }
    return null;
  })();

  useEffect(() => {
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${proto}//${window.location.host}${wsPath}`);

    ws.onmessage = (ev) => {
      try {
        const parsed: WizardEvent = JSON.parse(ev.data);
        setEvents((prev) => [...prev, parsed]);
        if (parsed.type === "agent_online") setAgentOnline(true);
        if (parsed.type === "agent_offline") setAgentOnline(false);
      } catch {
        /* ignore malformed */
      }
    };

    return () => ws.close();
  }, [wsPath]);

  return (
    <aside className="w-96 h-full overflow-y-auto border-r border-learn-surface bg-learn-surface/40 p-6 flex flex-col gap-4">
      <header className="pb-3 border-b border-learn-surface">
        <h2 className="text-lg font-semibold">Your first Claude Code session</h2>
        <p className="text-xs text-learn-cream/50 mt-1 flex items-center gap-2">
          <span>
            Session <span className="font-mono">{sessionId.slice(0, 8)}</span>
          </span>
          <span
            className={`inline-block w-2 h-2 rounded-full ${
              agentOnline ? "bg-green-400" : "bg-amber-400"
            }`}
            title={agentOnline ? "Guest agent online" : "Waiting for guest agent..."}
          />
        </p>
      </header>

      <ol className="space-y-4">
        {steps.map((s, i) => {
          const done = s.predicate(events);
          return (
            <li
              key={s.id}
              className={`rounded border p-3 transition-colors ${
                done
                  ? "border-green-700/60 bg-green-950/30"
                  : "border-learn-surface"
              }`}
            >
              <div className="flex items-start gap-3">
                <span
                  className={`font-semibold ${
                    done ? "text-green-400" : "text-learn-accent"
                  }`}
                >
                  {done ? "✓" : `${i + 1}.`}
                </span>
                <div>
                  <h3 className="font-medium">{s.title}</h3>
                  <p className="text-sm text-learn-cream/70 mt-1">{s.hint}</p>
                </div>
              </div>
            </li>
          );
        })}
      </ol>

      <div className="mt-auto pt-4 border-t border-learn-surface space-y-3">
        {previewPort && (
          <a
            href={`/sessions/${sessionId}/preview/${previewPort}/`}
            target="_blank"
            rel="noreferrer"
            className="block w-full bg-learn-accent text-learn-bg text-center font-medium py-2 rounded"
          >
            Open preview (port {previewPort}) &rarr;
          </a>
        )}
        <details className="text-xs text-learn-cream/40">
          <summary className="cursor-pointer">event log ({events.length})</summary>
          <pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap break-all">
            {events.map((e) => `${e.ts.slice(11, 19)}  ${e.type}\n`).join("")}
          </pre>
        </details>
        <button
          onClick={onEnd}
          className="w-full bg-learn-surface hover:bg-learn-surface/80 py-2 rounded text-sm text-learn-cream/80"
        >
          End session
        </button>
      </div>
    </aside>
  );
}
