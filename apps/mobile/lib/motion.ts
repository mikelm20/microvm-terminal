import {
  Easing,
  withSequence,
  withTiming,
  withDelay,
  type WithTimingConfig,
} from "react-native-reanimated";
import tokens from "@learn/shared-tokens/json";

/**
 * Motion helpers that read curves and durations from tokens.json.
 * Components call these from within worklets or useAnimatedStyle blocks.
 * We pass cubic-bezier as a bezier easing; duration comes as ms integers.
 */

function parseDurationMs(value: string): number {
  return parseInt(value.replace("ms", ""), 10);
}

/** Parse a CSS cubic-bezier(a,b,c,d) string into an Easing.bezier. */
function parseCurve(value: string) {
  const match = /cubic-bezier\(\s*([\d.]+)\s*,\s*([\d.-]+)\s*,\s*([\d.]+)\s*,\s*([\d.-]+)\s*\)/.exec(
    value,
  );
  if (!match) return Easing.out(Easing.cubic);
  const a = parseFloat(match[1]!);
  const b = parseFloat(match[2]!);
  const c = parseFloat(match[3]!);
  const d = parseFloat(match[4]!);
  return Easing.bezier(a, b, c, d);
}

export const motion = {
  duration: {
    snap: parseDurationMs(tokens.motion.duration.snap),
    base: parseDurationMs(tokens.motion.duration.base),
    settle: parseDurationMs(tokens.motion.duration.settle),
    moment: parseDurationMs(tokens.motion.duration.moment),
  },
  easing: {
    enter: parseCurve(tokens.motion.curve.enter),
    exit: parseCurve(tokens.motion.curve.exit),
    ambient: parseCurve(tokens.motion.curve.ambient),
  },
} as const;

/** Timing config for bubble/card enter. */
export function enterTiming(duration: number = motion.duration.base): WithTimingConfig {
  return { duration, easing: motion.easing.enter };
}

/** Timing config for screen exit. */
export function exitTiming(duration: number = motion.duration.base): WithTimingConfig {
  return { duration, easing: motion.easing.exit };
}

/** Ambient breathe sequence, looped in a useFrameCallback or derived value. */
export function breatheSequence(delay: number = 0) {
  return withDelay(
    delay,
    withSequence(
      withTiming(1.04, { duration: 1600, easing: motion.easing.ambient }),
      withTiming(1.0, { duration: 1600, easing: motion.easing.ambient }),
    ),
  );
}
