"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import dynamic from "next/dynamic";
import Link from "next/link";
import type { Lesson, LessonStep } from "@/lib/lessons";
import {
  loadProgress,
  markModuleComplete,
  markStepComplete,
  resetModule,
} from "@/lib/progress";

// xterm.js touches `self` on import, so the Terminal component is client-only.
const Terminal = dynamic(() => import("@/components/Terminal"), { ssr: false });

type Session = { id: string; ptyPath: string; wizardPath: string };
type WizardEvent = {
  type: string;
  ts?: string;
  payload?: Record<string, unknown>;
};

type Phase =
  | "booting"          // POST /sessions in flight
  | "wait-agent"       // VM up, waiting for guest agent
  | "wait-claude"      // bootstrap typed, waiting for claude process
  | "ready"            // lesson can begin
  | "sending"          // prompt injected, waiting for claude to respond
  | "step-done";       // success bar shown, awaiting Continuar

// Soft cap: if the shell prompt or claude don't show up after this, we let
// the user proceed manually instead of looping forever on a "loading" state.
const CLAUDE_BOOTSTRAP_TIMEOUT_MS = 15000;

type Copy = {
  skip: string;
  backToTree: string;
  bootingVm: string;
  bootingAgent: string;
  startingClaude: string;
  ready: string;
  sending: string;
  launchError: string;
  retry: string;
  promptLabel: string;
  sendBtn: string;
  sendingBtn: string;
  busyBtn: string;
  alreadyDoneBtn: string;
  continueBtn: string;
  finishBtn: string;
  moduleLabel: string;
  courseDone: string;
  nextModule: string;
  redoModule: string;
  controls: string;
  resetStep: string;
  resetModule: string;
  restartSandbox: string;
  resetConfirmStep: string;
  resetConfirmModule: string;
  restartConfirm: string;
};

const COPY: Record<"es" | "en", Copy> = {
  es: {
    skip: "Saltar",
    backToTree: "Volver al arbol",
    bootingVm: "Arrancando sandbox...",
    bootingAgent: "Esperando al agente del sandbox...",
    startingClaude: "Iniciando Claude Code...",
    ready: "Claude listo. Empieza el paso.",
    sending: "Esperando respuesta de Claude...",
    launchError: "No se ha podido arrancar la sandbox.",
    retry: "Reintentar",
    promptLabel: "Prompt para enviar",
    sendBtn: "Enviar a Claude",
    sendingBtn: "Esperando respuesta...",
    busyBtn: "Claude ocupado, espera",
    alreadyDoneBtn: "Marcar como hecho",
    continueBtn: "Continuar",
    finishBtn: "Terminar leccion",
    moduleLabel: "Modulo",
    courseDone: "Curso completado",
    nextModule: "Siguiente modulo",
    redoModule: "Rehacer modulo",
    controls: "Controles",
    resetStep: "Repetir paso",
    resetModule: "Reiniciar modulo",
    restartSandbox: "Reiniciar sandbox",
    resetConfirmStep: "¿Repetir este paso? Pierdes el avance del paso actual.",
    resetConfirmModule: "¿Reiniciar el modulo desde cero? Pierdes el avance del modulo.",
    restartConfirm: "¿Tirar el sandbox y arrancar uno nuevo? Pierdes lo que tengas en la VM.",
  },
  en: {
    skip: "Skip",
    backToTree: "Back to the tree",
    bootingVm: "Booting sandbox...",
    bootingAgent: "Waiting for sandbox agent...",
    startingClaude: "Starting Claude Code...",
    ready: "Claude ready. Start the step.",
    sending: "Waiting for Claude's response...",
    launchError: "Sandbox failed to launch.",
    retry: "Retry",
    promptLabel: "Prompt to send",
    sendBtn: "Send to Claude",
    sendingBtn: "Waiting for response...",
    busyBtn: "Claude busy, wait",
    alreadyDoneBtn: "Mark as done",
    continueBtn: "Continue",
    finishBtn: "Finish lesson",
    moduleLabel: "Module",
    courseDone: "Course completed",
    nextModule: "Next module",
    redoModule: "Redo module",
    controls: "Controls",
    resetStep: "Redo step",
    resetModule: "Restart module",
    restartSandbox: "Restart sandbox",
    resetConfirmStep: "Redo this step? You'll lose the current step's progress.",
    resetConfirmModule: "Restart the module from scratch? You'll lose the module's progress.",
    restartConfirm: "Throw away the sandbox and start a fresh one? You'll lose anything in the VM.",
  },
};

// How long the PTY must stay quiet (ms) after a prompt is injected before we
// consider claude's response complete. Tuned conservatively so streamed
// answers don't trip it mid-token.
const RESPONSE_IDLE_MS = 2500;

// Skip the first N ms after a send so we don't approve on echo + TUI redraw.
const SEND_ECHO_GRACE_MS = 1500;

// Claude code TUI shows "esc to interrupt" while it's processing a prompt.
// We test only the LATEST chunk (not the rolling buffer) because the TUI
// uses cursor positioning to redraw the bottom bar in place; with a rolling
// text buffer the string would persist after claude is actually done.
const CLAUDE_BUSY_HINT_RE = /esc to interrupt/i;
// How long after the last "esc to interrupt" sighting we still consider
// claude to be processing. Tuned so brief gaps between TUI redraws don't
// look like "done" while the request is still in flight.
const BUSY_HINT_FRESHNESS_MS = 1500;
// Minimum bytes received since send before we consider an auto-complete.
// Below this, the user almost certainly just got an echo + framing redraw,
// not a real response.
const MIN_RESPONSE_BYTES = 200;
// Hard cap on a single send. If we haven't auto-completed by this point,
// surface the success bar anyway so the lesson doesn't hang.
const RESPONSE_HARD_TIMEOUT_MS = 60000;

export default function LessonPlayer({ lesson }: { lesson: Lesson }) {
  const t = COPY[lesson.language];

  const [session, setSession] = useState<Session | null>(null);
  const [launchError, setLaunchError] = useState<string | null>(null);
  const [stepIdx, setStepIdx] = useState(0);
  const [phase, setPhase] = useState<Phase>("booting");
  const [finished, setFinished] = useState(false);
  // Once true, the user can interact with the lesson UI even if claude never
  // signaled ready. They might have dismissed the overlay or claude is slow.
  const [bootstrapTimedOut, setBootstrapTimedOut] = useState(false);
  // True while we see the "esc to interrupt" footer in the TUI buffer, i.e.
  // claude is processing. The Send button is disabled while this is true so
  // a second click can't shove a newline into claude's input.
  const [claudeBusy, setClaudeBusy] = useState(false);
  const [restartTick, setRestartTick] = useState(0);
  const [controlsOpen, setControlsOpen] = useState(false);

  const sendRef = useRef<((data: string) => void) | null>(null);
  const lastPtyAt = useRef<number>(0);
  const sendAtRef = useRef<number>(0);
  const bytesSinceSendRef = useRef<number>(0);
  // Timestamp of the last chunk that contained "esc to interrupt". Drives
  // both the Send-button disable state and the auto-complete gate.
  const lastBusyHintAt = useRef<number>(0);
  const idleTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const hardTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const bootstrapTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const bootstrappedRef = useRef(false);
  // Cumulative PTY bytes since first contact, used as a fallback signal that
  // claude is rendering when the wizard process_started event misses.
  const ptyBytesRef = useRef<number>(0);
  // Whether we've already auto-dismissed the workspace-trust dialog. Some
  // claude-code versions (GitHub #9113 regression) ignore the pre-baked
  // ~/.claude/.claude.json projects entries and show the dialog anyway.
  // If we see the dialog text, we send Enter to accept "1. Yes, I trust",
  // which is the default highlighted choice.
  const dismissedTrustRef = useRef(false);
  const wizardWsRef = useRef<WebSocket | null>(null);
  const decoderRef = useRef<TextDecoder | null>(null);

  const step: LessonStep | undefined = lesson.steps[stepIdx];

  // Resume in-progress: jump to the first step the user hasn't completed yet.
  useEffect(() => {
    const p = loadProgress();
    const m = p.modules[lesson.id];
    if (!m) return;
    const firstIncomplete = lesson.steps.findIndex(
      (s) => !m.completedSteps.includes(s.id),
    );
    if (firstIncomplete > 0) setStepIdx(firstIncomplete);
    else if (firstIncomplete === -1) setFinished(true);
  }, [lesson.id, lesson.steps]);

  // Spawn the sandbox VM. Re-runs when restartTick changes ("Reiniciar sandbox").
  useEffect(() => {
    let cancelled = false;
    async function spawn() {
      try {
        const res = await fetch("/sessions", {
          method: "POST",
          credentials: "same-origin",
        });
        if (!res.ok) throw new Error(`launch ${res.status}: ${await res.text()}`);
        const data = await res.json();
        if (cancelled) return;
        setSession({
          id: data.session_id,
          ptyPath: data.pty_path,
          wizardPath: data.wizard_path,
        });
        setPhase("wait-agent");
      } catch (e: unknown) {
        if (cancelled) return;
        setLaunchError(e instanceof Error ? e.message : String(e));
      }
    }
    spawn();
    return () => {
      cancelled = true;
    };
  }, [restartTick]);

  // Cleanup on unmount + on tab close. The control plane's pool is small
  // (3 VMs), so leaks here strand other users behind a 503.
  useEffect(() => {
    if (!session) return;
    const id = session.id;
    const destroy = () => {
      fetch(`/sessions/${id}`, {
        method: "DELETE",
        credentials: "same-origin",
        keepalive: true,
      }).catch(() => undefined);
    };
    window.addEventListener("beforeunload", destroy);
    window.addEventListener("pagehide", destroy);
    return () => {
      window.removeEventListener("beforeunload", destroy);
      window.removeEventListener("pagehide", destroy);
      destroy();
    };
  }, [session]);

  // Wizard WebSocket: drives auto-bootstrap of claude.
  useEffect(() => {
    if (!session) return;
    // Master safety net: regardless of agent_online / prompt detection /
    // claude detection, after this delay let the user use the terminal and
    // the lesson UI manually. Cancelled when claude is confirmed up.
    if (bootstrapTimeoutRef.current) clearTimeout(bootstrapTimeoutRef.current);
    bootstrapTimeoutRef.current = setTimeout(() => {
      setBootstrapTimedOut(true);
      setPhase((p) =>
        p === "wait-agent" || p === "wait-claude" ? "ready" : p,
      );
    }, CLAUDE_BOOTSTRAP_TIMEOUT_MS);

    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(
      `${proto}//${window.location.host}${session.wizardPath}`,
    );
    wizardWsRef.current = ws;

    ws.onmessage = (ev) => {
      let parsed: WizardEvent | null = null;
      try {
        parsed = JSON.parse(ev.data);
      } catch {
        return;
      }
      if (!parsed) return;

      if (parsed.type === "agent_online") {
        // Guest agent is up. Claude is being launched by learn-shell as we
        // speak. Move the visual phase forward so the banner reads
        // "Iniciando Claude..." instead of "Esperando al agente..." while
        // we wait for either process_started or PTY traffic.
        setPhase((p) => (p === "wait-agent" ? "wait-claude" : p));
      }

      if (
        parsed.type === "process_started" &&
        typeof parsed.payload?.name === "string" &&
        (parsed.payload.name === "claude" ||
          parsed.payload.name === "node" ||
          parsed.payload.name === "claude-code")
      ) {
        // Claude is up. The wrapper script (learn-shell) launches claude
        // which is a node process; the guest-agent may report it as either
        // depending on what /proc/<pid>/comm reflects. Either is enough.
        if (bootstrapTimeoutRef.current) clearTimeout(bootstrapTimeoutRef.current);
        // Give claude a beat to finish its splash + show the prompt.
        setTimeout(() => {
          setBootstrapTimedOut(false);
          setPhase((p) =>
            p === "wait-claude" || p === "wait-agent" ? "ready" : p,
          );
        }, 1500);
      }
    };

    return () => {
      wizardWsRef.current = null;
      ws.close();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session]);

  // The VM's autologin shell IS Claude Code (see vm-image/learn-shell.sh).
  // We don't inject a bootstrap command anymore: Claude is launched as the
  // user's login shell, with --dangerously-skip-permissions baked in. The
  // only thing we wait on is the `process_started: claude` wizard event,
  // which fires within ~1s of the autologin handing off to learn-shell.

  function retryBootstrap() {
    // Hard reload of the sandbox: tear down the VM and spawn a new one.
    // Wrapper relaunches Claude on its own.
    if (bootstrapTimeoutRef.current) clearTimeout(bootstrapTimeoutRef.current);
    setBootstrapTimedOut(false);
    setSession((s) => {
      if (s) {
        fetch(`/sessions/${s.id}`, {
          method: "DELETE",
          credentials: "same-origin",
          keepalive: true,
        }).catch(() => undefined);
      }
      return null;
    });
    setPhase("booting");
    setRestartTick((n) => n + 1);
  }

  function dismissBootstrap() {
    if (bootstrapTimeoutRef.current) clearTimeout(bootstrapTimeoutRef.current);
    setBootstrapTimedOut(true);
    setPhase((p) => (p === "wait-agent" || p === "wait-claude" ? "ready" : p));
  }

  const onTerminalReady = useCallback((send: (s: string) => void) => {
    // We're not running any bootstrap command from the browser anymore: the
    // VM autologins straight into Claude Code (via /usr/local/bin/learn-shell).
    // We only need a handle to the PTY for prompt injection inside lessons.
    sendRef.current = send;
  }, []);

  const onTerminalData = useCallback(
    (chunk: Uint8Array | string) => {
      const now = Date.now();
      lastPtyAt.current = now;
      const chunkLen =
        typeof chunk === "string" ? chunk.length : chunk.byteLength;
      // First sign of life from the PTY = autologin happened, claude is
      // about to (or already did) start. Use PTY traffic as a fallback
      // signal to advance the phase: process_started events from the guest
      // agent depend on the binary name (claude vs node) and can be missed.
      if (!bootstrappedRef.current) {
        bootstrappedRef.current = true;
        ptyBytesRef.current = 0;
      }
      ptyBytesRef.current += chunkLen;
      // Once enough TUI output has flowed (claude's splash + first frame is
      // ~3-8KB), assume claude is rendering and unblock the lesson UI.
      if (ptyBytesRef.current > 4000) {
        setPhase((p) =>
          p === "wait-agent" || p === "wait-claude" ? "ready" : p,
        );
        if (bootstrapTimeoutRef.current) {
          clearTimeout(bootstrapTimeoutRef.current);
          bootstrapTimeoutRef.current = null;
        }
        setBootstrapTimedOut(false);
      }

      // Decode + strip ANSI for busy-hint detection.
      if (!decoderRef.current) decoderRef.current = new TextDecoder();
      const text =
        typeof chunk === "string"
          ? chunk
          : decoderRef.current.decode(chunk, { stream: true });
      // eslint-disable-next-line no-control-regex
      const strippedChunk = text.replace(/\x1b\[[0-9;?]*[a-zA-Z]/g, "");

      // Workspace-trust dialog auto-dismiss. The pre-baked .claude.json
      // projects map should silence this, but the regression in #9113 means
      // it sometimes shows up anyway. The dialog defaults to highlighting
      // "Yes, I trust this folder", so a single Enter accepts.
      if (
        !dismissedTrustRef.current &&
        /quick safety check|trust this folder|Yes, I trust/i.test(strippedChunk) &&
        sendRef.current
      ) {
        dismissedTrustRef.current = true;
        const send = sendRef.current;
        // Tiny delay so claude has finished drawing the prompt before we
        // submit (otherwise the input race can confuse the TUI).
        setTimeout(() => send("\r"), 250);
      }
      // Same idea for the --dangerously-skip-permissions confirmation that
      // can still show up if skipDangerousModePermissionPrompt didn't take.
      if (
        !dismissedTrustRef.current &&
        /bypass permissions|are you sure.*permissions/i.test(strippedChunk) &&
        sendRef.current
      ) {
        const send = sendRef.current;
        setTimeout(() => send("\r"), 250);
      }

      // Busy-state tracking. We test ONLY the latest chunk: claude redraws
      // its "esc to interrupt" footer constantly while busy, so a fresh
      // sighting means it's still working. We refresh a timestamp instead
      // of testing a rolling buffer (which would retain the string forever
      // because the TUI uses cursor-positioning to overwrite in place).
      if (CLAUDE_BUSY_HINT_RE.test(strippedChunk)) {
        lastBusyHintAt.current = now;
      }
      const busyNow = now - lastBusyHintAt.current < BUSY_HINT_FRESHNESS_MS;
      setClaudeBusy(busyNow);

      // Response detection. Only relevant while waiting on a sent prompt.
      if (phase !== "sending") return;
      bytesSinceSendRef.current += chunkLen;

      // Skip the early echo + TUI redraw window.
      if (now - sendAtRef.current < SEND_ECHO_GRACE_MS) return;

      // Arm/reset the idle timer. Auto-complete fires when:
      //   1. enough bytes came back (so it's a real response, not just echo),
      //   2. claude is no longer busy (last busy hint was long enough ago),
      //   3. the PTY has been quiet for RESPONSE_IDLE_MS.
      if (idleTimer.current) clearTimeout(idleTimer.current);
      idleTimer.current = setTimeout(() => {
        const t = Date.now();
        const enough = bytesSinceSendRef.current >= MIN_RESPONSE_BYTES;
        const notBusy = t - lastBusyHintAt.current >= BUSY_HINT_FRESHNESS_MS;
        const quiet = t - lastPtyAt.current >= RESPONSE_IDLE_MS - 50;
        if (enough && notBusy && quiet) {
          completeStep();
        }
        // Otherwise stay in "sending"; the next chunk re-arms us. The hard
        // timeout below is the eventual safety net.
      }, RESPONSE_IDLE_MS);
    },
    [phase],
  );

  // Inject the current step's prompt into claude. We:
  //  1. Wrap the prompt in bracketed-paste markers so claude treats embedded
  //     newlines as literal text (its TUI uses bracketed paste mode by
  //     default; without this the inner \r becomes a literal newline in the
  //     input box and nothing submits).
  //  2. Send Enter as a SEPARATE chunk after a short delay so it lands
  //     outside the paste, where claude interprets it as "submit".
  function handleSend() {
    if (!step?.prompt || !sendRef.current) return;
    if (phase !== "ready" || claudeBusy) return;
    const send = sendRef.current;
    const text = step.prompt.trim().replace(/\s*\n\s*/g, " ");

    setPhase("sending");
    const now = Date.now();
    lastPtyAt.current = now;
    sendAtRef.current = now;
    bytesSinceSendRef.current = 0;
    lastBusyHintAt.current = 0;

    send(`\x1b[200~${text}\x1b[201~`);
    setTimeout(() => send("\r"), 80);

    // Hard cap. If detection misses (claude responded outside our heuristics
    // or the PTY went silent without a busy footer), surface the success bar
    // anyway after this so the lesson never wedges.
    if (hardTimeoutRef.current) clearTimeout(hardTimeoutRef.current);
    hardTimeoutRef.current = setTimeout(() => {
      if (idleTimer.current) clearTimeout(idleTimer.current);
      completeStep();
    }, RESPONSE_HARD_TIMEOUT_MS);
  }

  // Steps without a prompt (e.g., reflective ones) just need a manual mark.
  function handleManualDone() {
    if (phase === "step-done") return;
    completeStep();
  }

  // Lesson controls. All three reset state in different scopes.
  function handleResetStep() {
    if (!step) return;
    if (!window.confirm(t.resetConfirmStep)) return;
    if (idleTimer.current) clearTimeout(idleTimer.current);
    setPhase("ready");
    setControlsOpen(false);
  }

  function handleResetModule() {
    if (!window.confirm(t.resetConfirmModule)) return;
    resetModule(lesson.id);
    if (idleTimer.current) clearTimeout(idleTimer.current);
    setStepIdx(0);
    setPhase("ready");
    setFinished(false);
    setControlsOpen(false);
  }

  function handleRestartSandbox() {
    if (!window.confirm(t.restartConfirm)) return;
    setControlsOpen(false);
    // Tear down the current VM and force a fresh spawn via the spawn effect.
    if (session) {
      fetch(`/sessions/${session.id}`, {
        method: "DELETE",
        credentials: "same-origin",
        keepalive: true,
      }).catch(() => undefined);
    }
    if (idleTimer.current) clearTimeout(idleTimer.current);
    if (bootstrapTimeoutRef.current) clearTimeout(bootstrapTimeoutRef.current);
    bootstrappedRef.current = false;
    dismissedTrustRef.current = false;
    ptyBytesRef.current = 0;
    setBootstrapTimedOut(false);
    setClaudeBusy(false);
    setSession(null);
    setLaunchError(null);
    setPhase("booting");
    setRestartTick((n) => n + 1);
  }

  function completeStep() {
    if (!step) return;
    if (idleTimer.current) {
      clearTimeout(idleTimer.current);
      idleTimer.current = null;
    }
    if (hardTimeoutRef.current) {
      clearTimeout(hardTimeoutRef.current);
      hardTimeoutRef.current = null;
    }
    markStepComplete(lesson.id, step.id, step.xp ?? 10);
    setPhase("step-done");
  }

  function handleContinue() {
    if (stepIdx + 1 >= lesson.steps.length) {
      markModuleComplete(lesson.id);
      setFinished(true);
      return;
    }
    setStepIdx(stepIdx + 1);
    setPhase("ready");
  }

  if (finished) {
    return (
      <FinishedScreen
        lesson={lesson}
        t={t}
        onRedo={handleResetModule}
      />
    );
  }

  return (
    <div className="h-screen flex flex-col bg-learn-burgundy text-learn-warm overflow-hidden">
      <Topbar
        lesson={lesson}
        stepIdx={stepIdx}
        t={t}
        controlsOpen={controlsOpen}
        onToggleControls={() => setControlsOpen((v) => !v)}
        onResetStep={handleResetStep}
        onResetModule={handleResetModule}
        onRestartSandbox={handleRestartSandbox}
      />

      <main className="flex-1 grid grid-cols-1 md:grid-cols-[420px_1fr] min-h-0">
        <LessonPanel
          step={step}
          phase={phase}
          claudeBusy={claudeBusy}
          onSend={handleSend}
          onMarkDone={handleManualDone}
          t={t}
        />
        <div className="m-4 mr-6 ml-0 md:ml-0 rounded-2xl border border-learn-warm/10 bg-[#1f1510] overflow-hidden flex flex-col shadow-[0_20px_60px_rgba(0,0,0,0.35)]">
          <div className="flex items-center justify-between px-5 py-3 border-b border-learn-warm/10 bg-black/15">
            <div className="flex items-center gap-2.5 text-sm text-learn-warm/85">
              <PhaseDot phase={phase} />
              Claude Code
            </div>
            <div className="font-mono text-[11px] opacity-50">
              {lesson.preconditions?.working_directory?.replace("/home/learner", "~") ??
                "~/empresa-prueba"}
            </div>
          </div>
          <div className="flex-1 min-h-0 relative">
            {launchError ? (
              <LaunchError msg={launchError} t={t} />
            ) : session ? (
              <>
                <Terminal
                  wsPath={session.ptyPath}
                  onReady={onTerminalReady}
                  onData={onTerminalData}
                />
                {(phase === "wait-agent" || phase === "wait-claude") && (
                  <BootingBanner
                    phase={phase}
                    onSkip={dismissBootstrap}
                    t={t}
                  />
                )}
                {bootstrapTimedOut && phase === "ready" && (
                  <RetryClaudeBanner
                    onRetry={retryBootstrap}
                    onDismiss={() => setBootstrapTimedOut(false)}
                    t={t}
                  />
                )}
              </>
            ) : (
              <BootingBanner phase="booting" onSkip={() => undefined} t={t} />
            )}
          </div>
        </div>
      </main>

      {phase === "step-done" && step && (
        <SuccessBar
          step={step}
          isLast={stepIdx + 1 >= lesson.steps.length}
          onContinue={handleContinue}
          t={t}
        />
      )}
    </div>
  );
}

function PhaseDot({ phase }: { phase: Phase }) {
  const cls =
    phase === "ready" || phase === "step-done"
      ? "bg-learn-successMid shadow-[0_0_8px_rgba(111,207,122,0.6)]"
      : phase === "sending"
        ? "bg-learn-ember animate-pulse"
        : "bg-amber-400 animate-pulse";
  return <span className={`w-2 h-2 rounded-full ${cls}`} />;
}

function Topbar({
  lesson,
  stepIdx,
  t,
  controlsOpen,
  onToggleControls,
  onResetStep,
  onResetModule,
  onRestartSandbox,
}: {
  lesson: Lesson;
  stepIdx: number;
  t: Copy;
  controlsOpen: boolean;
  onToggleControls: () => void;
  onResetStep: () => void;
  onResetModule: () => void;
  onRestartSandbox: () => void;
}) {
  return (
    <div className="h-16 px-10 flex items-center justify-between gap-6 border-b border-learn-warm/10 relative">
      <div className="flex items-center gap-3.5 min-w-0">
        <Link
          href="/lessons"
          className="text-[11px] tracking-[0.22em] uppercase text-learn-warm/90 hover:text-learn-warmHi"
        >
          learn.example.com
        </Link>
        <span className="w-px h-3.5 bg-learn-warm/25" />
        <span className="text-[13px] opacity-70 truncate">
          {t.moduleLabel} {lesson.module_number}, {lesson.title}
        </span>
      </div>

      <div className="flex-1 flex justify-center">
        <ProgressBar steps={lesson.steps} stepIdx={stepIdx} />
      </div>

      <div className="flex items-center gap-3 relative">
        <button
          onClick={onToggleControls}
          className="text-[12px] opacity-70 hover:opacity-100 px-3 py-1.5 rounded-full border border-learn-warm/20 hover:bg-learn-warm/10"
          aria-haspopup="menu"
          aria-expanded={controlsOpen}
        >
          {t.controls} ▾
        </button>
        <Link href="/lessons" className="text-[13px] opacity-60 hover:opacity-100">
          {t.skip}
        </Link>

        {controlsOpen && (
          <div className="absolute top-full right-0 mt-2 w-60 bg-[#1f1510] border border-learn-warm/15 rounded-lg shadow-[0_18px_44px_rgba(0,0,0,0.4)] py-2 z-30">
            <ControlItem onClick={onResetStep} label={t.resetStep} />
            <ControlItem onClick={onResetModule} label={t.resetModule} />
            <ControlItem onClick={onRestartSandbox} label={t.restartSandbox} />
          </div>
        )}
      </div>
    </div>
  );
}

function ControlItem({
  onClick,
  label,
}: {
  onClick: () => void;
  label: string;
}) {
  return (
    <button
      onClick={onClick}
      className="w-full text-left px-4 py-2 text-[13px] text-learn-warm hover:bg-learn-warm/10"
    >
      {label}
    </button>
  );
}

function ProgressBar({
  steps,
  stepIdx,
}: {
  steps: { id: string }[];
  stepIdx: number;
}) {
  return (
    <div className="flex gap-2 w-[280px]">
      {steps.map((s, i) => {
        const isDone = i < stepIdx;
        const isCurrent = i === stepIdx;
        return (
          <div
            key={s.id}
            className={`flex-1 h-1.5 rounded-full overflow-hidden relative ${
              isDone ? "bg-learn-successMid" : "bg-learn-warm/15"
            }`}
          >
            {isCurrent && (
              <div className="absolute inset-y-0 left-0 w-3/5 bg-learn-warm rounded-full" />
            )}
          </div>
        );
      })}
    </div>
  );
}

function LessonPanel({
  step,
  phase,
  claudeBusy,
  onSend,
  onMarkDone,
  t,
}: {
  step: LessonStep | undefined;
  phase: Phase;
  claudeBusy: boolean;
  onSend: () => void;
  onMarkDone: () => void;
  t: Copy;
}) {
  if (!step) return null;
  const hasPrompt = !!step.prompt?.trim();
  return (
    <div className="px-9 py-8 overflow-y-auto relative flex flex-col">
      <h2 className="text-[26px] font-normal leading-tight text-learn-warmHi tracking-tight mb-5">
        {step.title}
      </h2>
      {step.body && (
        <p className="text-[15px] leading-relaxed opacity-80 mb-6 whitespace-pre-line">
          {step.body}
        </p>
      )}

      {hasPrompt && step.prompt && (
        <PromptCard
          prompt={step.prompt}
          phase={phase}
          claudeBusy={claudeBusy}
          onSend={onSend}
          t={t}
        />
      )}

      {!hasPrompt && (
        <button
          onClick={onMarkDone}
          disabled={phase !== "ready"}
          className="self-start px-5 py-2.5 rounded-full bg-learn-warmHi text-learn-burgundy-mid font-medium text-[14px] disabled:opacity-50"
        >
          {t.alreadyDoneBtn} →
        </button>
      )}

      {step.hint && (
        <p className="text-[13px] opacity-55 leading-snug pl-3.5 border-l-2 border-learn-warm/20 mt-5 whitespace-pre-line">
          {step.hint}
        </p>
      )}
    </div>
  );
}

function PromptCard({
  prompt,
  phase,
  claudeBusy,
  onSend,
  t,
}: {
  prompt: string;
  phase: Phase;
  claudeBusy: boolean;
  onSend: () => void;
  t: Copy;
}) {
  const trimmed = prompt.trim();
  const sending = phase === "sending";
  // Disable when not ready, when we're already waiting on a response, or
  // when claude is mid-processing of an earlier request.
  const disabled = phase !== "ready" || claudeBusy;
  const label = sending
    ? t.sendingBtn
    : claudeBusy && phase === "ready"
      ? t.busyBtn
      : t.sendBtn;
  const showSpinner = sending || (claudeBusy && phase === "ready");

  return (
    <div className="bg-black/40 border border-learn-warm/15 rounded-xl p-4 mb-3">
      <div className="text-[11px] tracking-wider uppercase opacity-50 mb-2 flex items-center justify-between">
        <span>{t.promptLabel}</span>
        <span className={showSpinner ? "text-learn-ember" : "opacity-0"}>
          {showSpinner ? "Claude pensando..." : ""}
        </span>
      </div>
      <pre className="font-mono text-[13px] leading-snug text-learn-warmHi whitespace-pre-wrap break-words mb-4">
        {trimmed}
      </pre>
      <button
        onClick={onSend}
        disabled={disabled}
        className={`w-full px-5 py-3 rounded-full font-medium text-[14px] inline-flex items-center justify-center gap-2 transition-colors ${
          showSpinner
            ? "bg-learn-ember/20 border border-learn-ember/40 text-learn-ember"
            : disabled
              ? "bg-learn-warm/10 border border-learn-warm/20 text-learn-warm/50"
              : "bg-learn-warmHi text-learn-burgundy-mid hover:bg-white"
        }`}
      >
        {showSpinner && (
          <span className="w-3 h-3 rounded-full border-2 border-learn-ember border-t-transparent animate-spin" />
        )}
        {label}
        {!showSpinner && !disabled && <span>→</span>}
      </button>
    </div>
  );
}

// Slim, dismissible status banner over the top of the terminal. The terminal
// behind it stays visible and interactive: if the bootstrap fails and the
// user wants to type `claude` themselves, they can.
function BootingBanner({
  phase,
  onSkip,
  t,
}: {
  phase: Phase | "booting";
  onSkip: () => void;
  t: Copy;
}) {
  const msg =
    phase === "booting"
      ? t.bootingVm
      : phase === "wait-agent"
        ? t.bootingAgent
        : t.startingClaude;
  return (
    <div className="absolute top-3 left-3 right-3 z-10 flex items-center justify-between gap-3 px-4 py-2 bg-learn-warm/10 border border-learn-warm/20 backdrop-blur rounded-lg text-learn-warm text-[13px]">
      <div className="flex items-center gap-2.5">
        <span className="w-3 h-3 rounded-full border-2 border-learn-ember border-t-transparent animate-spin" />
        <span>{msg}</span>
      </div>
      <button
        onClick={onSkip}
        className="text-[12px] opacity-70 hover:opacity-100 underline-offset-2 hover:underline"
      >
        Saltar
      </button>
    </div>
  );
}

function RetryClaudeBanner({
  onRetry,
  onDismiss,
  t,
}: {
  onRetry: () => void;
  onDismiss: () => void;
  t: Copy;
}) {
  return (
    <div className="absolute top-3 left-3 right-3 z-10 flex items-center justify-between gap-3 px-4 py-2 bg-amber-500/10 border border-amber-500/30 backdrop-blur rounded-lg text-amber-100 text-[13px]">
      <div className="flex items-center gap-2.5">
        <span>Claude no respondio. Mira el terminal: puede haber un error, o reintenta.</span>
      </div>
      <div className="flex items-center gap-3">
        <button
          onClick={onRetry}
          className="text-[12px] px-3 py-1 rounded-full bg-amber-300/20 border border-amber-300/40 hover:bg-amber-300/30"
        >
          {t.retry}
        </button>
        <button
          onClick={onDismiss}
          className="text-[12px] opacity-70 hover:opacity-100 underline-offset-2 hover:underline"
        >
          ✕
        </button>
      </div>
    </div>
  );
}

function SuccessBar({
  step,
  isLast,
  onContinue,
  t,
}: {
  step: LessonStep;
  isLast: boolean;
  onContinue: () => void;
  t: Copy;
}) {
  return (
    <div className="bg-learn-success px-6 py-2 flex items-center justify-between shadow-[0_-6px_18px_rgba(0,0,0,0.18)] animate-slideUp">
      <div className="flex items-center gap-3 min-w-0">
        <div className="w-6 h-6 rounded-full bg-white text-learn-successDark flex items-center justify-center text-sm font-bold flex-shrink-0">
          ✓
        </div>
        <div className="text-white min-w-0">
          <span className="text-[13px] font-medium tracking-tight">
            {step.success_title ?? "✓"}
          </span>
          {step.success_sub && (
            <span className="text-[12px] opacity-85 ml-2 hidden md:inline">
              · {step.success_sub}
            </span>
          )}
        </div>
      </div>
      <button
        onClick={onContinue}
        className="bg-white text-learn-successDark font-semibold px-5 py-1.5 rounded-lg text-[13px] hover:scale-[1.03] transition-transform flex-shrink-0"
      >
        {isLast ? t.finishBtn : t.continueBtn}
      </button>
    </div>
  );
}

function FinishedScreen({
  lesson,
  t,
  onRedo,
}: {
  lesson: Lesson;
  t: Copy;
  onRedo: () => void;
}) {
  const next = lesson.on_complete?.next_module ?? null;
  const isCourseEnd = lesson.on_complete?.course_complete;
  return (
    <div className="min-h-screen bg-learn-burgundy text-learn-warm flex items-center justify-center p-8">
      <div className="max-w-xl text-center space-y-6">
        <div className="inline-flex items-center gap-2.5 px-4 py-1.5 bg-learn-successMid/15 border border-learn-successMid/35 rounded-full text-[11px] tracking-wider uppercase font-medium text-learn-successLight">
          {isCourseEnd ? t.courseDone : `${t.moduleLabel} ${lesson.module_number} ✓`}
        </div>
        <h1 className="text-4xl md:text-5xl font-normal text-learn-warmHi tracking-tight leading-tight">
          {lesson.title}
        </h1>
        {lesson.on_complete?.takeaways && (
          <ul className="space-y-2.5 text-left max-w-md mx-auto">
            {lesson.on_complete.takeaways.map((tk, i) => (
              <li key={i} className="flex items-start gap-3 opacity-90 text-[15px]">
                <span className="mt-2 w-1.5 h-1.5 rounded-full bg-learn-ember shadow-[0_0_8px_rgba(255,197,145,0.6)] shrink-0" />
                <span>{tk}</span>
              </li>
            ))}
          </ul>
        )}
        <div className="flex flex-col items-center gap-3 pt-4">
          {next ? (
            <Link
              href={`/lessons/${next}?lang=${lesson.language}`}
              className="bg-learn-warmHi text-learn-burgundy-mid font-medium px-8 py-3 rounded-full inline-flex items-center gap-3 shadow-[0_14px_42px_rgba(0,0,0,0.24)] hover:-translate-y-0.5 transition-transform"
            >
              {t.nextModule} →
            </Link>
          ) : null}
          <button
            onClick={onRedo}
            className="text-sm text-learn-warm/80 hover:text-learn-warmHi underline-offset-2 hover:underline"
          >
            {t.redoModule}
          </button>
          <Link href="/lessons" className="text-sm opacity-60 hover:opacity-100">
            {t.backToTree}
          </Link>
        </div>
      </div>
    </div>
  );
}

function LaunchError({ msg, t }: { msg: string; t: Copy }) {
  return (
    <div className="h-full w-full flex items-center justify-center p-6">
      <div className="max-w-md text-center space-y-3">
        <p className="text-red-300 text-sm">{t.launchError}</p>
        <pre className="text-[11px] text-learn-warm/40 whitespace-pre-wrap break-words">
          {msg}
        </pre>
        <button
          onClick={() => window.location.reload()}
          className="px-5 py-2 rounded-full bg-learn-warm/10 border border-learn-warm/20 text-learn-warmHi text-sm"
        >
          {t.retry}
        </button>
      </div>
    </div>
  );
}
