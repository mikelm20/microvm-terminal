import { useEffect, useState } from "react";
import { View, Text, Pressable, Dimensions } from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import Animated, {
  FadeIn,
  FadeOut,
  useSharedValue,
  useAnimatedStyle,
  withTiming,
} from "react-native-reanimated";
import Svg, { Defs, LinearGradient as SvgLinearGradient, Rect, Stop } from "react-native-svg";
import { SafeAreaView } from "react-native-safe-area-context";
import { t } from "../../../lib/i18n";
import { haptics } from "../../../lib/haptics";
import { motion, enterTiming } from "../../../lib/motion";
import type { Lang } from "@learn/shared-voice";

const AUTO_ADVANCE_MS = motion.duration.moment;

/**
 * Full-bleed celebration. Burgundy-to-flame gradient, evidence-derived
 * headline, XP odometer, streak tick if first of the day, "Sigue".
 * Auto-advances after 1.4s unless the user taps.
 */
export default function Moment() {
  const params = useLocalSearchParams<{
    lessonId: string;
    stepId?: string;
    xp?: string;
    headline?: string;
    streak?: string;
    first_of_day?: string;
    lang?: string;
  }>();

  const lang = (params.lang === "en" ? "en" : "es") as Lang;
  const xpTarget = Number(params.xp ?? 0);
  const streak = Number(params.streak ?? 0);
  const firstOfDay = params.first_of_day === "1";
  const headline = params.headline ?? fallbackHeadline(lang, params.stepId);

  const [xp, setXp] = useState(0);

  useEffect(() => {
    haptics.noteStepCompleted();
    void haptics.milestone();

    const start = Date.now();
    const frame = setInterval(() => {
      const elapsed = Date.now() - start;
      const pct = Math.min(1, elapsed / 900);
      setXp(Math.round(xpTarget * easeOutCubic(pct)));
      if (pct >= 1) clearInterval(frame);
    }, 30);

    const tAuto = setTimeout(() => {
      close();
    }, AUTO_ADVANCE_MS);

    return () => {
      clearInterval(frame);
      clearTimeout(tAuto);
    };
  }, [xpTarget]);

  function close() {
    if (router.canGoBack()) router.back();
  }

  return (
    <Animated.View
      entering={FadeIn.duration(motion.duration.base)}
      exiting={FadeOut.duration(motion.duration.snap)}
      style={{ flex: 1 }}
    >
      <BackgroundGradient />
      <SafeAreaView className="flex-1">
        <Pressable className="flex-1 items-center justify-center px-8" onPress={close}>
          <HeroGlow />
          <Text className="text-ink-primary text-3xl leading-9 text-center">
            {headline}
          </Text>

          <View className="mt-8 items-center">
            <Text className="text-flame-glow text-5xl tracking-tight">
              {t(lang, "lesson.moment.xp_earned", { xp })}
            </Text>
            {firstOfDay && streak > 0 ? (
              <Text className="text-ink-secondary text-base mt-3">
                {t(lang, "lesson.moment.streak_tick", { n: streak })}
              </Text>
            ) : null}
          </View>

          <View className="mt-12 rounded-pill bg-warm px-8 py-3">
            <Text className="text-deep text-base">
              {t(lang, "lesson.moment.continue")}
            </Text>
          </View>
        </Pressable>
      </SafeAreaView>
    </Animated.View>
  );
}

function fallbackHeadline(lang: Lang, stepId?: string): string {
  void stepId;
  return lang === "es"
    ? "Primer encuentro con Claude"
    : "First encounter with Claude";
}

function easeOutCubic(t: number): number {
  return 1 - Math.pow(1 - t, 3);
}

/**
 * Expo SDK 52 does not bundle LinearGradient (that is in expo-linear-gradient,
 * which is not in our deps). react-native-svg is already a dep, so we paint
 * the gradient with SVG instead. Full-bleed covers the whole screen.
 */
function BackgroundGradient() {
  const { width, height } = Dimensions.get("window");
  return (
    <View style={{ position: "absolute", top: 0, bottom: 0, left: 0, right: 0 }}>
      <Svg width={width} height={height}>
        <Defs>
          <SvgLinearGradient id="bg" x1="0" y1="0" x2="0.25" y2="1">
            <Stop offset="0" stopColor="#6e2608" />
            <Stop offset="0.55" stopColor="#a8391d" />
            <Stop offset="1" stopColor="#ff9b5a" />
          </SvgLinearGradient>
        </Defs>
        <Rect x="0" y="0" width={width} height={height} fill="url(#bg)" />
      </Svg>
    </View>
  );
}

/** Ambient glow ring behind the headline. */
function HeroGlow() {
  const scale = useSharedValue(0.9);
  useEffect(() => {
    scale.value = withTiming(1.0, enterTiming(motion.duration.settle));
  }, [scale]);
  const style = useAnimatedStyle(() => ({ transform: [{ scale: scale.value }] }));
  return (
    <Animated.View
      style={style}
      className="absolute w-80 h-80 rounded-pill bg-flame-glow opacity-20"
    />
  );
}

