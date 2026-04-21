"use client";

import { XtermView } from "../lib/xterm";

type TerminalProps = {
  lessonId: string;
  ptyWsUrl?: string;
};

export default function Terminal({ lessonId, ptyWsUrl }: TerminalProps) {
  return (
    <div className="flex h-full min-h-[320px] w-full flex-col">
      <div className="flex items-center justify-between border-b border-surface-divider pb-2 text-xs uppercase tracking-widest text-ink-tertiary">
        <span>Terminal</span>
        <span>{lessonId}</span>
      </div>
      <div className="mt-2 flex-1 overflow-hidden rounded-chip bg-chromatic-deep">
        <XtermView ptyWsUrl={ptyWsUrl} />
      </div>
    </div>
  );
}
