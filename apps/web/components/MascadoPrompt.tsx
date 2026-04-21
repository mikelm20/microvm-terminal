"use client";

import { useCallback, useState } from "react";
import { track } from "../lib/track";

type MascadoPromptProps = {
  lessonId: string;
  stepId: string;
  prompt: string;
  hint?: string;
};

export default function MascadoPrompt({
  lessonId,
  stepId,
  prompt,
  hint,
}: MascadoPromptProps) {
  const [copied, setCopied] = useState(false);

  const onCopy = useCallback(async () => {
    try {
      if (typeof navigator !== "undefined" && navigator.clipboard) {
        await navigator.clipboard.writeText(prompt);
      } else {
        throw new Error("clipboard_unavailable");
      }
      setCopied(true);
      void track("step_mascado_copied", { lesson_id: lessonId, step_id: stepId });
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      setCopied(false);
    }
  }, [lessonId, prompt, stepId]);

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <span className="text-xs uppercase tracking-widest text-ink-tertiary">
          Prompt mascado
        </span>
        <button
          type="button"
          onClick={onCopy}
          className="rounded-pill border border-surface-divider px-3 py-1 text-xs text-ink-secondary transition-colors hover:bg-chromatic-burgundyTop hover:text-ink-primary"
        >
          {copied ? "Copiado" : "Copiar"}
        </button>
      </div>
      <pre className="max-h-56 overflow-auto whitespace-pre-wrap break-words rounded-chip bg-chromatic-deep p-3 font-mono text-[13px] leading-relaxed text-ink-primary">
{prompt}
      </pre>
      {hint ? (
        <p className="text-xs leading-relaxed text-ink-tertiary">{hint}</p>
      ) : null}
    </div>
  );
}
