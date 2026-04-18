"use client";

// Lightweight per-browser progress store. Phase A persists in localStorage
// only; once we have user accounts, the same shape moves to the server.

import type { Lesson } from "./lessons";

const KEY = "learn-progress-v1";

export type ModuleProgress = {
  completedSteps: string[];
  xp: number;
  startedAt?: string;
  lastVisited: string;
  finishedAt?: string;
};

export type ProgressState = {
  modules: Record<string, ModuleProgress>;
  lang: "es" | "en";
};

function emptyState(): ProgressState {
  return { modules: {}, lang: "es" };
}

export function loadProgress(): ProgressState {
  if (typeof window === "undefined") return emptyState();
  try {
    const raw = window.localStorage.getItem(KEY);
    if (!raw) return emptyState();
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") return emptyState();
    return {
      modules: parsed.modules ?? {},
      lang: parsed.lang === "en" ? "en" : "es",
    };
  } catch {
    return emptyState();
  }
}

export function saveProgress(state: ProgressState): void {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(KEY, JSON.stringify(state));
}

export function setLang(lang: "es" | "en"): void {
  const s = loadProgress();
  s.lang = lang;
  saveProgress(s);
}

export function markStepComplete(
  moduleId: string,
  stepId: string,
  xp: number,
): ProgressState {
  const state = loadProgress();
  const now = new Date().toISOString();
  const existing = state.modules[moduleId] ?? {
    completedSteps: [],
    xp: 0,
    startedAt: now,
    lastVisited: now,
  };
  if (!existing.completedSteps.includes(stepId)) {
    existing.completedSteps.push(stepId);
    existing.xp += xp;
  }
  existing.lastVisited = now;
  state.modules[moduleId] = existing;
  saveProgress(state);
  return state;
}

export function markModuleComplete(moduleId: string): ProgressState {
  const state = loadProgress();
  const now = new Date().toISOString();
  const existing = state.modules[moduleId];
  if (existing) {
    existing.finishedAt = now;
    existing.lastVisited = now;
    state.modules[moduleId] = existing;
    saveProgress(state);
  }
  return state;
}

export function resetProgress(): void {
  if (typeof window === "undefined") return;
  window.localStorage.removeItem(KEY);
}

export function resetModule(moduleId: string): ProgressState {
  const state = loadProgress();
  if (state.modules[moduleId]) {
    delete state.modules[moduleId];
    saveProgress(state);
  }
  return state;
}

export type LessonState = "locked" | "current" | "done";

// Linear unlock: a module is current when it's the lowest-numbered one not
// yet finished. Earlier ones are done. Later ones are locked until current
// is finished. The very first module (lowest module_number in the catalog)
// is always at least current.
export function deriveStates(
  lessons: Lesson[],
  progress: ProgressState,
): Map<string, LessonState> {
  const out = new Map<string, LessonState>();
  const sorted = [...lessons].sort((a, b) => a.module_number - b.module_number);
  let foundCurrent = false;
  for (const l of sorted) {
    const m = progress.modules[l.id];
    const isFinished =
      !!m && m.completedSteps.length >= l.steps.length;
    if (isFinished) {
      out.set(l.id, "done");
      continue;
    }
    if (!foundCurrent) {
      out.set(l.id, "current");
      foundCurrent = true;
    } else {
      out.set(l.id, "locked");
    }
  }
  return out;
}

export function totalXp(progress: ProgressState): number {
  let sum = 0;
  for (const m of Object.values(progress.modules)) sum += m.xp;
  return sum;
}

// Streak: number of consecutive UTC days the user touched any lesson up to
// and including today. If they didn't touch one today the streak is the run
// ending yesterday.
export function streakDays(progress: ProgressState): number {
  const days = new Set<string>();
  for (const m of Object.values(progress.modules)) {
    if (!m.lastVisited) continue;
    days.add(m.lastVisited.slice(0, 10));
  }
  if (days.size === 0) return 0;
  const sorted = [...days].sort().reverse();
  const today = new Date().toISOString().slice(0, 10);
  let cursor = sorted[0] === today ? today : sorted[0];
  let n = 0;
  for (const d of sorted) {
    if (d === cursor) {
      n += 1;
      const next = new Date(cursor);
      next.setUTCDate(next.getUTCDate() - 1);
      cursor = next.toISOString().slice(0, 10);
    } else {
      break;
    }
  }
  return n;
}
