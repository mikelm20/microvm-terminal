import type { WsEvent as WsEventT } from "@learn/shared-api/events";

/**
 * Local event simulator for the Convo branch in isolation. This is a
 * development-only utility that feeds a Conversation screen with a
 * scripted stream of WsEvents when the live control plane is not
 * reachable. Triggered by a dev toggle in the composer overflow.
 */

export interface SimulatedScript {
  /** Events in order, paired with the delay in ms before they fire. */
  steps: Array<{ delay_ms: number; event: WsEventT }>;
}

export function hardcodedM2Script(opts: { sessionId: string; lessonId: string }): SimulatedScript {
  const now = () => new Date().toISOString();
  const turnId = "turn-1";
  return {
    steps: [
      {
        delay_ms: 250,
        event: {
          ts: now(),
          type: "claude_busy",
          busy: true,
        },
      },
      {
        delay_ms: 350,
        event: {
          ts: now(),
          type: "claude_tool_call",
          tool: "Read",
          args: { file_path: "/home/learn/empresa-prueba/01-personas/marta.md" },
          path: "/home/learn/empresa-prueba/01-personas/marta.md",
          call_id: "call-1",
        },
      },
      {
        delay_ms: 800,
        event: {
          ts: now(),
          type: "claude_tool_result",
          call_id: "call-1",
          ok: true,
          summary: "Marta Gimenez, Directora de Ventas, 12 anios en la empresa.",
        },
      },
      {
        delay_ms: 300,
        event: {
          ts: now(),
          type: "claude_tool_call",
          tool: "Glob",
          args: { pattern: "01-personas/*.md" },
          call_id: "call-2",
        },
      },
      {
        delay_ms: 450,
        event: {
          ts: now(),
          type: "claude_tool_result",
          call_id: "call-2",
          ok: true,
          summary: "6 fichas encontradas",
        },
      },
      {
        delay_ms: 300,
        event: {
          ts: now(),
          type: "claude_token_streamed",
          turn_id: turnId,
          delta: "Marta es la directora de ventas.",
          total_chars: 30,
        },
      },
      {
        delay_ms: 120,
        event: {
          ts: now(),
          type: "claude_token_streamed",
          turn_id: turnId,
          delta: " Lleva 12 anios,",
          total_chars: 46,
        },
      },
      {
        delay_ms: 120,
        event: {
          ts: now(),
          type: "claude_token_streamed",
          turn_id: turnId,
          delta: " y firma los presupuestos por encima de 50k.",
          total_chars: 89,
        },
      },
      {
        delay_ms: 120,
        event: {
          ts: now(),
          type: "claude_message",
          role: "assistant",
          text: "Marta es la directora de ventas. Lleva 12 anios, y firma los presupuestos por encima de 50k.",
          turn_id: turnId,
        },
      },
      {
        delay_ms: 200,
        event: {
          ts: now(),
          type: "claude_busy",
          busy: false,
        },
      },
      {
        delay_ms: 200,
        event: {
          ts: now(),
          type: "step_satisfied",
          step_id: "s1",
          lesson_id: opts.lessonId,
          evidence: {
            type: "claude_tool_call",
            tool: "Read",
            call_id: "call-1",
          },
        },
      },
    ],
  };
}

export function runScript(
  script: SimulatedScript,
  onEvent: (ev: WsEventT) => void,
): () => void {
  const timers: Array<ReturnType<typeof setTimeout>> = [];
  let cumulative = 0;
  for (const step of script.steps) {
    cumulative += step.delay_ms;
    const t = setTimeout(() => onEvent(step.event), cumulative);
    timers.push(t);
  }
  return () => {
    for (const t of timers) clearTimeout(t);
  };
}
