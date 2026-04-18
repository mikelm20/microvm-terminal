// Local-first progress store.
//
// The learner's Path progress is the source of truth on the device. Every
// reducer mutates the in-memory state, schedules a debounced sync to the
// server, and persists the new state to secure-store. On app foreground the
// store re-syncs to catch up with other devices.
//
// Grace tokens: earn 1 per 7 consecutive days, max 2 active, silently
// consume when a day is missed.

import { useEffect, useState, useSyncExternalStore } from "react";
import * as SecureStore from "expo-secure-store";
import { AppState, Platform } from "react-native";
import {
  ProgressStateSchema,
  type ProgressState,
} from "@learn/shared-api/schemas";
import { getIdentity } from "./identity";

const STATE_KEY = "learn-progress-v1";
const LAST_VISIT_KEY = "learn-last-visit-v1";
const DEBOUNCE_MS = 500;

const emptyState = (): ProgressState => ({
  modules: {},
  streak_days: 0,
  grace_tokens: 0,
  last_visited: new Date().toISOString(),
  seen_coachmarks: [],
  updated_at: new Date().toISOString(),
});

// -------- storage helpers --------

async function readItem(key: string): Promise<string | null> {
  if (Platform.OS === "web") {
    if (typeof globalThis.localStorage === "undefined") return null;
    return globalThis.localStorage.getItem(key);
  }
  return SecureStore.getItemAsync(key);
}

async function writeItem(key: string, value: string): Promise<void> {
  if (Platform.OS === "web") {
    if (typeof globalThis.localStorage === "undefined") return;
    globalThis.localStorage.setItem(key, value);
    return;
  }
  await SecureStore.setItemAsync(key, value);
}

async function deleteStorageItem(key: string): Promise<void> {
  if (Platform.OS === "web") {
    if (typeof globalThis.localStorage === "undefined") return;
    globalThis.localStorage.removeItem(key);
    return;
  }
  await SecureStore.deleteItemAsync(key);
}

// -------- streak math --------

const DAY_MS = 24 * 60 * 60 * 1000;

function dayIndex(iso: string): number {
  // Floor to UTC day.
  const t = new Date(iso).getTime();
  return Math.floor(t / DAY_MS);
}

// -------- store --------

let state: ProgressState = emptyState();
let hydrated = false;
let syncTimer: ReturnType<typeof setTimeout> | null = null;
let apiBaseUrl: string | null = null;

// Dev-only: a forced "now" override so we can simulate clock-forward from
// Settings. When set, getNowIso() returns this instead of the real clock.
let nowOverride: string | null = null;
const NOW_OVERRIDE_KEY = "learn-now-override-v1";

const listeners = new Set<() => void>();

function emit(): void {
  for (const l of listeners) l();
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

function getNowIso(): string {
  return nowOverride ?? new Date().toISOString();
}

async function persist(): Promise<void> {
  await writeItem(STATE_KEY, JSON.stringify(state));
}

function scheduleSync(): void {
  if (syncTimer) clearTimeout(syncTimer);
  syncTimer = setTimeout(() => {
    syncTimer = null;
    void syncNow();
  }, DEBOUNCE_MS);
}

async function syncNow(): Promise<void> {
  if (!apiBaseUrl) return;
  try {
    const id = await getIdentity();
    const res = await fetch(`${apiBaseUrl}/progress/sync`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(id.cookie ? { Cookie: id.cookie } : {}),
      },
      body: JSON.stringify({ uuid: id.uuid, state }),
    });
    if (!res.ok) return;
    const body = (await res.json()) as { state: ProgressState; server_updated_at: string };
    // Last-write-wins on updated_at. If server is newer, adopt it.
    if (Date.parse(body.state.updated_at) > Date.parse(state.updated_at)) {
      state = body.state;
      await persist();
      emit();
    }
  } catch {
    // Silent: we stay local until next foreground.
  }
}

// -------- public API --------

export async function hydrateProgress(baseUrl: string): Promise<void> {
  apiBaseUrl = baseUrl;
  if (hydrated) return;
  const raw = await readItem(STATE_KEY);
  if (raw) {
    try {
      const parsed = ProgressStateSchema.parse(JSON.parse(raw));
      state = parsed;
    } catch {
      state = emptyState();
    }
  } else {
    state = emptyState();
  }
  const overrideRaw = await readItem(NOW_OVERRIDE_KEY);
  nowOverride = overrideRaw || null;
  hydrated = true;
  emit();

  // On foreground, re-sync.
  AppState.addEventListener("change", (s) => {
    if (s === "active") void syncNow();
  });
  // Initial catch-up.
  void syncNow();
}

export function getProgress(): ProgressState {
  return state;
}

export function useProgress(): ProgressState {
  return useSyncExternalStore(subscribe, getProgress, getProgress);
}

// Track last-visit across launches: we tick streak on first visit of a new
// day, consume grace on a missed day, break streak on more than one missed
// day. Called from the root layout on mount and on foreground.
export async function touchVisit(): Promise<void> {
  const nowIso = getNowIso();
  const lastRaw = await readItem(LAST_VISIT_KEY);
  await writeItem(LAST_VISIT_KEY, nowIso);

  if (!lastRaw) {
    // First ever visit: start the streak.
    state = {
      ...state,
      streak_days: Math.max(1, state.streak_days),
      last_visited: nowIso,
      updated_at: nowIso,
    };
    await persist();
    emit();
    scheduleSync();
    return;
  }

  const last = dayIndex(lastRaw);
  const now = dayIndex(nowIso);
  const diff = now - last;

  if (diff <= 0) {
    // Same-day visit. Just bump last_visited.
    state = { ...state, last_visited: nowIso, updated_at: nowIso };
    await persist();
    emit();
    scheduleSync();
    return;
  }

  if (diff === 1) {
    // Next-day visit. Tick streak. Maybe earn a grace token.
    const newStreak = state.streak_days + 1;
    const earnedGrace = newStreak > 0 && newStreak % 7 === 0;
    const nextGrace = earnedGrace ? Math.min(2, state.grace_tokens + 1) : state.grace_tokens;
    state = {
      ...state,
      streak_days: newStreak,
      grace_tokens: nextGrace,
      last_visited: nowIso,
      updated_at: nowIso,
    };
  } else if (diff === 2 && state.grace_tokens > 0) {
    // Missed one day. Silently consume a grace token, streak intact, tick.
    state = {
      ...state,
      streak_days: state.streak_days + 1,
      grace_tokens: state.grace_tokens - 1,
      last_visited: nowIso,
      updated_at: nowIso,
    };
  } else {
    // Missed more than we can cover. Streak breaks. Start a new 1-day run.
    state = {
      ...state,
      streak_days: 1,
      last_visited: nowIso,
      updated_at: nowIso,
    };
  }
  await persist();
  emit();
  scheduleSync();
}

// Reducers (exported per CONTRACTS.md spec).

export async function completeStep(
  moduleId: string,
  stepId: string,
  xp: number,
): Promise<void> {
  const mod = state.modules[moduleId] ?? {
    completed_steps: [],
    paused_at: null,
    xp_earned: 0,
  };
  if (mod.completed_steps.includes(stepId)) {
    state = { ...state, updated_at: getNowIso() };
  } else {
    state = {
      ...state,
      modules: {
        ...state.modules,
        [moduleId]: {
          completed_steps: [...mod.completed_steps, stepId],
          paused_at: null,
          xp_earned: mod.xp_earned + xp,
        },
      },
      updated_at: getNowIso(),
    };
  }
  await persist();
  emit();
  scheduleSync();
}

export async function startModule(moduleId: string): Promise<void> {
  if (state.modules[moduleId]) return;
  state = {
    ...state,
    modules: {
      ...state.modules,
      [moduleId]: {
        completed_steps: [],
        paused_at: null,
        xp_earned: 0,
      },
    },
    updated_at: getNowIso(),
  };
  await persist();
  emit();
  scheduleSync();
}

export async function pauseModule(moduleId: string): Promise<void> {
  const mod = state.modules[moduleId];
  if (!mod) return;
  state = {
    ...state,
    modules: {
      ...state.modules,
      [moduleId]: { ...mod, paused_at: getNowIso() },
    },
    updated_at: getNowIso(),
  };
  await persist();
  emit();
  scheduleSync();
}

export async function resumeModule(moduleId: string): Promise<void> {
  const mod = state.modules[moduleId];
  if (!mod) return;
  state = {
    ...state,
    modules: {
      ...state.modules,
      [moduleId]: { ...mod, paused_at: null },
    },
    updated_at: getNowIso(),
  };
  await persist();
  emit();
  scheduleSync();
}

export async function addGraceToken(): Promise<void> {
  state = {
    ...state,
    grace_tokens: Math.min(2, state.grace_tokens + 1),
    updated_at: getNowIso(),
  };
  await persist();
  emit();
  scheduleSync();
}

export async function consumeGraceToken(): Promise<void> {
  state = {
    ...state,
    grace_tokens: Math.max(0, state.grace_tokens - 1),
    updated_at: getNowIso(),
  };
  await persist();
  emit();
  scheduleSync();
}

export async function tickStreak(): Promise<void> {
  const newStreak = state.streak_days + 1;
  const earnedGrace = newStreak > 0 && newStreak % 7 === 0;
  state = {
    ...state,
    streak_days: newStreak,
    grace_tokens: earnedGrace ? Math.min(2, state.grace_tokens + 1) : state.grace_tokens,
    updated_at: getNowIso(),
  };
  await persist();
  emit();
  scheduleSync();
}

export async function breakStreak(): Promise<void> {
  state = { ...state, streak_days: 0, updated_at: getNowIso() };
  await persist();
  emit();
  scheduleSync();
}

export async function markCoachmarkSeen(id: string): Promise<void> {
  if (state.seen_coachmarks.includes(id)) return;
  state = {
    ...state,
    seen_coachmarks: [...state.seen_coachmarks, id],
    updated_at: getNowIso(),
  };
  await persist();
  emit();
  scheduleSync();
}

export async function resetAll(): Promise<void> {
  state = emptyState();
  await deleteStorageItem(STATE_KEY);
  await deleteStorageItem(LAST_VISIT_KEY);
  emit();
}

// ---- Dev-only helpers ----

export async function setDevNowOverride(iso: string | null): Promise<void> {
  nowOverride = iso;
  if (iso) {
    await writeItem(NOW_OVERRIDE_KEY, iso);
  } else {
    await deleteStorageItem(NOW_OVERRIDE_KEY);
  }
  emit();
}

export async function setDevLastVisit(iso: string): Promise<void> {
  await writeItem(LAST_VISIT_KEY, iso);
}

export async function devJumpDays(days: number): Promise<void> {
  const base = new Date();
  const target = new Date(base.getTime() + days * DAY_MS);
  await setDevNowOverride(target.toISOString());
}

// Return-bucket classification: 2-7d short, 7-21d medium, 21+d long.
export type ReturnBucket = "none" | "short" | "medium" | "long";

export function returnBucketFor(nowIso: string, lastIso: string): ReturnBucket {
  const diffDays = Math.floor(
    (new Date(nowIso).getTime() - new Date(lastIso).getTime()) / DAY_MS,
  );
  if (diffDays < 2) return "none";
  if (diffDays < 7) return "short";
  if (diffDays < 21) return "medium";
  return "long";
}

export function currentReturnBucket(): { bucket: ReturnBucket; days: number } {
  const nowIso = nowOverride ?? new Date().toISOString();
  const bucket = returnBucketFor(nowIso, state.last_visited);
  const days = Math.max(
    0,
    Math.floor(
      (new Date(nowIso).getTime() - new Date(state.last_visited).getTime()) / DAY_MS,
    ),
  );
  return { bucket, days };
}

// Optional hook so components rerender when the override flips.
export function useDevNowOverride(): string | null {
  const [v, setV] = useState<string | null>(nowOverride);
  useEffect(() => {
    return subscribe(() => setV(nowOverride));
  }, []);
  return v;
}
