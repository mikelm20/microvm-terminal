"use client";

import Link from "next/link";

export default function Landing() {
  return (
    <div className="min-h-screen bg-learn-burgundy text-learn-warm relative overflow-hidden">
      <div className="fixed top-0 left-0 right-0 z-20 px-10 py-6 flex items-center justify-between">
        <span className="text-[11px] tracking-[0.22em] uppercase text-learn-warm/85">
          learn.example.com
        </span>
        <Link
          href="/sandbox"
          className="text-[13px] opacity-70 hover:opacity-100 px-4 py-2 rounded-full hover:bg-learn-warm/10 transition-colors"
        >
          Sandbox libre
        </Link>
      </div>

      <section className="relative z-10 min-h-screen flex flex-col items-center justify-center px-6 text-center">
        <div className="inline-flex items-center gap-2 px-3.5 py-1.5 bg-learn-warm/10 border border-learn-warm/20 rounded-full text-xs tracking-wider uppercase font-normal opacity-85 mb-6">
          <span className="w-1.5 h-1.5 rounded-full bg-learn-successMid shadow-[0_0_10px_rgba(111,207,122,0.8)]" />
          Sin instalar nada
        </div>

        <h1 className="text-5xl md:text-6xl lg:text-7xl font-normal text-learn-warmHi leading-[1.04] tracking-tight max-w-[20ch] mb-6">
          Aprende Claude&nbsp;Code{" "}
          <em className="not-italic font-normal bg-learn-flame bg-clip-text text-transparent">
            usandolo
          </em>
        </h1>

        <p className="text-base md:text-lg opacity-75 max-w-[52ch] leading-relaxed mb-9">
          Haz, no leas.
        </p>

        <Link
          href="/lessons/m2-primera-conversacion?lang=es"
          className="inline-flex items-center gap-3 px-9 py-4 bg-learn-warmHi text-learn-burgundy-mid rounded-full text-base font-medium shadow-[0_12px_48px_rgba(0,0,0,0.2)] hover:-translate-y-0.5 transition-transform"
        >
          Empezar gratis
          <span className="inline-block relative w-[18px] h-[18px]">
            <span className="absolute top-1/2 right-0 w-3 h-px bg-current -translate-y-1/2" />
            <span className="absolute top-1/2 right-0 w-2 h-2 border-r border-t border-current rotate-45 -translate-y-1/2" />
          </span>
        </Link>

        <Link
          href="/lessons"
          className="mt-5 text-sm opacity-60 hover:opacity-100"
        >
          Ver el camino completo
        </Link>
      </section>

      <div className="absolute bottom-10 left-1/2 -translate-x-1/2 flex gap-9 items-center text-xs opacity-55 whitespace-nowrap">
        <Proof num="5 min" txt="a tu primer prompt" />
        <span className="w-[3px] h-[3px] rounded-full bg-learn-warm/30" />
        <Proof num="7" txt="modulos" />
        <span className="w-[3px] h-[3px] rounded-full bg-learn-warm/30" />
        <Proof num="0" txt="instalacion" />
      </div>
    </div>
  );
}

function Proof({ num, txt }: { num: string; txt: string }) {
  return (
    <span className="inline-flex items-baseline gap-2">
      <span className="text-[15px] font-medium text-learn-warmHi tracking-tight">
        {num}
      </span>
      <span className="text-xs">{txt}</span>
    </span>
  );
}
