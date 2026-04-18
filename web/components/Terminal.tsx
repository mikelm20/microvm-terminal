"use client";

import { useEffect, useRef } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

export type TerminalSendFn = (data: string) => void;
export type TerminalDataFn = (chunk: Uint8Array | string) => void;

export default function Terminal({
  wsPath,
  onReady,
  onData,
}: {
  wsPath: string;
  // Called once the WS is open, hands back a function to write into the PTY.
  // Used by the lesson player to inject prompts.
  onReady?: (send: TerminalSendFn) => void;
  // Called on every chunk of output from the PTY. Used by the lesson player
  // to detect when claude has finished responding (debounced idle).
  onData?: TerminalDataFn;
}) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  // Stable refs so the effect doesn't tear down when callbacks change identity.
  const onReadyRef = useRef(onReady);
  const onDataRef = useRef(onData);

  useEffect(() => {
    onReadyRef.current = onReady;
  }, [onReady]);

  useEffect(() => {
    onDataRef.current = onData;
  }, [onData]);

  useEffect(() => {
    if (!hostRef.current) return;

    const term = new XTerm({
      cursorBlink: true,
      convertEol: false,
      fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, monospace",
      fontSize: 14,
      theme: {
        background: "#0d0604",
        foreground: "#f2e6d5",
        cursor: "#d97455",
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(hostRef.current);

    const requestFit = () => {
      try {
        fit.fit();
      } catch {
        /* terminal not sized yet */
      }
    };
    requestFit();

    const resizeObserver = new ResizeObserver(requestFit);
    resizeObserver.observe(hostRef.current);

    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${proto}//${window.location.host}${wsPath}`);
    ws.binaryType = "arraybuffer";

    ws.onopen = () => {
      term.writeln("\x1b[2m-- conectado al sandbox --\x1b[0m");
      onReadyRef.current?.((data: string) => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(data);
        }
      });
    };
    ws.onmessage = (ev) => {
      if (ev.data instanceof ArrayBuffer) {
        const buf = new Uint8Array(ev.data);
        term.write(buf);
        onDataRef.current?.(buf);
      } else if (typeof ev.data === "string") {
        term.write(ev.data);
        onDataRef.current?.(ev.data);
      }
    };
    ws.onerror = () => {
      term.writeln("\r\n\x1b[31m-- error de websocket --\x1b[0m");
    };
    ws.onclose = () => {
      term.writeln("\r\n\x1b[33m-- sandbox desconectado --\x1b[0m");
    };

    const dataHandler = term.onData((d) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(d);
      }
    });

    return () => {
      dataHandler.dispose();
      resizeObserver.disconnect();
      ws.close();
      term.dispose();
    };
  }, [wsPath]);

  return <div ref={hostRef} className="h-full w-full" />;
}
