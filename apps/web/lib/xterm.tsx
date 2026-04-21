"use client";

import { useEffect, useRef } from "react";
import { Terminal as Xterm } from "xterm";
import { FitAddon } from "@xterm/addon-fit";
import "xterm/css/xterm.css";
import { connectPty, type PtyHandle } from "./ws";

type XtermViewProps = {
  ptyWsUrl?: string;
};

const LOCAL_PROMPT = "learn$ ";

export function XtermView({ ptyWsUrl }: XtermViewProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const term = new Xterm({
      convertEol: true,
      cursorBlink: true,
      fontFamily: "JetBrains Mono, SF Mono, Menlo, monospace",
      fontSize: 13,
      theme: {
        background: "#4a1a06",
        foreground: "#fff6e6",
        cursor: "#ff9b5a",
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(containerRef.current);
    fit.fit();

    const onResize = (): void => {
      try {
        fit.fit();
      } catch {
        // ignore
      }
    };
    window.addEventListener("resize", onResize);

    let pty: PtyHandle | null = null;
    let localLine = "";

    if (ptyWsUrl) {
      term.writeln("\x1b[2mConectando a la maquina...\x1b[0m");
      pty = connectPty(ptyWsUrl, {
        onOpen: () => term.writeln("\x1b[32mConectado\x1b[0m"),
        onClose: () => term.writeln("\x1b[33mDesconectado\x1b[0m"),
        onError: () => term.writeln("\x1b[31mError de conexion\x1b[0m"),
        onData: (data) => {
          if (typeof data === "string") {
            term.write(data);
          } else {
            term.write(new Uint8Array(data));
          }
        },
      });
      term.onData((data) => {
        pty?.send(data);
      });
    } else {
      // Local echo loop so the terminal feels alive without a VM.
      term.writeln("\x1b[2mModo local. Escribe y pulsa Enter.\x1b[0m");
      term.write(LOCAL_PROMPT);
      term.onData((data) => {
        for (const ch of data) {
          const code = ch.charCodeAt(0);
          if (code === 13) {
            term.write("\r\n");
            if (localLine.length > 0) {
              term.writeln(`\x1b[2m(eco) ${localLine}\x1b[0m`);
            }
            localLine = "";
            term.write(LOCAL_PROMPT);
          } else if (code === 127) {
            if (localLine.length > 0) {
              localLine = localLine.slice(0, -1);
              term.write("\b \b");
            }
          } else if (code >= 32) {
            localLine += ch;
            term.write(ch);
          }
        }
      });
    }

    return () => {
      window.removeEventListener("resize", onResize);
      pty?.close();
      term.dispose();
    };
  }, [ptyWsUrl]);

  return <div ref={containerRef} className="h-full w-full" />;
}
