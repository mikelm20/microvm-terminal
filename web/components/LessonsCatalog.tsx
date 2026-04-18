"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import type { Lesson } from "@/lib/lessons";
import {
  deriveStates,
  loadProgress,
  resetProgress,
  setLang,
  streakDays,
  totalXp,
  type LessonState,
  type ProgressState,
} from "@/lib/progress";

export default function LessonsCatalog({
  lessonsEs,
  lessonsEn,
}: {
  lessonsEs: Lesson[];
  lessonsEn: Lesson[];
}) {
  const [progress, setProgress] = useState<ProgressState | null>(null);
  const [lang, setLangState] = useState<"es" | "en">("es");

  useEffect(() => {
    const p = loadProgress();
    setProgress(p);
    setLangState(p.lang);
  }, []);

  const lessons = lang === "en" ? lessonsEn : lessonsEs;
  const states = progress
    ? deriveStates(lessons, progress)
    : new Map<string, LessonState>();
  const xp = progress ? totalXp(progress) : 0;
  const streak = progress ? streakDays(progress) : 0;

  function switchLang(next: "es" | "en") {
    setLang(next);
    setLangState(next);
  }

  function handleResetAll() {
    const confirmMsg =
      lang === "en"
        ? "Reset all progress? You'll lose XP, streak, and completed modules."
        : "¿Resetear todo el progreso? Pierdes XP, racha y modulos completados.";
    if (!window.confirm(confirmMsg)) return;
    resetProgress();
    setProgress({ modules: {}, lang });
  }

  return (
    <div className="min-h-screen bg-learn-burgundy text-learn-warm relative overflow-x-hidden">
      <Topbar
        xp={xp}
        streak={streak}
        lang={lang}
        onSwitchLang={switchLang}
        onResetAll={handleResetAll}
        hasProgress={!!progress && Object.keys(progress.modules).length > 0}
      />

      <main className="relative z-10 max-w-3xl mx-auto px-6 pt-28 pb-24">
        <header className="text-center mb-16">
          <h1 className="text-4xl md:text-5xl font-normal text-learn-warmHi tracking-tight mb-3">
            {lang === "en" ? "Your path" : "Tu camino"}
          </h1>
          <p className="text-learn-warm/65 text-base">
            {lang === "en"
              ? "Follow the thread. Each module unlocks the next."
              : "Sigue el hilo. Cada modulo te desbloquea el siguiente."}
          </p>
        </header>

        <Tree lessons={lessons} states={states} lang={lang} />
      </main>
    </div>
  );
}

function Topbar({
  xp,
  streak,
  lang,
  onSwitchLang,
  onResetAll,
  hasProgress,
}: {
  xp: number;
  streak: number;
  lang: "es" | "en";
  onSwitchLang: (l: "es" | "en") => void;
  onResetAll: () => void;
  hasProgress: boolean;
}) {
  return (
    <div className="fixed top-0 left-0 right-0 z-20 px-8 py-5 flex items-center justify-between">
      <div className="flex items-center gap-3 text-learn-warm/90">
        <span className="text-[11px] tracking-[0.22em] uppercase">
          learn.example.com
        </span>
      </div>
      <div className="flex items-center gap-3">
        <Stat label={lang === "en" ? "day" : "dia"} value={streak} icon="flame" />
        <Stat label="XP" value={xp} icon="bolt" />
        <button
          onClick={() => onSwitchLang(lang === "es" ? "en" : "es")}
          className="text-xs tracking-wider uppercase text-learn-warm/60 hover:text-learn-warm border border-learn-warm/20 rounded-full px-3 py-1.5"
        >
          {lang === "es" ? "EN" : "ES"}
        </button>
        {hasProgress && (
          <button
            onClick={onResetAll}
            className="text-xs text-learn-warm/50 hover:text-learn-warm border border-learn-warm/15 rounded-full px-3 py-1.5"
            title={lang === "en" ? "Reset all progress" : "Resetear todo el progreso"}
          >
            {lang === "en" ? "Reset" : "Reset"}
          </button>
        )}
      </div>
    </div>
  );
}

function Stat({
  label,
  value,
  icon,
}: {
  label: string;
  value: number;
  icon: "flame" | "bolt";
}) {
  return (
    <span className="inline-flex items-center gap-2 px-3 py-1.5 bg-learn-warm/10 border border-learn-warm/20 rounded-full text-sm">
      <span className="w-3.5 h-3.5 inline-flex items-center justify-center">
        {icon === "flame" ? (
          <svg viewBox="0 0 24 24" className="w-full h-full">
            <path
              fill="#ff9b5a"
              d="M12 2c.2 2.4 1.5 4.6 3.3 6.2 1.8 1.6 2.9 3.6 2.9 5.6 0 3.7-2.8 6.7-6.5 6.7S5.2 17.5 5.2 13.8c0-1.3.5-2.5 1.3-3.4.2.9 1 1.5 2 1.5 1.2 0 2.2-1 2.2-2.2 0-1.4-.3-4.3-.7-7.7.7 1 1.8 2 3 2z"
            />
          </svg>
        ) : (
          <svg viewBox="0 0 24 24" className="w-full h-full">
            <path
              fill="#ffc591"
              d="M13.6 2 4 13.5h6.4L9.5 22l10-12.4h-6.5z"
            />
          </svg>
        )}
      </span>
      <span className="text-learn-warmHi">{value}</span>
      <span className="text-[11px] opacity-60">{label}</span>
    </span>
  );
}

function Tree({
  lessons,
  states,
  lang,
}: {
  lessons: Lesson[];
  states: Map<string, LessonState>;
  lang: "es" | "en";
}) {
  return (
    <div className="relative px-8">
      {lessons.map((lesson, i) => {
        const state = states.get(lesson.id) ?? "locked";
        const onLeft = i % 2 === 0;
        return (
          <Node
            key={lesson.id}
            lesson={lesson}
            state={state}
            onLeft={onLeft}
            lang={lang}
          />
        );
      })}
    </div>
  );
}

function Node({
  lesson,
  state,
  onLeft,
  lang,
}: {
  lesson: Lesson;
  state: LessonState;
  onLeft: boolean;
  lang: "es" | "en";
}) {
  const baseCircle =
    "w-[88px] h-[88px] rounded-full flex items-center justify-center text-3xl font-normal absolute top-0 z-10 transition-transform";
  const positionCircle = onLeft ? "left-0" : "right-0";
  const positionInfo = onLeft
    ? "left-[112px] text-left"
    : "right-[112px] text-right";

  let circleClass = baseCircle + " " + positionCircle + " ";
  let circleContent: React.ReactNode = null;
  let infoOpacity = "";
  if (state === "done") {
    circleClass +=
      "bg-gradient-to-b from-learn-successMid to-learn-successDark text-white shadow-[0_8px_28px_rgba(79,168,92,0.45)]";
    circleContent = <span className="text-[34px] font-semibold">✓</span>;
  } else if (state === "current") {
    circleClass +=
      "bg-gradient-to-b from-learn-ember to-learn-rust text-learn-deep cursor-pointer animate-pulseRing";
    circleContent = <span>{lesson.module_number}</span>;
  } else {
    circleClass +=
      "bg-learn-warm/10 border border-learn-warm/15 text-learn-warm/30";
    circleContent = <span className="text-2xl opacity-50">🔒</span>;
    infoOpacity = "opacity-40";
  }

  const tag =
    state === "done"
      ? lang === "en"
        ? `Module ${lesson.module_number}, completed`
        : `Modulo ${lesson.module_number}, completado`
      : state === "current"
        ? lang === "en"
          ? `Module ${lesson.module_number}, next`
          : `Modulo ${lesson.module_number}, siguiente`
        : `${lang === "en" ? "Module" : "Modulo"} ${lesson.module_number}`;

  const href = `/lessons/${lesson.id}?lang=${lesson.language}`;

  // Done modules are still entrable so the user can review or redo them.
  const interactive = state !== "locked";

  return (
    <div className="relative h-[120px] mb-14 last:mb-0">
      {state === "current" && (
        <div className="absolute z-20" style={{ top: "-18px", left: onLeft ? "44px" : "auto", right: onLeft ? "auto" : "44px", transform: "translateX(-50%)" }}>
          <span className="bg-learn-warmHi text-learn-burgundy-mid px-3 py-1 rounded-full text-[10px] font-semibold tracking-[0.08em] uppercase whitespace-nowrap shadow">
            {lang === "en" ? "Start" : "Empezar"}
          </span>
        </div>
      )}

      {interactive ? (
        <Link href={href} className={circleClass}>
          {circleContent}
        </Link>
      ) : (
        <div className={circleClass}>{circleContent}</div>
      )}

      <div
        className={`absolute top-1/2 -translate-y-1/2 max-w-[260px] ${positionInfo} ${infoOpacity}`}
      >
        <div className="text-[11px] uppercase tracking-[0.1em] opacity-55 mb-1">
          {tag}
        </div>
        <div className="text-lg font-normal text-learn-warmHi leading-tight tracking-tight mb-1">
          {lesson.title}
        </div>
        <div className="text-sm opacity-60 leading-snug">
          {lesson.subtitle ?? ""}
        </div>
        {state === "current" && (
          <Link
            href={href}
            className="inline-flex items-center gap-2 mt-3 px-4 py-2 bg-learn-warmHi text-learn-burgundy-mid rounded-full text-[13px] font-medium shadow"
          >
            {lang === "en" ? "Continue" : "Continuar"}
            <span>→</span>
          </Link>
        )}
      </div>
    </div>
  );
}
