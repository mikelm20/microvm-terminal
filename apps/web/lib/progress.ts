"use client";

// LocalStorage-backed per-lesson progress. Shared key so one read gives you
// every lesson the learner has touched. Server-rendered code never imports
// this module.

const STORAGE_KEY = "learn.progress.v1";

export type LessonProgress = {
  lessonId: string;
  completedStepIds: string[];
  lastOpenedAt: string; // ISO
};

type ProgressMap = Record<string, LessonProgress>;

function readMap(): ProgressMap {
  if (typeof window === "undefined") return {};
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed: unknown = JSON.parse(raw);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as ProgressMap;
    }
    return {};
  } catch {
    return {};
  }
}

function writeMap(map: ProgressMap): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(map));
  } catch {
    // ignore quota / private mode
  }
}

export function readLessonProgress(lessonId: string): LessonProgress {
  const map = readMap();
  const existing = map[lessonId];
  if (existing) return existing;
  return {
    lessonId,
    completedStepIds: [],
    lastOpenedAt: new Date().toISOString(),
  };
}

export function touchLesson(lessonId: string): LessonProgress {
  const map = readMap();
  const existing = map[lessonId] ?? {
    lessonId,
    completedStepIds: [],
    lastOpenedAt: new Date().toISOString(),
  };
  const next: LessonProgress = {
    ...existing,
    lastOpenedAt: new Date().toISOString(),
  };
  map[lessonId] = next;
  writeMap(map);
  return next;
}

export function markStepCompleted(
  lessonId: string,
  stepId: string,
): LessonProgress {
  const map = readMap();
  const existing = map[lessonId] ?? {
    lessonId,
    completedStepIds: [],
    lastOpenedAt: new Date().toISOString(),
  };
  const completed = new Set(existing.completedStepIds);
  completed.add(stepId);
  const next: LessonProgress = {
    ...existing,
    completedStepIds: Array.from(completed),
    lastOpenedAt: new Date().toISOString(),
  };
  map[lessonId] = next;
  writeMap(map);
  return next;
}

export function readAllProgress(): ProgressMap {
  return readMap();
}
