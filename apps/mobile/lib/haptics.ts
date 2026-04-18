import * as Haptics from "expo-haptics";
import { AccessibilityInfo } from "react-native";

/**
 * Haptic vocabulary tied to semantic signals from tokens.json.
 * Off until the learner completes their 3rd step across the app lifetime,
 * and always suppressed when Reduce Motion is on.
 */

let completedStepsLifetime = 0;
let reduceMotionCached: boolean | null = null;

async function reduceMotionOn(): Promise<boolean> {
  if (reduceMotionCached !== null) return reduceMotionCached;
  try {
    reduceMotionCached = await AccessibilityInfo.isReduceMotionEnabled();
  } catch {
    reduceMotionCached = false;
  }
  return reduceMotionCached;
}

async function guard(): Promise<boolean> {
  if (completedStepsLifetime < 3) return false;
  if (await reduceMotionOn()) return false;
  return true;
}

export const haptics = {
  /** Bump the lifetime counter. After the 3rd tick, haptics are allowed. */
  noteStepCompleted(): void {
    completedStepsLifetime += 1;
  },

  /** For the forced-on path during the initial moments while onboarding. */
  allowImmediately(): void {
    completedStepsLifetime = 999;
  },

  /** 12ms tap. Send button, tool-call card enter. */
  async tap(): Promise<void> {
    if (!(await guard())) return;
    try {
      await Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    } catch {
      // ignore platforms where haptics are unsupported
    }
  },

  /** Medium double pulse. Step satisfied, action confirmed. */
  async confirm(): Promise<void> {
    if (!(await guard())) return;
    try {
      await Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    } catch {
      // ignore
    }
  },

  /** Heavy pulse. Moment take-over, streak milestone. */
  async milestone(): Promise<void> {
    if (!(await guard())) return;
    try {
      await Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Heavy);
    } catch {
      // ignore
    }
  },
};
